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
