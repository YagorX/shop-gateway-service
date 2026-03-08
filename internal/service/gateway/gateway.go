package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/YagorX/shop-gateway/internal/domain"
	"github.com/YagorX/shop-gateway/internal/observability"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	defaultListLimit = 20
	maxListLimit     = 100
)

var (
	errServiceNotInitialized = errors.New("gateway service is not initialized")
	errProductIDRequired     = errors.New("product id is required")
)

type GatewayService struct {
	logger     *slog.Logger
	repository ProductRepository
}

func NewGatewayService(logger *slog.Logger, repository ProductRepository) (*GatewayService, error) {
	if logger == nil {
		return nil, errors.New("logger is empty")
	}
	if repository == nil {
		return nil, errors.New("repository is empty")
	}
	return &GatewayService{logger: logger, repository: repository}, nil
}

func (service *GatewayService) ListProducts(ctx context.Context, limit, offset int) ([]domain.Product, error) {
	const op = "service.gateway.ListProducts"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	ctx, span := otel.Tracer("shop-gateway/internal/service/gateway").Start(ctx, op)
	defer span.End()

	defer func() {
		metrics.GatewayServiceRequestDuration.WithLabelValues("ListProducts").Observe(time.Since(startedAt).Seconds())
	}()

	if err := service.ensureInitialized(); err != nil {
		slog.Error("gateway service is not initialized", slog.String("op", op))
		metrics.GatewayServiceRequestsTotal.WithLabelValues("ListProducts", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	originalLimit, originalOffset := limit, offset
	limit, offset, err := normalizePagination(limit, offset)
	if err != nil {
		service.logger.Warn("invalid pagination",
			slog.String("op", op),
			slog.Int("limit", originalLimit),
			slog.Int("offset", originalOffset),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		metrics.GatewayServiceRequestsTotal.WithLabelValues("ListProducts", "invalid_argument").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetAttributes(
		attribute.Int("product.limit", originalLimit),
		attribute.Int("product.offset", originalOffset),
		attribute.Int("product.effective_limit", limit),
		attribute.Int("product.effective_offset", offset),
	)

	service.logger.Debug("list products started",
		slog.String("op", op),
		slog.Int("requested_limit", originalLimit),
		slog.Int("requested_offset", originalOffset),
		slog.Int("effective_limit", limit),
		slog.Int("effective_offset", offset),
	)

	products, err := service.repository.List(ctx, limit, offset)
	if err != nil {
		service.logger.Error("repository list failed",
			slog.String("op", op),
			slog.Int("limit", originalLimit),
			slog.Int("effective_limit", limit),
			slog.Int("offset", offset),
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		metrics.GatewayServiceRequestsTotal.WithLabelValues("ListProducts", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	for i, p := range products {
		if err := p.Validate(); err != nil {
			validationErr := fmt.Errorf("invalid product at index %d: %w", i, err)
			service.logger.Error("invalid product data returned by repository",
				slog.String("op", op),
				slog.Int("index", i),
				slog.String("error", validationErr.Error()),
				slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			)
			metrics.GatewayServiceRequestsTotal.WithLabelValues("ListProducts", "error").Inc()
			span.RecordError(validationErr)
			span.SetStatus(codes.Error, validationErr.Error())
			return nil, validationErr
		}
	}

	service.logger.Info("list products completed",
		slog.String("op", op),
		slog.Int("limit", originalLimit),
		slog.Int("effective_limit", limit),
		slog.Int("offset", offset),
		slog.Int("result_count", len(products)),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	metrics.GatewayServiceRequestsTotal.WithLabelValues("ListProducts", "success").Inc()
	span.SetAttributes(attribute.Int("product.result_count", len(products)))
	span.SetStatus(codes.Ok, "success")

	return products, nil
}

func (service *GatewayService) GetProduct(ctx context.Context, id string) (domain.Product, error) {
	const op = "service.gateway.GetProduct"
	startedAt := time.Now()
	metrics := observability.MustMetrics()

	ctx, span := otel.Tracer("shop-gateway/internal/service/gateway").Start(ctx, op)
	defer span.End()

	defer func() {
		metrics.GatewayServiceRequestDuration.WithLabelValues("GetProduct").Observe(time.Since(startedAt).Seconds())
	}()

	if err := service.ensureInitialized(); err != nil {
		slog.Error("gateway service is not initialized", slog.String("op", op))
		metrics.GatewayServiceRequestsTotal.WithLabelValues("GetProduct", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return domain.Product{}, err
	}

	id = strings.TrimSpace(id)
	span.SetAttributes(attribute.String("product.id", id))

	service.logger.Debug("get product started",
		slog.String("op", op),
		slog.String("id", id),
	)

	if id == "" {
		service.logger.Warn("product id is required",
			slog.String("op", op),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		metrics.GatewayServiceRequestsTotal.WithLabelValues("GetProduct", "invalid_argument").Inc()
		span.RecordError(errProductIDRequired)
		span.SetStatus(codes.Error, errProductIDRequired.Error())
		return domain.Product{}, errProductIDRequired
	}

	product, err := service.repository.GetByID(ctx, id)
	if err != nil {
		level := slog.LevelError
		msg := "repository get product failed"
		status := "error"
		if errors.Is(err, domain.ErrProductNotFound) {
			level = slog.LevelWarn
			msg = "product not found"
		}
		service.logger.Log(ctx, level, msg,
			slog.String("op", op),
			slog.String("product_id", id),
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		if errors.Is(err, domain.ErrProductNotFound) {
			status = "not_found"
		}
		metrics.GatewayServiceRequestsTotal.WithLabelValues("GetProduct", status).Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return domain.Product{}, err
	}

	if err := product.Validate(); err != nil {
		validationErr := fmt.Errorf("invalid product returned by repository: %w", err)
		service.logger.Error("repository returned invalid product",
			slog.String("op", op),
			slog.String("product_id", id),
			slog.String("error", validationErr.Error()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		metrics.GatewayServiceRequestsTotal.WithLabelValues("GetProduct", "error").Inc()
		span.RecordError(validationErr)
		span.SetStatus(codes.Error, validationErr.Error())
		return domain.Product{}, validationErr
	}

	service.logger.Info("get product completed",
		slog.String("op", op),
		slog.String("product_id", id),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	metrics.GatewayServiceRequestsTotal.WithLabelValues("GetProduct", "success").Inc()

	span.SetAttributes(attribute.String("product.result_id", product.ID))
	span.SetStatus(codes.Ok, "success")

	return product, nil
}

func (service *GatewayService) ensureInitialized() error {
	if service == nil || service.repository == nil || service.logger == nil {
		return errServiceNotInitialized
	}
	return nil
}

func normalizePagination(limit, offset int) (int, int, error) {
	if offset < 0 {
		return 0, 0, domain.ErrInvalidPagination
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	return limit, offset, nil
}
