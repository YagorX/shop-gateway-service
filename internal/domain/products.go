package domain

import "errors"

var (
	ErrProductNotFound   = errors.New("product not found")
	ErrInvalidPagination = errors.New("invalid pagination")
)

type Product struct {
	ID          string
	SKU         string
	Name        string
	Description string
	PriceCents  int64
	Currency    string
	Stock       int32
	Active      bool
}

func (p Product) Validate() error {
	if p.ID == "" {
		return errors.New("product id is required")
	}
	if p.SKU == "" {
		return errors.New("product sku is required")
	}
	if p.Name == "" {
		return errors.New("product name is required")
	}
	if p.PriceCents < 0 {
		return errors.New("product price must be >= 0")
	}
	if p.Currency == "" {
		return errors.New("product currency is required")
	}
	if p.Stock < 0 {
		return errors.New("product stock must be >= 0")
	}
	return nil
}
