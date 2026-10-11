package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"

	"github.com/YagorX/shop-gateway/internal/ratelimit"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/middleware"
)

func rateLimited(t *testing.T, limit int) (http.Handler, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	limiter := ratelimit.NewLimiter(client, limit, time.Minute)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	return middleware.RateLimit(discardLogger(), limiter)(next), mr
}

func doFromIP(h http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/products", nil)
	req.RemoteAddr = "203.0.113.5:51234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func TestRateLimitRejectsWithRetryAfter(t *testing.T) {
	h, _ := rateLimited(t, 1)

	if rec := doFromIP(h); rec.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", rec.Code)
	}

	rec := doFromIP(h)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: status = %d, want 429", rec.Code)
	}

	// Окно — минута, первый запрос только что: ждать почти всю минуту, но не больше.
	secs, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	if err != nil {
		t.Fatalf("Retry-After = %q, want integer seconds", rec.Header().Get("Retry-After"))
	}
	if secs < 1 || secs > 60 {
		t.Fatalf("Retry-After = %d, want 1..60", secs)
	}
}

// Fail-open: отказ Redis не должен класть весь gateway.
func TestRateLimitFailsOpenWhenRedisIsDown(t *testing.T) {
	h, mr := rateLimited(t, 1)
	mr.Close()

	if rec := doFromIP(h); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: request must pass when Redis is down", rec.Code)
	}
}
