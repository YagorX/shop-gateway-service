package ratelimit

import (
	"context"
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

	redis.call('ZREMRANGEBYSCORE', key, 0, now - window)

	local count = tonumber(redis.call('ZCARD', key))

	if count < limit then
		redis.call('ZADD', key, now, now)
		redis.call('PEXPIRE', key, window)
		return 1
	end
	return 0
`)

func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {

	result, err := slidingWindowScript.Run(
		ctx,
		l.client,
		[]string{key},
		l.window.Milliseconds(),
		l.limit,
		time.Now().UnixMilli(),
	).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}
