package gateway

import (
	"context"

	"github.com/YagorX/shop-gateway/internal/domain"
)

type ProductStream interface {
	Recv() (domain.Product, error)
}

type ProductRepository interface {
	List(ctx context.Context, limit, offset int) ([]domain.Product, error)
	GetByID(ctx context.Context, id string) (domain.Product, error)
	Stream(ctx context.Context, limit, offset int) (ProductStream, error)
}

type AuthRepository interface {
	Register(ctx context.Context, username, email, password string) (string, error)
	Login(ctx context.Context, emailOrName, password string, appID int64, deviceID string) (accessToken string, refreshToken string, err error)
	ValidateToken(ctx context.Context, token string, appID int64) (string, error)
	Refresh(ctx context.Context, refreshToken string, appID int64, deviceID string) (accessToken string, newRefreshToken string, err error)
	Logout(ctx context.Context, refreshToken string, appID int64, deviceID string) error
	IsAdmin(ctx context.Context, userUUID string) (bool, error)
}
