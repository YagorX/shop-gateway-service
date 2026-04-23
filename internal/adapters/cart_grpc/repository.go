package cart_grpc

import (
	"context"
	"errors"
	"fmt"

	cart_client "github.com/YagorX/shop-gateway/internal/client/grpc/cart"
	"github.com/YagorX/shop-gateway/internal/domain"
	gateway "github.com/YagorX/shop-gateway/internal/service/gateway"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// compile-time check: Repository implements gateway.CartRepository
var _ gateway.CartRepository = (*Repository)(nil)

type Repository struct {
	client *cart_client.Client
}

func NewRepository(client *cart_client.Client) (*Repository, error) {
	if client == nil {
		return nil, fmt.Errorf("cart grpc client is nil")
	}

	return &Repository{
		client: client,
	}, nil
}

func (r *Repository) AddItem(ctx context.Context, userID, productID string, quantity int32, priceSnapshot int64) error {
	_, err := r.client.AddItem(ctx, userID, productID, quantity, priceSnapshot)
	return mapGRPCError(err)
}

func (r *Repository) RemoveItem(ctx context.Context, userID string, productID string) error {
	_, err := r.client.RemoveItem(ctx, userID, productID)
	return mapGRPCError(err)
}

func (r *Repository) UpdateItem(ctx context.Context, userID string, productID string, quantity int32) error {
	_, err := r.client.UpdateItem(ctx, userID, productID, quantity)
	return mapGRPCError(err)
}

func (r *Repository) GetCart(ctx context.Context, userID string) (*domain.Cart, error) {
	resp, err := r.client.GetCart(ctx, userID)
	if err != nil {
		return nil, mapGRPCError(err)
	}

	items := make([]domain.CartItem, len(resp.GetItems()))
	for i, item := range resp.GetItems() {
		items[i] = domain.CartItem{
			ProductID:            item.GetProductId(),
			Quantity:             item.GetQuantity(),
			PriceSnapshotKopecks: item.GetPriceSnapshot(),
			AddedAt:              item.GetAddedAt().AsTime(),
		}
	}

	return &domain.Cart{
		UserID: userID,
		Items:  items,
	}, nil
}

func (r *Repository) ClearCart(ctx context.Context, userID string) error {
	_, err := r.client.ClearCart(ctx, userID)
	return mapGRPCError(err)
}

func mapGRPCError(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("cart grpc request failed: %w", err)
	}
	switch st.Code() {
	case codes.NotFound:
		return domain.ErrCartNotFound
	case codes.InvalidArgument:
		return errors.New(st.Message())
	default:
		return fmt.Errorf("cart grpc request failed: %w", err)
	}
}
