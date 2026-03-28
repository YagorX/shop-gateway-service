package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/YagorX/shop-gateway/internal/transport/http/v1/contracts"
)

const (
	bearerPrefix = "Bearer "
	appIDHeader  = "X-App-Id"
)

type contextKey string

const (
	userUUIDKey contextKey = "user_uuid"
	appIDKey    contextKey = "app_id"
)

func UserUUIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}

	v, ok := ctx.Value(userUUIDKey).(string)
	return v, ok
}

func AppIDFromContext(ctx context.Context) (int64, bool) {
	if ctx == nil {
		return 0, false
	}

	v, ok := ctx.Value(appIDKey).(int64)
	return v, ok
}

func Auth(logger *slog.Logger, authService contracts.AuthService) Middleware {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const op = "http.middleware.Auth"

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

			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if authHeader == "" {
				log.Warn("authorization header is empty")
				writeError(w, http.StatusUnauthorized, "unauthorized", "authorization header is required")
				return
			}

			if !strings.HasPrefix(authHeader, bearerPrefix) {
				log.Warn("authorization header has invalid prefix")
				writeError(w, http.StatusUnauthorized, "unauthorized", "authorization header must use Bearer scheme")
				return
			}

			token := strings.TrimSpace(strings.TrimPrefix(authHeader, bearerPrefix))
			if token == "" {
				log.Warn("bearer token is empty")
				writeError(w, http.StatusUnauthorized, "unauthorized", "bearer token is required")
				return
			}

			rawAppID := strings.TrimSpace(r.Header.Get(appIDHeader))
			if rawAppID == "" {
				log.Warn("x-app-id header is empty")
				writeError(w, http.StatusBadRequest, "bad_request", "x-app-id header is required")
				return
			}

			appID, err := strconv.ParseInt(rawAppID, 10, 64)
			if err != nil || appID <= 0 {
				log.Warn("x-app-id header is invalid", slog.String("value", rawAppID))
				writeError(w, http.StatusBadRequest, "bad_request", "invalid x-app-id header")
				return
			}

			userUUID, err := authService.ValidateToken(r.Context(), token, appID)
			if err != nil {
				log.Warn("token validation failed",
					slog.Int64("app_id", appID),
					slog.String("error", err.Error()),
				)
				writeError(w, http.StatusUnauthorized, "unauthorized", "token validation failed")
				return
			}

			log.Debug("token validated successfully",
				slog.String("user_uuid", userUUID),
				slog.Int64("app_id", appID),
			)

			ctx := context.WithValue(r.Context(), userUUIDKey, userUUID)
			ctx = context.WithValue(ctx, appIDKey, appID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
