package handlers

import (
	"net/http"
	"time"

	"github.com/YagorX/shop-gateway/internal/observability"
)

type SwaggerHandler struct {
	UIPath   string
	SpecPath string
}

func NewSwaggerHandler(uiPath, specPath string) *SwaggerHandler {
	if specPath == "" {
		return nil
	}

	return &SwaggerHandler{
		UIPath:   uiPath,
		SpecPath: specPath,
	}
}

func (swagger *SwaggerHandler) UI(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	status := "200"

	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/swagger/").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/swagger/", status).Inc()
	}()

	if r.Method != http.MethodGet {
		status = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if swagger == nil || swagger.UIPath == "" {
		status = "500"
		writeError(w, http.StatusInternalServerError, "UI path is empty", "UI path is empty")
		return
	}

	http.ServeFile(w, r, swagger.UIPath)
}

func (swagger *SwaggerHandler) OpenAPI(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	status := "200"

	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/swagger/openapi.yaml").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/swagger/openapi.yaml", status).Inc()
	}()

	if r.Method != http.MethodGet {
		status = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if swagger == nil || swagger.SpecPath == "" {
		status = "500"
		writeError(w, http.StatusInternalServerError, "SpecPath path is empty", "SpecPath path is empty")
		return
	}

	w.Header().Set("Content-Type", "application/yaml")
	http.ServeFile(w, r, swagger.SpecPath)
}
