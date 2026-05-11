package middleware

import (
	"log/slog"
	"net"
	"net/http"

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

			allowed, err := limiter.Allow(r.Context(), host)
			if err != nil {
				log.Error("failed to check rate limit", slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			}

			if !allowed {
				log.Warn("rate limit exceeded", slog.String("remote_addr", ip))
				writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "too many requests")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
