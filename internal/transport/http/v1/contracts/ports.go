package contracts

import (
	"context"
	"log/slog"

	"github.com/YagorX/shop-gateway/internal/domain"
)

type LogLevelController interface {
	SetLevel(level string) error
	Level() slog.Level
}

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

type ReadinessChecker interface {
	Check(ctx context.Context) error
}

type ProductService interface {
	ListProducts(ctx context.Context, limit, offset int) ([]domain.Product, error)
	GetProduct(ctx context.Context, id string) (domain.Product, error)
}

type AuthService interface {
	Register(ctx context.Context, username, email, password string) (string, error)
	Login(ctx context.Context, emailOrName, password string, appID int64, deviceID string) (accessToken string, refreshToken string, err error)
	ValidateToken(ctx context.Context, token string, appID int64) (string, error)
	Refresh(ctx context.Context, refreshToken string, appID int64, deviceID string) (accessToken string, newRefreshToken string, err error)
	Logout(ctx context.Context, refreshToken string, appID int64, deviceID string) error
	IsAdmin(ctx context.Context, userUUID string) (bool, error)
}
