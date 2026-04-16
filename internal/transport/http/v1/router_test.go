package v1

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
