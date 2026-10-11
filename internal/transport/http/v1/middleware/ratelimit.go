package middleware

import (
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"

	"github.com/YagorX/shop-gateway/internal/ratelimit"
)

func RateLimit(logger *slog.Logger, limiter *ratelimit.Limiter) Middleware {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const op = "http.middleware.RateLimit"

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

			allowed, retryAfter, err := limiter.Allow(r.Context(), host)
			if err != nil {
				// Fail-open: без Redis теряем лимит по IP, но не весь сайт.
				// Перегрузку auth при этом всё равно сдерживает семафор на bcrypt.
				log.Error("rate limiter unavailable, request allowed", slog.String("error", err.Error()))
				next.ServeHTTP(w, r)
				return
			}

			if !allowed {
				log.Warn("rate limit exceeded", slog.String("remote_addr", ip))
				// Заголовок ставится до writeError: после WriteHeader он уже не уйдёт клиенту.
				w.Header().Set("Retry-After", strconv.Itoa(max(1, int(math.Ceil(retryAfter.Seconds())))))
				writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "too many requests")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
