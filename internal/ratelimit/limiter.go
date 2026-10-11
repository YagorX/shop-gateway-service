package ratelimit

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type Limiter struct {
	client *redis.Client
	limit  int
	window time.Duration
}

func NewLimiter(client *redis.Client, limit int, window time.Duration) *Limiter {
	return &Limiter{
		client: client,
		limit:  limit,
		window: window,
	}
}

var slidingWindowScript = redis.NewScript(`
    local key    = KEYS[1]
	local window = tonumber(ARGV[1])
	local limit  = tonumber(ARGV[2])
	local now    = tonumber(ARGV[3])
	local member = ARGV[4]

	redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
	local count = tonumber(redis.call('ZCARD', key))

	if count < limit then
		redis.call('ZADD', key, now, member)
		redis.call('PEXPIRE', key, window)
		return {1, 0}
	end

	-- Место освободится, когда самый старый запрос выпадет из окна.
	local oldest = tonumber(redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')[2])
	return {0, oldest + window - now}
`)

// Allow записывает запрос, если в окне ещё есть место. При отказе retryAfter — через сколько
// место освободится: тогда самый старый запрос окна выпадет из него.
func (l *Limiter) Allow(ctx context.Context, key string) (allowed bool, retryAfter time.Duration, err error) {
	now := time.Now().UnixMilli()
	// Элементы ZSET уникальны: с member = now два запроса в одну миллисекунду слились бы в одну запись.
	member := fmt.Sprintf("%d-%d", now, rand.Int64())

	res, err := slidingWindowScript.Run(
		ctx,
		l.client,
		[]string{key},
		l.window.Milliseconds(),
		l.limit,
		now,
		member,
	).Int64Slice()
	if err != nil {
		return false, 0, err
	}
	if len(res) != 2 {
		return false, 0, fmt.Errorf("ratelimit: unexpected script result %v", res)
	}

	return res[0] == 1, time.Duration(res[1]) * time.Millisecond, nil
}
