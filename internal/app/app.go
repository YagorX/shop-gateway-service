package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	auth_adapter "github.com/YagorX/shop-gateway/internal/adapters/auth_grpc"
	cart_adapter "github.com/YagorX/shop-gateway/internal/adapters/cart_grpc"
	catalog_adapter "github.com/YagorX/shop-gateway/internal/adapters/catalog_grpc"
	httpapp "github.com/YagorX/shop-gateway/internal/app/httpapp"
	auth_client "github.com/YagorX/shop-gateway/internal/client/grpc/auth"

	cart_client "github.com/YagorX/shop-gateway/internal/client/grpc/cart"
	catalog_client "github.com/YagorX/shop-gateway/internal/client/grpc/catalog"
	"github.com/YagorX/shop-gateway/internal/config"
	"github.com/YagorX/shop-gateway/internal/observability"
	"github.com/YagorX/shop-gateway/internal/ratelimit"
	gateway_srv "github.com/YagorX/shop-gateway/internal/service/gateway"
	_ "github.com/YagorX/shop-gateway/internal/transport/grpc/v1/handlers"
	grpchandlers "github.com/YagorX/shop-gateway/internal/transport/grpc/v1/handlers"
	httpv1 "github.com/YagorX/shop-gateway/internal/transport/http/v1"
	handlershttp "github.com/YagorX/shop-gateway/internal/transport/http/v1/handlers"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type App struct {
	logger            *slog.Logger
	httpApp           *httpapp.App
	grpcCatalogClient *catalog_client.Client
	grpcAuthClient    *auth_client.Client
	grpcCartClient    *cart_client.Client

	shutdownTracing func(context.Context) error

	errCh chan error
}

