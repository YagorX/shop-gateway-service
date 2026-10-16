package ratelimit

import (
	"context"
	"errors"
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

// FailureLimiter считает только неудачи (например, 401 на вход), а не все запросы: с одного IP
// бывает целый офис, и успешные входы его не должны блокировать. Фиксированное окно от первой неудачи:
// счётчик — одна строка в Redis, точность скользящего окна здесь не нужна.
type FailureLimiter struct {
	client *redis.Client
	limit  int
	window time.Duration
	prefix string
}

func NewLimiter(client *redis.Client, limit int, window time.Duration) *Limiter {
	return &Limiter{
		client: client,
		limit:  limit,
		window: window,
	}
}

func NewFailureLimiter(client *redis.Client, limit int, window time.Duration, prefix string) *FailureLimiter {
	return &FailureLimiter{
		client: client,
		limit:  limit,
		window: window,
		prefix: prefix,
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

// Fail засчитывает неудачу. blocked — эта неудача достигла порога: блокировку логируют один раз.
func (l *FailureLimiter) Fail(ctx context.Context, key string) (blocked bool, err error) {
	k := l.key(key)

	var incr *redis.IntCmd
	// MULTI: INCR и EXPIRE применяются вместе. Без него при обрыве соединения между командами
	// мог остаться ключ без TTL, и IP был бы заблокирован навсегда.
	_, err = l.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		incr = pipe.Incr(ctx, k)
		// NX: срок ставит только первая неудача, окно считается от неё. Без NX каждая неудача
		// продлевала бы окно, и у офиса с редкими опечатками счётчик не обнулялся бы никогда.
		pipe.ExpireNX(ctx, k, l.window)
		return nil
	})
	if err != nil {
		return false, err
	}

	return incr.Val() == int64(l.limit), nil
}

// Blocked — через сколько IP разблокируется; 0 — не заблокирован.
func (l *FailureLimiter) Blocked(ctx context.Context, key string) (time.Duration, error) {
	k := l.key(key)

	var get *redis.StringCmd
	var ttl *redis.DurationCmd
	_, err := l.client.Pipelined(ctx, func(p redis.Pipeliner) error {
		get = p.Get(ctx, k)
		ttl = p.PTTL(ctx, k)
		return nil
	})
	// Нет ключа — нет неудач. Pipelined отдаёт redis.Nil от GET как ошибку всего конвейера.
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	count, err := get.Int()
	if err != nil {
		return 0, err
	}
	if count < l.limit {
		return 0, nil
	}

	// Ключ мог истечь между GET и PTTL (PTTL = -2): блокировка кончилась.
	return max(ttl.Val(), 0), nil
}

func (l *FailureLimiter) key(key string) string {
	return fmt.Sprintf("%s:%s", l.prefix, key)
}
