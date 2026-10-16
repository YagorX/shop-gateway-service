package ratelimit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"

	"github.com/YagorX/shop-gateway/internal/ratelimit"
)

// miniredis выполняет Lua-скрипты, поэтому проверяется настоящий скрипт лимитера, а не заглушка.
func newLimiter(t *testing.T, limit int, window time.Duration) (*ratelimit.Limiter, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return ratelimit.NewLimiter(client, limit, window), mr
}

func TestLimiterAllowsUpToLimitThenRejects(t *testing.T) {
	const window = time.Minute

	l, _ := newLimiter(t, 3, window)
	ctx := context.Background()

	for i := range 3 {
		allowed, retryAfter, err := l.Allow(ctx, "203.0.113.5")
		if err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("request %d rejected, limit is 3", i+1)
		}
		if retryAfter != 0 {
			t.Fatalf("request %d: retryAfter = %v for an allowed request", i+1, retryAfter)
		}
	}

	allowed, retryAfter, err := l.Allow(ctx, "203.0.113.5")
	if err != nil {
		t.Fatalf("4th request: %v", err)
	}
	if allowed {
		t.Fatal("4th request allowed over the limit")
	}
	// Самый старый запрос сделан только что, значит место освободится почти через всё окно.
	if retryAfter <= 0 || retryAfter > window {
		t.Fatalf("retryAfter = %v, want in (0, %v]", retryAfter, window)
	}
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	l, _ := newLimiter(t, 1, time.Minute)
	ctx := context.Background()

	if allowed, _, _ := l.Allow(ctx, "203.0.113.5"); !allowed {
		t.Fatal("first IP rejected")
	}
	if allowed, _, _ := l.Allow(ctx, "198.51.100.7"); !allowed {
		t.Fatal("second IP rejected: limits must be per key")
	}
}

// Раньше элементом ZSET было само время: запросы в одну миллисекунду сливались в одну запись,
// и параллельный всплеск проходил сверх лимита.
func TestLimiterCountsConcurrentRequests(t *testing.T) {
	const limit = 10

	l, _ := newLimiter(t, limit, time.Minute)
	ctx := context.Background()

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, err := l.Allow(ctx, "203.0.113.5")
			if err != nil {
				t.Errorf("Allow: %v", err)
				return
			}
			if ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowed != limit {
		t.Fatalf("allowed %d of 50 concurrent requests, want exactly %d", allowed, limit)
	}
}

func TestLimiterWindowSlides(t *testing.T) {
	const window = 50 * time.Millisecond

	l, _ := newLimiter(t, 1, window)
	ctx := context.Background()

	if allowed, _, _ := l.Allow(ctx, "203.0.113.5"); !allowed {
		t.Fatal("first request rejected")
	}
	if allowed, _, _ := l.Allow(ctx, "203.0.113.5"); allowed {
		t.Fatal("second request allowed inside the window")
	}

	// Скрипт берёт время из аргумента, а не из Redis: ждём реальное окно.
	time.Sleep(window + 20*time.Millisecond)

	if allowed, _, _ := l.Allow(ctx, "203.0.113.5"); !allowed {
		t.Fatal("request rejected after the window slid past the old one")
	}
}

func TestLimiterReturnsErrorWhenRedisIsDown(t *testing.T) {
	l, mr := newLimiter(t, 1, time.Minute)
	mr.Close()

	if _, _, err := l.Allow(context.Background(), "203.0.113.5"); err == nil {
		t.Fatal("expected an error with Redis down")
	}
}

func newFailureLimiter(t *testing.T, limit int, window time.Duration) (*ratelimit.FailureLimiter, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return ratelimit.NewFailureLimiter(client, limit, window, "test:fail"), mr
}

