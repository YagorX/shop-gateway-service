package middleware_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/YagorX/shop-gateway/internal/transport/http/v1/middleware"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type stubAuthService struct {
	validateErr error
	isAdmin     bool
	isAdminErr  error
}

func (s stubAuthService) Register(context.Context, string, string, string) (string, error) {
	return "", nil
}
func (s stubAuthService) Login(context.Context, string, string, int64, string) (string, string, error) {
	return "", "", nil
}
func (s stubAuthService) ValidateToken(context.Context, string, int64) (string, error) {
	return "user-uuid", s.validateErr
}
func (s stubAuthService) Refresh(context.Context, string, int64, string) (string, string, error) {
	return "", "", nil
}
func (s stubAuthService) Logout(context.Context, string, int64, string) error {
	return nil
}
func (s stubAuthService) IsAdmin(context.Context, string) (bool, error) {
	return s.isAdmin, s.isAdminErr
}

// 401 заставляет фронт стереть токены. Отвечать им можно, только когда auth сказал, что токен плохой;
// если auth недоступен, это 5xx, иначе рестарт auth разлогинивает всех и вызывает шторм логинов.
func TestAuthMiddlewareMapsValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "valid token", err: nil, want: http.StatusOK},
		{name: "bad token", err: status.Error(codes.Unauthenticated, "invalid token"), want: http.StatusUnauthorized},
		{name: "deactivated user", err: status.Error(codes.NotFound, "user not found"), want: http.StatusUnauthorized},
		{name: "auth unavailable", err: status.Error(codes.Unavailable, "connection refused"), want: http.StatusServiceUnavailable},
		{name: "auth timeout", err: status.Error(codes.DeadlineExceeded, "deadline exceeded"), want: http.StatusGatewayTimeout},
		{name: "auth internal", err: status.Error(codes.Internal, "internal error"), want: http.StatusInternalServerError},
		{
			// Клиент gateway оборачивает ошибку через %w: код должен читаться и из обёртки.
			name: "wrapped unavailable",
			err:  fmt.Errorf("client.grpc.auth.ValidateToken: %w", status.Error(codes.Unavailable, "connection refused")),
			want: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			h := middleware.Auth(discardLogger(), stubAuthService{validateErr: tt.err})(next)

			req := httptest.NewRequest(http.MethodGet, "/cart", nil)
			req.Header.Set("Authorization", "Bearer token")
			req.Header.Set("X-App-Id", "1")
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}
