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
	logger             *slog.Logger
	catalog_repository ProductRepository
	auth_repository    AuthRepository
}

func NewGatewayService(logger *slog.Logger, catalog_repository ProductRepository, auth_repository AuthRepository) (*GatewayService, error) {
	if logger == nil {
		return nil, errors.New("logger is empty")
	}
	if catalog_repository == nil {
		return nil, errors.New("recatalog_repository is empty")
	}
	if auth_repository == nil {
		return nil, errors.New("auth_repository is empty")
	}
	return &GatewayService{logger: logger, catalog_repository: catalog_repository, auth_repository: auth_repository}, nil
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

	products, err := service.catalog_repository.List(ctx, limit, offset)
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

	product, err := service.catalog_repository.GetByID(ctx, id)
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

func (service *GatewayService) StreamProducts(ctx context.Context, limit, offset int) (ProductStream, error) {
	const op = "service.gateway.StreamProducts"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	ctx, span := otel.Tracer("shop-gateway/internal/service/gateway").Start(ctx, op)
	defer span.End()

	defer func() {
		metrics.GatewayServiceRequestDuration.WithLabelValues("StreamProducts").Observe(time.Since(startedAt).Seconds())
	}()

	if err := service.ensureInitialized(); err != nil {
		slog.Error("gateway service is not initialized", slog.String("op", op))
		metrics.GatewayServiceRequestsTotal.WithLabelValues("StreamProducts", "error").Inc()
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
		metrics.GatewayServiceRequestsTotal.WithLabelValues("StreamProducts", "invalid_argument").Inc()
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

	service.logger.Debug("stream products started",
		slog.String("op", op),
		slog.Int("requested_limit", originalLimit),
		slog.Int("requested_offset", originalOffset),
		slog.Int("effective_limit", limit),
		slog.Int("effective_offset", offset),
	)

	stream, err := service.catalog_repository.Stream(ctx, limit, offset)
	if err != nil {
		service.logger.Error("repository stream failed",
			slog.String("op", op),
			slog.Int("limit", originalLimit),
			slog.Int("effective_limit", limit),
			slog.Int("offset", offset),
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		metrics.GatewayServiceRequestsTotal.WithLabelValues("StreamProducts", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	service.logger.Info("stream products initialized",
		slog.String("op", op),
		slog.Int("limit", originalLimit),
		slog.Int("effective_limit", limit),
		slog.Int("offset", offset),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	metrics.GatewayServiceRequestsTotal.WithLabelValues("StreamProducts", "success").Inc()
	span.SetStatus(codes.Ok, "success")

	return stream, nil
}

func (service *GatewayService) Register(ctx context.Context, username, email, password string) (string, error) {
	const op = "service.gateway.Register"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	ctx, span := otel.Tracer("shop-gateway/internal/service/gateway").Start(ctx, op)
	defer span.End()

	defer func() {
		metrics.GatewayServiceRequestDuration.WithLabelValues("Register").Observe(time.Since(startedAt).Seconds())
	}()

	if err := service.ensureInitialized(); err != nil {
		service.logger.Error("gateway service is not initialized", slog.String("op", op))
		metrics.GatewayServiceRequestsTotal.WithLabelValues("Register", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	span.SetAttributes(
		attribute.String("auth.username", username),
		attribute.String("auth.email", email),
	)

	service.logger.Info("register started",
		slog.String("op", op),
		slog.String("username", username),
		slog.String("email", email),
	)

	userUUID, err := service.auth_repository.Register(ctx, username, email, password)
	if err != nil {
		service.logger.Error("register failed",
			slog.String("op", op),
			slog.String("username", username),
			slog.String("email", email),
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		metrics.GatewayServiceRequestsTotal.WithLabelValues("Register", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	service.logger.Info("register completed",
		slog.String("op", op),
		slog.String("username", username),
		slog.String("email", email),
		slog.String("user_uuid", userUUID),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	metrics.GatewayServiceRequestsTotal.WithLabelValues("Register", "success").Inc()
	span.SetAttributes(attribute.String("auth.user_uuid", userUUID))
	span.SetStatus(codes.Ok, "success")

	return userUUID, nil
}

func (service *GatewayService) Login(
	ctx context.Context,
	emailOrName, password string,
	appID int64,
	deviceID string,
) (string, string, error) {
	const op = "service.gateway.Login"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	ctx, span := otel.Tracer("shop-gateway/internal/service/gateway").Start(ctx, op)
	defer span.End()

	defer func() {
		metrics.GatewayServiceRequestDuration.WithLabelValues("Login").Observe(time.Since(startedAt).Seconds())
	}()

	if err := service.ensureInitialized(); err != nil {
		service.logger.Error("gateway service is not initialized", slog.String("op", op))
		metrics.GatewayServiceRequestsTotal.WithLabelValues("Login", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", "", err
	}

	span.SetAttributes(
		attribute.String("auth.email_or_name", emailOrName),
		attribute.Int64("auth.app_id", appID),
		attribute.String("auth.device_id", deviceID),
	)

	service.logger.Info("login started",
		slog.String("op", op),
		slog.String("email_or_name", emailOrName),
		slog.Int64("app_id", appID),
		slog.String("device_id", deviceID),
	)

	accessToken, refreshToken, err := service.auth_repository.Login(ctx, emailOrName, password, appID, deviceID)
	if err != nil {
		service.logger.Error("login failed",
			slog.String("op", op),
			slog.String("email_or_name", emailOrName),
			slog.Int64("app_id", appID),
			slog.String("device_id", deviceID),
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		metrics.GatewayServiceRequestsTotal.WithLabelValues("Login", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", "", err
	}

	service.logger.Info("login completed",
		slog.String("op", op),
		slog.String("email_or_name", emailOrName),
		slog.Int64("app_id", appID),
		slog.String("device_id", deviceID),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	metrics.GatewayServiceRequestsTotal.WithLabelValues("Login", "success").Inc()
	span.SetStatus(codes.Ok, "success")

	return accessToken, refreshToken, nil
}

func (service *GatewayService) ValidateToken(ctx context.Context, token string, appID int64) (string, error) {
	const op = "service.gateway.ValidateToken"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	ctx, span := otel.Tracer("shop-gateway/internal/service/gateway").Start(ctx, op)
	defer span.End()

	defer func() {
		metrics.GatewayServiceRequestDuration.WithLabelValues("ValidateToken").Observe(time.Since(startedAt).Seconds())
	}()

	if err := service.ensureInitialized(); err != nil {
		service.logger.Error("gateway service is not initialized", slog.String("op", op))
		metrics.GatewayServiceRequestsTotal.WithLabelValues("ValidateToken", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	span.SetAttributes(attribute.Int64("auth.app_id", appID))

	service.logger.Info("validate token started",
		slog.String("op", op),
		slog.Int64("app_id", appID),
	)

	userUUID, err := service.auth_repository.ValidateToken(ctx, token, appID)
	if err != nil {
		service.logger.Error("validate token failed",
			slog.String("op", op),
			slog.Int64("app_id", appID),
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		metrics.GatewayServiceRequestsTotal.WithLabelValues("ValidateToken", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	service.logger.Info("validate token completed",
		slog.String("op", op),
		slog.String("user_uuid", userUUID),
		slog.Int64("app_id", appID),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	metrics.GatewayServiceRequestsTotal.WithLabelValues("ValidateToken", "success").Inc()
	span.SetAttributes(attribute.String("auth.user_uuid", userUUID))
	span.SetStatus(codes.Ok, "success")

	return userUUID, nil
}

func (service *GatewayService) Refresh(
	ctx context.Context,
	refreshToken string,
	appID int64,
	deviceID string,
) (string, string, error) {
	const op = "service.gateway.Refresh"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	ctx, span := otel.Tracer("shop-gateway/internal/service/gateway").Start(ctx, op)
	defer span.End()

	defer func() {
		metrics.GatewayServiceRequestDuration.WithLabelValues("Refresh").Observe(time.Since(startedAt).Seconds())
	}()

	if err := service.ensureInitialized(); err != nil {
		service.logger.Error("gateway service is not initialized", slog.String("op", op))
		metrics.GatewayServiceRequestsTotal.WithLabelValues("Refresh", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", "", err
	}

	span.SetAttributes(
		attribute.Int64("auth.app_id", appID),
		attribute.String("auth.device_id", deviceID),
	)

	service.logger.Info("refresh started",
		slog.String("op", op),
		slog.Int64("app_id", appID),
		slog.String("device_id", deviceID),
	)

	accessToken, newRefreshToken, err := service.auth_repository.Refresh(ctx, refreshToken, appID, deviceID)
	if err != nil {
		service.logger.Error("refresh failed",
			slog.String("op", op),
			slog.Int64("app_id", appID),
			slog.String("device_id", deviceID),
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		metrics.GatewayServiceRequestsTotal.WithLabelValues("Refresh", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", "", err
	}

	service.logger.Info("refresh completed",
		slog.String("op", op),
		slog.Int64("app_id", appID),
		slog.String("device_id", deviceID),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	metrics.GatewayServiceRequestsTotal.WithLabelValues("Refresh", "success").Inc()
	span.SetStatus(codes.Ok, "success")

	return accessToken, newRefreshToken, nil
}

func (service *GatewayService) Logout(ctx context.Context, refreshToken string, appID int64, deviceID string) error {
	const op = "service.gateway.Logout"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	ctx, span := otel.Tracer("shop-gateway/internal/service/gateway").Start(ctx, op)
	defer span.End()

	defer func() {
		metrics.GatewayServiceRequestDuration.WithLabelValues("Logout").Observe(time.Since(startedAt).Seconds())
	}()

	if err := service.ensureInitialized(); err != nil {
		service.logger.Error("gateway service is not initialized", slog.String("op", op))
		metrics.GatewayServiceRequestsTotal.WithLabelValues("Logout", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(
		attribute.Int64("auth.app_id", appID),
		attribute.String("auth.device_id", deviceID),
	)

	service.logger.Info("logout started",
		slog.String("op", op),
		slog.Int64("app_id", appID),
		slog.String("device_id", deviceID),
	)

	err := service.auth_repository.Logout(ctx, refreshToken, appID, deviceID)
	if err != nil {
		service.logger.Error("logout failed",
			slog.String("op", op),
			slog.Int64("app_id", appID),
			slog.String("device_id", deviceID),
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		metrics.GatewayServiceRequestsTotal.WithLabelValues("Logout", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	service.logger.Info("logout completed",
		slog.String("op", op),
		slog.Int64("app_id", appID),
		slog.String("device_id", deviceID),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	metrics.GatewayServiceRequestsTotal.WithLabelValues("Logout", "success").Inc()
	span.SetStatus(codes.Ok, "success")

	return nil
}

func (service *GatewayService) IsAdmin(ctx context.Context, userUUID string) (bool, error) {
	const op = "service.gateway.IsAdmin"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	ctx, span := otel.Tracer("shop-gateway/internal/service/gateway").Start(ctx, op)
	defer span.End()

	defer func() {
		metrics.GatewayServiceRequestDuration.WithLabelValues("IsAdmin").Observe(time.Since(startedAt).Seconds())
	}()

	if err := service.ensureInitialized(); err != nil {
		service.logger.Error("gateway service is not initialized", slog.String("op", op))
		metrics.GatewayServiceRequestsTotal.WithLabelValues("IsAdmin", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}

	span.SetAttributes(attribute.String("auth.user_uuid", userUUID))

	service.logger.Info("is admin started",
		slog.String("op", op),
		slog.String("user_uuid", userUUID),
	)

	isAdmin, err := service.auth_repository.IsAdmin(ctx, userUUID)
	if err != nil {
		service.logger.Error("is admin failed",
			slog.String("op", op),
			slog.String("user_uuid", userUUID),
			slog.String("error", err.Error()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
		metrics.GatewayServiceRequestsTotal.WithLabelValues("IsAdmin", "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}

	service.logger.Info("is admin completed",
		slog.String("op", op),
		slog.String("user_uuid", userUUID),
		slog.Bool("is_admin", isAdmin),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	)
	metrics.GatewayServiceRequestsTotal.WithLabelValues("IsAdmin", "success").Inc()
	span.SetAttributes(attribute.Bool("auth.is_admin", isAdmin))
	span.SetStatus(codes.Ok, "success")

	return isAdmin, nil
}

func (service *GatewayService) ensureInitialized() error {
	if service == nil || service.catalog_repository == nil || service.auth_repository == nil || service.logger == nil {
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