func New(ctx context.Context, cfg *config.Config) (*App, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	runtimeLogger := observability.NewLogger(observability.LoggerOptions{
		Service: cfg.ServiceName,
		Env:     cfg.Env,
		Version: cfg.Version,
		Level:   cfg.LogLevel,
	})
	observability.SetDefaultLogger(runtimeLogger.Logger)

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	limiter := ratelimit.NewLimiter(redisClient, cfg.RateLimit.Limit, cfg.RateLimit.Window)

	shutdownTracing, err := observability.InitTracing(
		ctx,
		cfg.ServiceName,
		cfg.Version,
		cfg.Env,
		cfg.OTLP.Endpoint,
	)
	if err != nil {
		runtimeLogger.Logger.Warn("tracing is disabled", slog.String("error", err.Error()))
		shutdownTracing = nil
	}

	stopTracing := func() {
		if shutdownTracing != nil {
			_ = shutdownTracing(context.Background())
		}
	}
	var grpc_catalog_client *catalog_client.Client
	var grpc_auth_client *auth_client.Client
	var grpc_cart_client *cart_client.Client

	err = retryWithExponentialBackoff(ctx,
		func() error {
			grpc_catalog_client, err = catalog_client.NewClient(runtimeLogger.Logger, cfg.CatalogGRPC.Addr, cfg.CatalogGRPC.Timeout)
			return err
		},
		cfg.Retry.MaxRetries,
		cfg.Retry.InitialInterval,
		cfg.Retry.MaxInterval,
	)
	if err != nil {
		stopTracing()
		return nil, fmt.Errorf("create grpc catalog client: %w", err)
	}

	err = retryWithExponentialBackoff(ctx,
		func() error {
			grpc_auth_client, err = auth_client.NewClient(runtimeLogger.Logger, cfg.AuthGRPC.Addr, cfg.AuthGRPC.Timeout, cfg.AuthTLS)
			return err
		},
		cfg.Retry.MaxRetries,
		cfg.Retry.InitialInterval,
		cfg.Retry.MaxInterval,
	)
	if err != nil {
		stopTracing()
		_ = grpc_catalog_client.Close()
		return nil, fmt.Errorf("create grpc auth client: %w", err)
	}

	err = retryWithExponentialBackoff(ctx,
		func() error {
			grpc_cart_client, err = cart_client.NewClient(runtimeLogger.Logger, cfg.CartGRPC.Addr, cfg.CartGRPC.Timeout, cfg.CartTLS)
			return err
		},
		cfg.Retry.MaxRetries,
		cfg.Retry.InitialInterval,
		cfg.Retry.MaxInterval,
	)
	if err != nil {
		stopTracing()
		_ = grpc_catalog_client.Close()
		_ = grpc_auth_client.Close()
		return nil, fmt.Errorf("create grpc cart client: %w", err)
	}

	catalog_adapter, err := catalog_adapter.NewRepository(grpc_catalog_client)
	if err != nil {
		stopTracing()
		_ = grpc_catalog_client.Close()
		_ = grpc_auth_client.Close()
		return nil, fmt.Errorf("create catalog_adapter: %w", err)
	}

	auth_adapter, err := auth_adapter.NewRepository(grpc_auth_client)
	if err != nil {
		stopTracing()
		_ = grpc_catalog_client.Close()
		_ = grpc_auth_client.Close()
		return nil, fmt.Errorf("create auth_adapter: %w", err)
	}

	cart_adapter, err := cart_adapter.NewRepository(grpc_cart_client)
	if err != nil {
		stopTracing()
		_ = grpc_catalog_client.Close()
		_ = grpc_auth_client.Close()
		return nil, fmt.Errorf("create cart_adapter: %w", err)
	}

	gatewaySrv, err := gateway_srv.NewGatewayService(runtimeLogger.Logger, catalog_adapter, auth_adapter, cart_adapter)
	if err != nil {
		stopTracing()
		_ = grpc_catalog_client.Close()
		_ = grpc_auth_client.Close()
		return nil, fmt.Errorf("create gateway_service: %w", err)
	}

	swaggerHandler := handlershttp.NewSwaggerHandler(cfg.Swagger.UIPath, cfg.Swagger.RedocPath, cfg.Swagger.SpecPath)
	swaggerSpecDir := ""
	if cfg.Swagger.SpecPath != "" {
		swaggerSpecDir = filepath.Dir(cfg.Swagger.SpecPath)
	}
	statusHandler := handlershttp.NewStatusHandler(cfg.TemplatePath, handlershttp.StatusPageData{
		ServiceName:  cfg.ServiceName,
		Environment:  cfg.Env,
		Version:      cfg.Version,
		HTTPAddress:  cfg.HTTPAddr(),
		HTTPSEnabled: cfg.HTTPTLS.Enabled,
		Status:       "ready",
		Now:          time.Now().String(),
	})

	httpRouter := httpv1.NewRouter(httpv1.RouterDeps{
		LogLevelController: runtimeLogger,
		ReadinessChecker: grpchandlers.MultiReadinessChecker{
			Checkers: []grpchandlers.ReadinessChecker{
				grpchandlers.CatalogHealthChecker{
					Addr:    cfg.CatalogGRPC.Addr,
					Timeout: cfg.CatalogGRPC.Timeout,
				},
				grpchandlers.AuthHealthChecker{
					Addr:    cfg.AuthGRPC.Addr,
					Timeout: cfg.AuthGRPC.Timeout,
					TLS: grpchandlers.TLSConfig{
						Enabled:        cfg.AuthTLS.Enabled,
						CAFile:         cfg.AuthTLS.CAFile,
						ServerName:     cfg.AuthTLS.ServerName,
						ClientCertFile: cfg.AuthTLS.ClientCertFile,
						ClientKeyFile:  cfg.AuthTLS.ClientKeyFile,
					},
				},
				grpchandlers.CartHealthChecker{
					Addr:    cfg.CartGRPC.Addr,
					Timeout: cfg.CartGRPC.Timeout,
					TLS: grpchandlers.TLSConfig{
						Enabled:        cfg.CartTLS.Enabled,
						CAFile:         cfg.CartTLS.CAFile,
						ServerName:     cfg.CartTLS.ServerName,
						ClientCertFile: cfg.CartTLS.ClientCertFile,
						ClientKeyFile:  cfg.CartTLS.ClientKeyFile,
					},
				},
			},
		},
		ProductService: gatewaySrv,
		AuthService:    gatewaySrv,
		CartService:    gatewaySrv,
		SwaggerHandler: swaggerHandler,
		SwaggerSpecDir: swaggerSpecDir,
		StatusHandler:  statusHandler,
		Limiter:        limiter,
	})

	otelHandler := otelhttp.NewHandler(httpRouter, "gateway.http")

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr(),
		Handler:           otelHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	httpRuntime, err := httpapp.New(runtimeLogger.Logger, httpServer, cfg.HTTPTLS.Enabled, cfg.HTTPTLS.CertFile, cfg.HTTPTLS.KeyFile)
	if err != nil {
		stopTracing()
		_ = grpc_catalog_client.Close()
		_ = grpc_auth_client.Close()
		_ = grpc_cart_client.Close()
		return nil, fmt.Errorf("create http app: %w", err)
	}

	return &App{
		logger:            runtimeLogger.Logger,
		httpApp:           httpRuntime,
		shutdownTracing:   shutdownTracing,
		grpcCatalogClient: grpc_catalog_client,
		grpcAuthClient:    grpc_auth_client,
		grpcCartClient:    grpc_cart_client,
		errCh:             make(chan error, 1),
	}, nil

}

func (a *App) Run() error {
	if a == nil {
		return errors.New("app is nil")
	}

	go func() {
		if err := a.httpApp.Run(); err != nil {
			a.errCh <- err
			a.logger.Error("http app failed", slog.String("error", err.Error()))
			close(a.errCh)
		}
	}()

	a.logger.Info("gateway service bootstrap completed",
		slog.String("http_addr", a.httpApp.Addr()),
	)

	return nil
}

func (a *App) Errors() <-chan error {
	return a.errCh
}

func (a *App) Shutdown(ctx context.Context) error {
	if a == nil {
		return nil
	}

	var shutdownErr error

	if a.httpApp != nil {
		if err := a.httpApp.Stop(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("stop http app: %w", err))
		}
	}

	if a.shutdownTracing != nil {
		if err := a.shutdownTracing(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shutdown tracing: %w", err))
		}
	}

	if a.grpcCatalogClient != nil {
		if err := a.grpcCatalogClient.Close(); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("stop grpc client: %w", err))
		}
	}

	if a.grpcAuthClient != nil {
		if err := a.grpcAuthClient.Close(); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("stop grpc auth client: %w", err))
		}
	}

	if a.grpcCartClient != nil {
		if err := a.grpcCartClient.Close(); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("stop grpc cart client: %w", err))
		}
	}

	a.logger.Info("gateway service stopped")

	return shutdownErr
}
