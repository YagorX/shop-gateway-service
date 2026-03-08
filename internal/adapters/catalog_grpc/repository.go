package cataloggrpc

import (
	"context"
	"errors"
	"fmt"
	"math"

	catalogv1 "github.com/YagorX/shop-contracts/gen/go/proto/catalog/v1"
	clientcatalog "github.com/YagorX/shop-gateway/internal/client/grpc/catalog"
	"github.com/YagorX/shop-gateway/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gateway "github.com/YagorX/shop-gateway/internal/service/gateway"
)

type CatalogAdapter struct {
	client *clientcatalog.Client
}

var _ gateway.ProductRepository = (*CatalogAdapter)(nil)

func NewRepository(client *clientcatalog.Client) (*CatalogAdapter, error) {
	if client == nil {
		return nil, errors.New("catalog grpc client is nil")
	}
	return &CatalogAdapter{client: client}, nil
}

func (r *CatalogAdapter) List(ctx context.Context, limit, offset int) ([]domain.Product, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("catalog grpc repository is not initialized")
	}
	if limit < 0 || offset < 0 {
		return nil, domain.ErrInvalidPagination
	}
	if limit > math.MaxUint32 || offset > math.MaxUint32 {
		return nil, domain.ErrInvalidPagination
	}

	resp, err := r.client.ListProducts(ctx, uint32(limit), uint32(offset))
	if err != nil {
		return nil, mapGRPCError(err)
	}
	if resp == nil {
		return nil, errors.New("catalog grpc list response is nil")
	}

	items := resp.GetItems()
	products := make([]domain.Product, 0, len(items))
	for _, item := range items {
		products = append(products, toDomainProduct(item))
	}

	return products, nil
}

func (r *CatalogAdapter) GetByID(ctx context.Context, id string) (domain.Product, error) {
	if r == nil || r.client == nil {
		return domain.Product{}, errors.New("catalog grpc repository is not initialized")
	}

	resp, err := r.client.GetProduct(ctx, id)
	if err != nil {
		return domain.Product{}, mapGRPCError(err)
	}
	if resp == nil || resp.GetProduct() == nil {
		return domain.Product{}, domain.ErrProductNotFound
	}

	return toDomainProduct(resp.GetProduct()), nil
}

func toDomainProduct(p *catalogv1.Product) domain.Product {
	if p == nil {
		return domain.Product{}
	}

	stock := int32(p.GetStock())
	if p.GetStock() > math.MaxInt32 {
		stock = math.MaxInt32
	}

	return domain.Product{
		ID:          p.GetId(),
		SKU:         p.GetSku(),
		Name:        p.GetName(),
		Description: p.GetDescription(),
		PriceCents:  int64(math.Round(p.GetPrice() * 100)),
		Currency:    p.GetCurrency(),
		Stock:       stock,
		Active:      p.GetActive(),
	}
}

func mapGRPCError(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("catalog grpc request failed: %w", err)
	}

	switch st.Code() {
	case codes.NotFound:
		return domain.ErrProductNotFound
	case codes.InvalidArgument:
		return domain.ErrInvalidPagination
	default:
		return fmt.Errorf("catalog grpc request failed: %w", err)
	}
}
