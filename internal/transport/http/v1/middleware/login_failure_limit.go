package middleware

import (
	"context"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/YagorX/shop-gateway/internal/ratelimit"
)

// LoginFailureLimit блокирует вход с IP, с которого пришло слишком много неудачных логинов.
// Ловит перебор по многим аккаунтам: лимит в auth считает неудачи на один логин и такого не видит.
// Считается только 401 — офис за одним IP успешными входами блокировку не набирает.
func LoginFailureLimit(logger *slog.Logger, limiter *ratelimit.FailureLimiter) Middleware {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const op = "http.middleware.LoginFailureLimit"

			log := logger.With(
				slog.String("op", op),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)

			if limiter == nil {
				log.Debug("rate limiter is not configured, skipping")
				next.ServeHTTP(w, r)
				return
			}

			ip := r.RemoteAddr
			if ip == "" {
				log.Warn("remote address is empty, skipping rate limit")
				next.ServeHTTP(w, r)
				return
			}

			host, _, err := net.SplitHostPort(ip)
			if err != nil {
				log.Warn("invalid remote address format, skipping rate limit", slog.String("remote_addr", ip), slog.String("error", err.Error()))
				next.ServeHTTP(w, r)
				return
			}

			retryAfter, err := limiter.Blocked(r.Context(), host)
			if err != nil {
				// Fail-open, как у общего лимитера: подбор тогда сдерживает лимит по логину в auth.
				log.Error("failed to check block status", slog.String("error", err.Error()))
				next.ServeHTTP(w, r)
				return
			}

			if retryAfter > 0 {
				// Debug, а не Warn: в атаку таких отказов тысячи. Сама блокировка залогирована один раз ниже.
				log.Debug("login failure limit exceeded", slog.String("remote_addr", host))
				// Заголовок ставится до writeError: после WriteHeader он уже не уйдёт клиенту.
				w.Header().Set("Retry-After", strconv.Itoa(max(1, int(math.Ceil(retryAfter.Seconds())))))
				writeError(w, http.StatusTooManyRequests, "too_many_login_failures", "too many failed logins from this address, retry later")
				return
			}

			// Статус нужен после обработчика. responseRecorder из idempotency.go копирует и тело,
			// но у логина оно маленькое.
			rec := &responseRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			// Только неверные учётные данные. 400, 429 от auth и 5xx — не попытки угадать пароль.
			if rec.statusCode != http.StatusUnauthorized {
				return
			}

			// Клиент мог уже закрыть соединение, но неудачу засчитать надо всё равно.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Second)
			defer cancel()

			blockedNow, err := limiter.Fail(ctx, host)
			if err != nil {
				log.Error("failed to record login failure", slog.String("error", err.Error()))
				return
			}
			if blockedNow {
				log.Warn("address blocked after repeated login failures", slog.String("remote_addr", host))
			}
		})
	}
}
