package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/YagorX/shop-gateway/internal/transport/http/v1/handlers"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubAuthService struct {
	loginErr    error
	registerErr error
}

func (s *stubAuthService) Register(context.Context, string, string, string) (string, error) {
	return "user-uuid", s.registerErr
}
func (s *stubAuthService) Login(context.Context, string, string, int64, string) (string, string, error) {
	return "access", "refresh", s.loginErr
}
func (s *stubAuthService) ValidateToken(context.Context, string, int64) (string, error) {
	return testUserID, nil
}
func (s *stubAuthService) Refresh(context.Context, string, int64, string) (string, string, error) {
	return "access", "refresh", nil
}
func (s *stubAuthService) Logout(context.Context, string, int64, string) error {
	return nil
}
func (s *stubAuthService) IsAdmin(context.Context, string) (bool, error) {
	return false, nil
}

// Ошибка в том виде, в каком её отдаёт gRPC-клиент gateway: статус auth, обёрнутый через %w.
func authError(code codes.Code, msg string) error {
	return fmt.Errorf("client.grpc.auth.Login: %w", status.Error(code, msg))
}

// Так auth отвечает при перегрузке семафора bcrypt: ResourceExhausted + RetryInfo.
func overloadedAuthError(t *testing.T, delay time.Duration) error {
	t.Helper()

	st, err := status.New(codes.ResourceExhausted, "server is busy, retry later").
		WithDetails(&errdetails.RetryInfo{RetryDelay: durationpb.New(delay)})
	if err != nil {
		t.Fatalf("build status: %v", err)
	}

	return fmt.Errorf("client.grpc.auth.Login: %w", st.Err())
}

func doLogin(t *testing.T, svc *stubAuthService) *httptest.ResponseRecorder {
	t.Helper()

	body := jsonBody(map[string]any{
		"email_or_name": "user",
		"password":      "secret",
		"app_id":        1,
		"device_id":     "device-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rec := httptest.NewRecorder()

	handlers.NewAuthHandler(svc).Login(rec, req)

	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	m := decodeBody(t, rec.Body)
	e, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("в ответе нет error: %v", m)
	}

	code, _ := e["code"].(string)

	return code
}

// ── error mapping ─────────────────────────────────────────────────────────────

// Коды upstream, которые фронт должен отличать от поломки: на них можно повторить запрос.
func TestAuthHandler_Login_MapsUpstreamErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantHTTP int
		wantCode string
	}{
		{
			name:     "wrong password",
			err:      authError(codes.Unauthenticated, "invalid email or password"),
			wantHTTP: http.StatusUnauthorized,
			wantCode: "unauthenticated",
		},
		{
			name:     "auth unavailable",
			err:      authError(codes.Unavailable, "connection refused"),
			wantHTTP: http.StatusServiceUnavailable,
			wantCode: "service_unavailable",
		},
		{
			name:     "auth timeout",
			err:      authError(codes.DeadlineExceeded, "context deadline exceeded"),
			wantHTTP: http.StatusGatewayTimeout,
			wantCode: "upstream_timeout",
		},
		{
			name:     "auth overloaded",
			err:      overloadedAuthError(t, time.Second),
			wantHTTP: http.StatusTooManyRequests,
			wantCode: "too_many_requests",
		},
		{
			name:     "unknown upstream failure",
			err:      authError(codes.Internal, "internal error"),
			wantHTTP: http.StatusInternalServerError,
			wantCode: "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doLogin(t, &stubAuthService{loginErr: tt.err})

			if rec.Code != tt.wantHTTP {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tt.wantHTTP, rec.Body.String())
			}
			if got := errorCode(t, rec); got != tt.wantCode {
				t.Fatalf("error.code = %q, want %q", got, tt.wantCode)
			}
		})
	}
}

// Пауза из RetryInfo доходит до клиента как Retry-After в целых секундах, округлённых вверх.
func TestAuthHandler_Login_OverloadSetsRetryAfter(t *testing.T) {
	tests := []struct {
		name  string
		delay time.Duration
		want  string
	}{
		{name: "whole seconds", delay: 2 * time.Second, want: "2"},
		{name: "fraction rounds up", delay: 1500 * time.Millisecond, want: "2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doLogin(t, &stubAuthService{loginErr: overloadedAuthError(t, tt.delay)})

			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("status = %d, want 429", rec.Code)
			}
			if got := rec.Header().Get("Retry-After"); got != tt.want {
				t.Fatalf("Retry-After = %q, want %q", got, tt.want)
			}
		})
	}
}

// Без RetryInfo в статусе Retry-After всё равно ставится: 429 без подсказки паузы провоцирует немедленный повтор.
func TestAuthHandler_Login_OverloadWithoutRetryInfo(t *testing.T) {
	rec := doLogin(t, &stubAuthService{loginErr: authError(codes.ResourceExhausted, "busy")})

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want %q", got, "1")
	}
}

func TestAuthHandler_Register_Overload(t *testing.T) {
	body := jsonBody(map[string]any{"username": "user", "email": "user@example.com", "password": "secret"})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", body)
	rec := httptest.NewRecorder()

	handlers.NewAuthHandler(&stubAuthService{registerErr: overloadedAuthError(t, time.Second)}).Register(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want %q", got, "1")
	}
}

// Так auth отвечает на логин, заблокированный после серии неудач: ResourceExhausted, как и перегрузка,
// но с ErrorInfo.Reason = LOGIN_LOCKED и паузой до разблокировки.
func lockedAuthError(t *testing.T, delay time.Duration) error {
	t.Helper()

	st, err := status.New(codes.ResourceExhausted, "too many failed login attempts, retry later").WithDetails(
		&errdetails.RetryInfo{RetryDelay: durationpb.New(delay)},
		&errdetails.ErrorInfo{Reason: "LOGIN_LOCKED", Domain: "auth.shop"},
	)
	if err != nil {
		t.Fatalf("build status: %v", err)
	}

	return fmt.Errorf("client.grpc.auth.Login: %w", st.Err())
}

// Блокировку логина клиент должен отличать от перегрузки: ждать минуты, а не секунду, и показать человеку причину.
func TestAuthHandler_Login_LockedIsNotOverload(t *testing.T) {
	rec := doLogin(t, &stubAuthService{loginErr: lockedAuthError(t, 5*time.Minute)})

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "300" {
		t.Fatalf("Retry-After = %q, want 300", got)
	}

	e, _ := decodeBody(t, rec.Body)["error"].(map[string]any)
	if code, _ := e["code"].(string); code != "login_locked" {
		t.Fatalf("error.code = %q, want login_locked", code)
	}
	if msg, _ := e["message"].(string); msg != "too many failed login attempts, retry later" {
		t.Fatalf("error.message = %q, want the message auth meant for the client", msg)
	}
}

// Перегрузка с ErrorInfo от нового auth по-прежнему too_many_requests.
func TestAuthHandler_Login_OverloadWithReason(t *testing.T) {
	st, err := status.New(codes.ResourceExhausted, "server is busy, retry later").WithDetails(
		&errdetails.RetryInfo{RetryDelay: durationpb.New(time.Second)},
		&errdetails.ErrorInfo{Reason: "SERVER_OVERLOADED", Domain: "auth.shop"},
	)
	if err != nil {
		t.Fatalf("build status: %v", err)
	}

	rec := doLogin(t, &stubAuthService{loginErr: fmt.Errorf("client.grpc.auth.Login: %w", st.Err())})

	if got := errorCode(t, rec); got != "too_many_requests" {
		t.Fatalf("error.code = %q, want too_many_requests", got)
	}
}
