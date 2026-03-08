package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/YagorX/shop-gateway/internal/domain"
	"github.com/YagorX/shop-gateway/internal/observability"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/contracts"
)

type ProductsHandler struct {
	service contracts.ProductService
}

func NewProductsHandler(product_service contracts.ProductService) *ProductsHandler {
	return &ProductsHandler{service: product_service}
}

func (h *ProductsHandler) List(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	status := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/products").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/products", status).Inc()
	}()

	if r.Method != http.MethodGet {
		status = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.service == nil {
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "product service is not initialized")
		return
	}

	limit, offset, err := parsePagination(r)
	if err != nil {
		status = "400"
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	products, err := h.service.ListProducts(r.Context(), limit, offset)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidPagination) {
			status = "400"
			writeError(w, http.StatusBadRequest, "invalid_pagination", err.Error())
			return
		}
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": products,
		"count": len(products),
	})
}

func (h *ProductsHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	status := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/products/{id}").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/products/{id}", status).Inc()
	}()

	if r.Method != http.MethodGet {
		status = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.service == nil {
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "product service is not initialized")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/products/")
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "/") {
		status = "400"
		writeError(w, http.StatusBadRequest, "invalid_product_id", "invalid product id")
		return
	}

	product, err := h.service.GetProduct(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			status = "404"
			writeError(w, http.StatusNotFound, "product_not_found", err.Error())
			return
		}
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, product)
}

func parsePagination(r *http.Request) (int, int, error) {
	q := r.URL.Query()

	limit := 0
	offset := 0

	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, errors.New("limit must be integer")
		}
		limit = v
	}

	if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, errors.New("offset must be integer")
		}
		offset = v
	}

	return limit, offset, nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
