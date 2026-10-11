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
