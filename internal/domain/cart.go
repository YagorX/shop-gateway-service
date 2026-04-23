package domain

import (
	"errors"
	"time"
)

var ErrCartNotFound = errors.New("cart not found")

type CartItem struct {
	ProductID            string
	Quantity             int32
	PriceSnapshotKopecks int64
	AddedAt              time.Time
}

type Cart struct {
	UserID    string
	Items     []CartItem
	CreatedAt time.Time
	UpdatedAt time.Time
}
