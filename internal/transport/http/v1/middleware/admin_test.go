package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/YagorX/shop-gateway/internal/transport/http/v1/middleware"
)

// Так /admin/log-level собран в роутере: сначала Auth проверяет токен, потом RequireAdmin — роль.
func adminChain(svc stubAuthService) http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	return middleware.Auth(discardLogger(), svc)(middleware.RequireAdmin(discardLogger(), svc)(next))
}

func adminRequest(withToken bool) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/admin/log-level?level=debug", nil)
	if withToken {
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("X-App-Id", "1")
	}

	return req
}

func TestRequireAdmin(t *testing.T) {
	tests := []struct {
		name      string
		svc       stubAuthService
		withToken bool
		want      int
	}{
		{name: "admin", svc: stubAuthService{isAdmin: true}, withToken: true, want: http.StatusOK},
		{name: "not admin", svc: stubAuthService{isAdmin: false}, withToken: true, want: http.StatusForbidden},
		{name: "no token", svc: stubAuthService{isAdmin: true}, withToken: false, want: http.StatusUnauthorized},
		{
			name:      "bad token",
			svc:       stubAuthService{isAdmin: true, validateErr: status.Error(codes.Unauthenticated, "invalid token")},
			withToken: true,
			want:      http.StatusUnauthorized,
		},
		{
			// Недоступный auth — не повод ни пускать, ни разлогинивать.
			name:      "auth unavailable on role check",
			svc:       stubAuthService{isAdminErr: status.Error(codes.Unavailable, "connection refused")},
			withToken: true,
			want:      http.StatusServiceUnavailable,
		},
		{
			name:      "auth timeout on role check",
			svc:       stubAuthService{isAdminErr: status.Error(codes.DeadlineExceeded, "deadline exceeded")},
			withToken: true,
			want:      http.StatusGatewayTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			adminChain(tt.svc).ServeHTTP(rec, adminRequest(tt.withToken))

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

// Без Auth перед собой RequireAdmin не знает пользователя и обязан отказать, а не пропустить.
func TestRequireAdminWithoutAuthFailsClosed(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := middleware.RequireAdmin(discardLogger(), stubAuthService{isAdmin: true})(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, adminRequest(true))

	if rec.Code == http.StatusOK {
		t.Fatal("request passed RequireAdmin without Auth in front of it")
	}
}
