package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/YagorX/shop-gateway/internal/domain"
	gateway "github.com/YagorX/shop-gateway/internal/service/gateway"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/handlers"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/middleware"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubCartService struct {
	addItemFn    func(ctx context.Context, userID, productID string, quantity int32, priceSnapshot int64) error
	removeItemFn func(ctx context.Context, userID string, productID string) error
	updateItemFn func(ctx context.Context, userID string, productID string, quantity int32) error
	getCartFn    func(ctx context.Context, userID string) (*domain.Cart, error)
	clearCartFn  func(ctx context.Context, userID string) error
}

func (s *stubCartService) AddItem(ctx context.Context, userID, productID string, quantity int32, priceSnapshot int64) error {
	if s.addItemFn != nil {
		return s.addItemFn(ctx, userID, productID, quantity, priceSnapshot)
	}
	return nil
}
func (s *stubCartService) RemoveItem(ctx context.Context, userID string, productID string) error {
	if s.removeItemFn != nil {
		return s.removeItemFn(ctx, userID, productID)
	}
	return nil
}
func (s *stubCartService) UpdateItem(ctx context.Context, userID string, productID string, quantity int32) error {
	if s.updateItemFn != nil {
		return s.updateItemFn(ctx, userID, productID, quantity)
	}
	return nil
}
func (s *stubCartService) GetCart(ctx context.Context, userID string) (*domain.Cart, error) {
	if s.getCartFn != nil {
		return s.getCartFn(ctx, userID)
	}
	return &domain.Cart{UserID: userID, Items: []domain.CartItem{}, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}
func (s *stubCartService) ClearCart(ctx context.Context, userID string) error {
	if s.clearCartFn != nil {
		return s.clearCartFn(ctx, userID)
	}
	return nil
}

type stubProductService struct {
	getProductFn func(ctx context.Context, id string) (domain.Product, error)
}

func (s *stubProductService) ListProducts(ctx context.Context, limit, offset int) ([]domain.Product, error) {
	return nil, nil
}
func (s *stubProductService) GetProduct(ctx context.Context, id string) (domain.Product, error) {
	if s.getProductFn != nil {
		return s.getProductFn(ctx, id)
	}
	return domain.Product{ID: id, PriceCents: 500}, nil
}
func (s *stubProductService) StreamProducts(_ context.Context, _, _ int) (gateway.ProductStream, error) {
	return nil, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

const testUserID = "user-uuid-123"

// withUser injects the userID into the request context as the auth middleware would.
func withUser(r *http.Request) *http.Request {
	ctx := middleware.ContextWithUserUUID(r.Context(), testUserID)
	return r.WithContext(ctx)
}

func jsonBody(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

func decodeBody(t *testing.T, body *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return m
}

// ── AddItem ───────────────────────────────────────────────────────────────────

func TestCartHandler_AddItem_OK(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})

	body := jsonBody(map[string]any{"product_id": "prod-1", "quantity": 2})
	req := httptest.NewRequest(http.MethodPost, "/cart/items", body)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.AddItem(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	m := decodeBody(t, rec.Body)
	if m["ok"] != true {
		t.Fatalf("expected ok=true, got %v", m)
	}
}

func TestCartHandler_AddItem_WrongMethod(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	req := httptest.NewRequest(http.MethodGet, "/cart/items", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.AddItem(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestCartHandler_AddItem_NoUser(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	body := jsonBody(map[string]any{"product_id": "prod-1", "quantity": 1})
	req := httptest.NewRequest(http.MethodPost, "/cart/items", body)
	// no withUser → no userID in context
	rec := httptest.NewRecorder()

	h.AddItem(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCartHandler_AddItem_BadBody(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	req := httptest.NewRequest(http.MethodPost, "/cart/items", bytes.NewBufferString("{invalid json"))
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.AddItem(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCartHandler_AddItem_ProductNotFound(t *testing.T) {
	productSvc := &stubProductService{
		getProductFn: func(_ context.Context, _ string) (domain.Product, error) {
			return domain.Product{}, domain.ErrProductNotFound
		},
	}
	h := handlers.NewCartHandler(&stubCartService{}, productSvc)

	body := jsonBody(map[string]any{"product_id": "no-such", "quantity": 1})
	req := httptest.NewRequest(http.MethodPost, "/cart/items", body)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.AddItem(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCartHandler_AddItem_CartServiceError(t *testing.T) {
	cartSvc := &stubCartService{
		addItemFn: func(_ context.Context, _, _ string, _ int32, _ int64) error {
			return errors.New("redis down")
		},
	}
	h := handlers.NewCartHandler(cartSvc, &stubProductService{})

	body := jsonBody(map[string]any{"product_id": "prod-1", "quantity": 1})
	req := httptest.NewRequest(http.MethodPost, "/cart/items", body)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.AddItem(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ── GetCart ───────────────────────────────────────────────────────────────────

func TestCartHandler_GetCart_OK(t *testing.T) {
	now := time.Now()
	cartSvc := &stubCartService{
		getCartFn: func(_ context.Context, userID string) (*domain.Cart, error) {
			return &domain.Cart{
				UserID: userID,
				Items: []domain.CartItem{
					{ProductID: "prod-1", Quantity: 3, PriceSnapshotKopecks: 900, AddedAt: now},
				},
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		},
	}
	h := handlers.NewCartHandler(cartSvc, &stubProductService{})

	req := httptest.NewRequest(http.MethodGet, "/cart", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.GetCart(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	m := decodeBody(t, rec.Body)
	if m["user_id"] != testUserID {
		t.Fatalf("expected user_id=%q, got %v", testUserID, m["user_id"])
	}
	items, ok := m["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 item, got %v", m["items"])
	}
}

func TestCartHandler_GetCart_WrongMethod(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	req := httptest.NewRequest(http.MethodPost, "/cart", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.GetCart(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestCartHandler_GetCart_NoUser(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	req := httptest.NewRequest(http.MethodGet, "/cart", nil)
	rec := httptest.NewRecorder()

	h.GetCart(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCartHandler_GetCart_NotFound(t *testing.T) {
	cartSvc := &stubCartService{
		getCartFn: func(_ context.Context, _ string) (*domain.Cart, error) {
			return nil, domain.ErrCartNotFound
		},
	}
	h := handlers.NewCartHandler(cartSvc, &stubProductService{})

	req := httptest.NewRequest(http.MethodGet, "/cart", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.GetCart(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ── RemoveItem ────────────────────────────────────────────────────────────────

func TestCartHandler_RemoveItem_OK(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	body := jsonBody(map[string]any{"product_id": "prod-1"})
	req := httptest.NewRequest(http.MethodDelete, "/cart/items/remove", body)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.RemoveItem(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCartHandler_RemoveItem_WrongMethod(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	req := httptest.NewRequest(http.MethodGet, "/cart/items/remove", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.RemoveItem(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestCartHandler_RemoveItem_NoUser(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	body := jsonBody(map[string]any{"product_id": "prod-1"})
	req := httptest.NewRequest(http.MethodDelete, "/cart/items/remove", body)
	rec := httptest.NewRecorder()

	h.RemoveItem(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCartHandler_RemoveItem_CartNotFound(t *testing.T) {
	cartSvc := &stubCartService{
		removeItemFn: func(_ context.Context, _ string, _ string) error {
			return domain.ErrCartNotFound
		},
	}
	h := handlers.NewCartHandler(cartSvc, &stubProductService{})
	body := jsonBody(map[string]any{"product_id": "prod-1"})
	req := httptest.NewRequest(http.MethodDelete, "/cart/items/remove", body)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.RemoveItem(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ── UpdateItem ────────────────────────────────────────────────────────────────

func TestCartHandler_UpdateItem_OK(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	body := jsonBody(map[string]any{"product_id": "prod-1", "quantity": 5})
	req := httptest.NewRequest(http.MethodPatch, "/cart/items/update", body)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.UpdateItem(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	m := decodeBody(t, rec.Body)
	if m["ok"] != true {
		t.Fatalf("expected ok=true, got %v", m)
	}
}

func TestCartHandler_UpdateItem_WrongMethod(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	req := httptest.NewRequest(http.MethodPost, "/cart/items/update", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.UpdateItem(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestCartHandler_UpdateItem_NoUser(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	body := jsonBody(map[string]any{"product_id": "prod-1", "quantity": 3})
	req := httptest.NewRequest(http.MethodPatch, "/cart/items/update", body)
	rec := httptest.NewRecorder()

	h.UpdateItem(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCartHandler_UpdateItem_CartNotFound(t *testing.T) {
	cartSvc := &stubCartService{
		updateItemFn: func(_ context.Context, _ string, _ string, _ int32) error {
			return domain.ErrCartNotFound
		},
	}
	h := handlers.NewCartHandler(cartSvc, &stubProductService{})
	body := jsonBody(map[string]any{"product_id": "prod-1", "quantity": 2})
	req := httptest.NewRequest(http.MethodPatch, "/cart/items/update", body)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.UpdateItem(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ── ClearCart ─────────────────────────────────────────────────────────────────

func TestCartHandler_ClearCart_OK(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	req := httptest.NewRequest(http.MethodDelete, "/cart/clear", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.ClearCart(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCartHandler_ClearCart_WrongMethod(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	req := httptest.NewRequest(http.MethodGet, "/cart/clear", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.ClearCart(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestCartHandler_ClearCart_NoUser(t *testing.T) {
	h := handlers.NewCartHandler(&stubCartService{}, &stubProductService{})
	req := httptest.NewRequest(http.MethodDelete, "/cart/clear", nil)
	rec := httptest.NewRecorder()

	h.ClearCart(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCartHandler_ClearCart_CartNotFound(t *testing.T) {
	cartSvc := &stubCartService{
		clearCartFn: func(_ context.Context, _ string) error {
			return domain.ErrCartNotFound
		},
	}
	h := handlers.NewCartHandler(cartSvc, &stubProductService{})
	req := httptest.NewRequest(http.MethodDelete, "/cart/clear", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.ClearCart(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCartHandler_ClearCart_ServiceError(t *testing.T) {
	cartSvc := &stubCartService{
		clearCartFn: func(_ context.Context, _ string) error {
			return errors.New("mongo timeout")
		},
	}
	h := handlers.NewCartHandler(cartSvc, &stubProductService{})
	req := httptest.NewRequest(http.MethodDelete, "/cart/clear", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()

	h.ClearCart(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
