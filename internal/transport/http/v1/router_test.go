package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/YagorX/shop-gateway/internal/ratelimit"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/contracts"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/handlers"
)

func TestNewRouter_ServesSwaggerSpecFromConfiguredDir(t *testing.T) {
	t.Parallel()

	specDir := t.TempDir()
	specContent := "openapi: 3.0.3\ninfo:\n  title: Test API\n"
	specPath := filepath.Join(specDir, "gateway.v1.yaml")
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	router := NewRouter(RouterDeps{
		SwaggerSpecDir: specDir,
	})

	req := httptest.NewRequest(http.MethodGet, "/swagger/spec/gateway.v1.yaml", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != specContent {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestNewRouter_ServesRedocWhenConfigured(t *testing.T) {
	t.Parallel()

	redocPath := filepath.Join(t.TempDir(), "redoc.html")
	redocContent := "<html><body>ReDoc</body></html>"
	if err := os.WriteFile(redocPath, []byte(redocContent), 0o644); err != nil {
		t.Fatalf("write redoc: %v", err)
	}

	router := NewRouter(RouterDeps{
		SwaggerHandler: handlers.NewSwaggerHandler("", redocPath, "openapi/gateway.v1.yaml"),
	})

	req := httptest.NewRequest(http.MethodGet, "/redoc/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "ReDoc") {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestNewRouter_ServesStatusWhenTemplateConfigured(t *testing.T) {
	t.Parallel()

	templatePath := filepath.Join(t.TempDir(), "status.html")
	template := "<html><body>{{.ServiceName}} {{.Status}}</body></html>"
	if err := os.WriteFile(templatePath, []byte(template), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	router := NewRouter(RouterDeps{
		StatusHandler: handlers.NewStatusHandler(templatePath, handlers.StatusPageData{
			ServiceName: "gateway-service",
			Status:      "ready",
		}),
	})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "gateway-service") {
		t.Fatalf("expected rendered status page, got %q", body)
	}
}

func TestNewRouter_SkipsStatusRouteWhenTemplateMissing(t *testing.T) {
	t.Parallel()

	router := NewRouter(RouterDeps{
		StatusHandler: handlers.NewStatusHandler("", handlers.StatusPageData{
			ServiceName: "gateway-service",
		}),
	})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when status route is not configured, got %d", rec.Code)
	}
}

// Неверный пароль — как его отдаёт gRPC-клиент gateway.
type unauthenticatedAuthService struct{ contracts.AuthService }

func (unauthenticatedAuthService) Login(context.Context, string, string, int64, string) (string, string, error) {
	return "", "", status.Error(codes.Unauthenticated, "invalid email or password")
}

// /auth/login подключён через лимит неудач по IP: после порога запрос до auth не доходит.
func TestNewRouter_LoginGoesThroughFailureLimit(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	router := NewRouter(RouterDeps{
		AuthService:         unauthenticatedAuthService{},
		LoginFailureLimiter: ratelimit.NewFailureLimiter(client, 1, time.Minute, "ratelimit:login-failures"),
	})

	login := func() int {
		body := strings.NewReader(`{"email_or_name":"user","password":"wrong","app_id":1,"device_id":"d1"}`)
		req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
		req.RemoteAddr = "203.0.113.5:51234"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		return rec.Code
	}

	if code := login(); code != http.StatusUnauthorized {
		t.Fatalf("first login: status = %d, want 401", code)
	}
	if code := login(); code != http.StatusTooManyRequests {
		t.Fatalf("second login: status = %d, want 429 from the IP failure limit", code)
	}
}
