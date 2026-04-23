package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/YagorX/shop-gateway/internal/domain"
	"github.com/YagorX/shop-gateway/internal/observability"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/contracts"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/middleware"
)

type CartHandler struct {
	cartService    contracts.CartService
	productService contracts.ProductService
}

func NewCartHandler(cart_service contracts.CartService, product_service contracts.ProductService) *CartHandler {
	return &CartHandler{
		cartService:    cart_service,
		productService: product_service,
	}
}

func (h *CartHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	status := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/cart/items").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/cart/items", status).Inc()
	}()

	if r.Method != http.MethodPost {
		status = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.cartService == nil || h.productService == nil {
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "service is not initialized")
		return
	}

	userID, ok := middleware.UserUUIDFromContext(r.Context())
	if !ok {
		status = "401"
		writeError(w, http.StatusUnauthorized, "unauthorized", "user not authenticated")
		return
	}

	var req struct {
		ProductID string `json:"product_id"`
		Quantity  int32  `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		status = "400"
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	product, err := h.productService.GetProduct(r.Context(), req.ProductID)
	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			status = "404"
			writeError(w, http.StatusNotFound, "not_found", "product not found")
			return
		}
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to get product")
		return
	}

	if err = h.cartService.AddItem(r.Context(), userID, req.ProductID, req.Quantity, product.PriceCents); err != nil {
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to add item")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *CartHandler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	status := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/cart/items").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/cart/items", status).Inc()
	}()

	if r.Method != http.MethodDelete {
		status = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.cartService == nil {
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "cart service is not initialized")
		return
	}

	userID, ok := middleware.UserUUIDFromContext(r.Context())
	if !ok {
		status = "401"
		writeError(w, http.StatusUnauthorized, "unauthorized", "user not authenticated")
		return
	}

	var req struct {
		ProductID string `json:"product_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		status = "400"
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	if err := h.cartService.RemoveItem(r.Context(), userID, req.ProductID); err != nil {
		if errors.Is(err, domain.ErrCartNotFound) {
			status = "404"
			writeError(w, http.StatusNotFound, "not_found", "cart not found")
			return
		}
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to remove item")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *CartHandler) ClearCart(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	status := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/cart/clear").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/cart/clear", status).Inc()
	}()

	if r.Method != http.MethodDelete {
		status = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.cartService == nil {
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "cart service is not initialized")
		return
	}

	userID, ok := middleware.UserUUIDFromContext(r.Context())
	if !ok {
		status = "401"
		writeError(w, http.StatusUnauthorized, "unauthorized", "user not authenticated")
		return
	}

	if err := h.cartService.ClearCart(r.Context(), userID); err != nil {
		if errors.Is(err, domain.ErrCartNotFound) {
			status = "404"
			writeError(w, http.StatusNotFound, "not_found", "cart not found")
			return
		}
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to clear cart")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *CartHandler) GetCart(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	status := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/cart").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/cart", status).Inc()
	}()

	if r.Method != http.MethodGet {
		status = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.cartService == nil {
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "cart service is not initialized")
		return
	}

	userID, ok := middleware.UserUUIDFromContext(r.Context())
	if !ok {
		status = "401"
		writeError(w, http.StatusUnauthorized, "unauthorized", "user not authenticated")
		return
	}

	cart, err := h.cartService.GetCart(r.Context(), userID)
	if err != nil {
		if errors.Is(err, domain.ErrCartNotFound) {
			status = "404"
			writeError(w, http.StatusNotFound, "not_found", "cart not found")
			return
		}
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to get cart")
		return
	}

	items := make([]map[string]any, len(cart.Items))
	for i, item := range cart.Items {
		items[i] = map[string]any{
			"product_id":             item.ProductID,
			"quantity":               item.Quantity,
			"price_snapshot_kopecks": item.PriceSnapshotKopecks,
			"added_at":               item.AddedAt.Format(time.RFC3339),
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":    cart.UserID,
		"items":      items,
		"created_at": cart.CreatedAt.Format(time.RFC3339),
		"updated_at": cart.UpdatedAt.Format(time.RFC3339),
	})
}

func (h *CartHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	status := "200"
	defer func() {
		metrics.GatewayHTTPRequestDuration.WithLabelValues(r.Method, "/cart/items/update").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayHTTPRequestsTotal.WithLabelValues(r.Method, "/cart/items/update", status).Inc()
	}()

	if r.Method != http.MethodPatch {
		status = "405"
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if h.cartService == nil {
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "cart service is not initialized")
		return
	}

	userID, ok := middleware.UserUUIDFromContext(r.Context())
	if !ok {
		status = "401"
		writeError(w, http.StatusUnauthorized, "unauthorized", "user not authenticated")
		return
	}

	var req struct {
		ProductID string `json:"product_id"`
		Quantity  int32  `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		status = "400"
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	err := h.cartService.UpdateItem(r.Context(), userID, req.ProductID, req.Quantity)
	if err != nil {
		if errors.Is(err, domain.ErrCartNotFound) {
			status = "404"
			writeError(w, http.StatusNotFound, "not_found", "cart not found")
			return
		}
		status = "500"
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update item in cart")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