func TestFailureLimiterBlocksAtLimit(t *testing.T) {
	const window = 15 * time.Minute

	l, _ := newFailureLimiter(t, 3, window)
	ctx := context.Background()

	if wait, err := l.Blocked(ctx, "203.0.113.5"); err != nil || wait != 0 {
		t.Fatalf("no failures yet: Blocked = %v, %v; want 0, nil", wait, err)
	}

	for i := 1; i <= 3; i++ {
		blockedNow, err := l.Fail(ctx, "203.0.113.5")
		if err != nil {
			t.Fatalf("failure %d: %v", i, err)
		}
		// Порог достигнут ровно один раз: по этому флагу блокировка логируется без повторов.
		if blockedNow != (i == 3) {
			t.Fatalf("failure %d: blockedNow = %v", i, blockedNow)
		}
		if i < 3 {
			if wait, _ := l.Blocked(ctx, "203.0.113.5"); wait != 0 {
				t.Fatalf("after %d failures Blocked = %v, want 0", i, wait)
			}
		}
	}

	wait, err := l.Blocked(ctx, "203.0.113.5")
	if err != nil {
		t.Fatalf("Blocked: %v", err)
	}
	if wait <= 0 || wait > window {
		t.Fatalf("Blocked = %v at the limit, want in (0, %v]", wait, window)
	}

	if blockedNow, _ := l.Fail(ctx, "203.0.113.5"); blockedNow {
		t.Fatal("failure over the limit reported blockedNow again")
	}
}

// Окно считается от первой неудачи (EXPIRE NX): поздние неудачи его не продлевают,
// иначе у офиса с редкими опечатками счётчик не обнулялся бы никогда.
func TestFailureLimiterWindowStartsAtFirstFailure(t *testing.T) {
	const window = 15 * time.Minute

	l, mr := newFailureLimiter(t, 100, window)
	ctx := context.Background()

	if _, err := l.Fail(ctx, "203.0.113.5"); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	mr.FastForward(10 * time.Minute)
	if _, err := l.Fail(ctx, "203.0.113.5"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	if ttl := mr.TTL("test:fail:203.0.113.5"); ttl != 5*time.Minute {
		t.Fatalf("TTL = %v after a second failure, want 5m: the window must not be extended", ttl)
	}

	mr.FastForward(5 * time.Minute)
	if mr.Exists("test:fail:203.0.113.5") {
		t.Fatal("counter outlived its window")
	}
}

func TestFailureLimiterUnblocksAfterWindow(t *testing.T) {
	l, mr := newFailureLimiter(t, 1, time.Minute)
	ctx := context.Background()

	if _, err := l.Fail(ctx, "203.0.113.5"); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if wait, _ := l.Blocked(ctx, "203.0.113.5"); wait == 0 {
		t.Fatal("not blocked at the limit")
	}

	mr.FastForward(time.Minute)

	if wait, err := l.Blocked(ctx, "203.0.113.5"); err != nil || wait != 0 {
		t.Fatalf("after the window Blocked = %v, %v; want 0, nil", wait, err)
	}
}

// Ключи лимитера живут под своим префиксом и не пересекаются с общим лимитером по голому IP.
func TestFailureLimiterKeysArePrefixedAndIndependent(t *testing.T) {
	l, mr := newFailureLimiter(t, 1, time.Minute)
	ctx := context.Background()

	if _, err := l.Fail(ctx, "203.0.113.5"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	if !mr.Exists("test:fail:203.0.113.5") {
		t.Fatalf("keys = %v, want test:fail:203.0.113.5", mr.Keys())
	}
	if wait, _ := l.Blocked(ctx, "198.51.100.7"); wait != 0 {
		t.Fatalf("another IP is blocked for %v", wait)
	}
}

// Ошибку Redis лимитер возвращает, а не глотает: решение fail-open принимает middleware.
func TestFailureLimiterReturnsRedisErrors(t *testing.T) {
	mr := miniredis.RunT(t)
	// Без повторов: иначе go-redis полторы секунды переподключается к закрытому порту на каждый вызов.
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	l := ratelimit.NewFailureLimiter(client, 1, time.Minute, "test:fail")
	mr.Close()

	if _, err := l.Blocked(context.Background(), "203.0.113.5"); err == nil {
		t.Fatal("Blocked: no error with Redis down")
	}
	if _, err := l.Fail(context.Background(), "203.0.113.5"); err == nil {
		t.Fatal("Fail: no error with Redis down")
	}
}
