package middleware_test

import (
	"context"
	"encoding/json"
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

const loginFailureKey = "ratelimit:login-failures:203.0.113.5"

// Обработчик-заглушка: отвечает заданным статусом и считает, сколько раз до него дошли.
type loginStub struct {
	status int
	calls  int
}

func (s *loginStub) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	s.calls++
	w.WriteHeader(s.status)
}

func loginFailureLimited(t *testing.T, limit int, next http.Handler) (http.Handler, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	limiter := ratelimit.NewFailureLimiter(client, limit, 15*time.Minute, "ratelimit:login-failures")

	return middleware.LoginFailureLimit(discardLogger(), limiter)(next), mr
}

func doLoginFromIP(h http.Handler, ctx context.Context) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = "203.0.113.5:51234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func failuresRecorded(t *testing.T, mr *miniredis.Miniredis) int {
	t.Helper()

	if !mr.Exists(loginFailureKey) {
		return 0
	}
	v, err := mr.Get(loginFailureKey)
	if err != nil {
		t.Fatalf("read counter: %v", err)
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		t.Fatalf("counter = %q, want integer", v)
	}

	return n
}

// Считается только 401. Успешный вход (офис за одним IP), 400, 429 от auth и 5xx — не попытки подбора.
func TestLoginFailureLimitCountsOnlyUnauthorized(t *testing.T) {
	tests := []struct {
		status int
		want   int
	}{
		{status: http.StatusOK, want: 0},
		{status: http.StatusBadRequest, want: 0},
		{status: http.StatusTooManyRequests, want: 0},
		{status: http.StatusInternalServerError, want: 0},
		{status: http.StatusUnauthorized, want: 1},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			h, mr := loginFailureLimited(t, 10, &loginStub{status: tt.status})

			rec := doLoginFromIP(h, context.Background())
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d passed through", rec.Code, tt.status)
			}
			if got := failuresRecorded(t, mr); got != tt.want {
				t.Fatalf("failures recorded = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLoginFailureLimitBlocksAfterLimit(t *testing.T) {
	next := &loginStub{status: http.StatusUnauthorized}
	h, _ := loginFailureLimited(t, 2, next)

	for i := range 2 {
		if rec := doLoginFromIP(h, context.Background()); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}

	rec := doLoginFromIP(h, context.Background())
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d after the limit, want 429", rec.Code)
	}
	if next.calls != 2 {
		t.Fatalf("handler called %d times, want 2: a blocked IP must not reach auth", next.calls)
	}

	// Ответ в общем конверте API, а не text/plain: фронт разбирает error.code.
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %q", rec.Body.String())
	}
	if body.Error.Code != "too_many_login_failures" {
		t.Fatalf("error.code = %q, want too_many_login_failures", body.Error.Code)
	}

	secs, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	if err != nil || secs < 1 || secs > 15*60 {
		t.Fatalf("Retry-After = %q, want 1..900 seconds", rec.Header().Get("Retry-After"))
	}
}

// Сто успешных входов с одного IP — обычное утро в офисе, а не атака.
func TestLoginFailureLimitIgnoresSuccessfulLogins(t *testing.T) {
	h, _ := loginFailureLimited(t, 2, &loginStub{status: http.StatusOK})

	for i := range 100 {
		if rec := doLoginFromIP(h, context.Background()); rec.Code != http.StatusOK {
			t.Fatalf("login %d: status = %d, want 200", i+1, rec.Code)
		}
	}
}

// Fail-open: без Redis теряется лимит по IP, но не вход; подбор сдерживает лимит по логину в auth.
func TestLoginFailureLimitFailsOpenWhenRedisIsDown(t *testing.T) {
	next := &loginStub{status: http.StatusUnauthorized}
	h, mr := loginFailureLimited(t, 1, next)
	mr.Close()

	if rec := doLoginFromIP(h, context.Background()); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want the handler's 401 with Redis down", rec.Code)
	}
	if next.calls != 1 {
		t.Fatalf("handler called %d times, want 1", next.calls)
	}
}

// Клиент, закрывший соединение после ответа, неудачу всё равно получает: иначе обрыв обходил бы счёт.
func TestLoginFailureLimitRecordsFailureAfterClientCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		cancel()
	})
	h, mr := loginFailureLimited(t, 10, next)

	doLoginFromIP(h, ctx)

	if got := failuresRecorded(t, mr); got != 1 {
		t.Fatalf("failures recorded = %d after the client cancelled, want 1", got)
	}
}

func TestLoginFailureLimitWithoutLimiterPassesThrough(t *testing.T) {
	next := &loginStub{status: http.StatusUnauthorized}
	h := middleware.LoginFailureLimit(discardLogger(), nil)(next)

	if rec := doLoginFromIP(h, context.Background()); rec.Code != http.StatusUnauthorized || next.calls != 1 {
		t.Fatalf("status = %d, calls = %d; want the request passed to the handler", rec.Code, next.calls)
	}
}
