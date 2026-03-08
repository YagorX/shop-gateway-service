package gateway

import (
	"context"

	"github.com/YagorX/shop-gateway/internal/domain"
)

type ProductRepository interface {
	List(ctx context.Context, limit, offset int) ([]domain.Product, error)
	GetByID(ctx context.Context, id string) (domain.Product, error)
}
