package middleware

import (
	"log/slog"
	"net/http"

	"github.com/YagorX/shop-gateway/internal/transport/http/v1/contracts"
)

// RequireAdmin пропускает только администраторов. Ставится после Auth: токен к этому моменту
// уже проверен и user_uuid лежит в контексте, остаётся спросить auth, админ ли пользователь.
func RequireAdmin(logger *slog.Logger, authService contracts.AuthService) Middleware {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const op = "http.middleware.RequireAdmin"

			log := logger.With(
				slog.String("op", op),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)

			if authService == nil {
				log.Error("auth service is nil")
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			}

			userUUID, ok := UserUUIDFromContext(r.Context())
			if !ok {
				// Ошибка сборки маршрута, а не клиента: перед RequireAdmin не поставили Auth.
				log.Error("user uuid is missing in context: RequireAdmin must run after Auth")
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			}

			isAdmin, err := authService.IsAdmin(r.Context(), userUUID)
			if err != nil {
				writeUpstreamError(w, log, err)
				return
			}

			if !isAdmin {
				log.Warn("user is not admin", slog.String("user_uuid", userUUID))
				writeError(w, http.StatusForbidden, "forbidden", "admin access required")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
