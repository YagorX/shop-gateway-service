package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	adapter "github.com/YagorX/shop-gateway/internal/adapters/catalog_grpc"
	httpapp "github.com/YagorX/shop-gateway/internal/app/httpapp"
	catalog_client "github.com/YagorX/shop-gateway/internal/client/grpc/catalog"
	"github.com/YagorX/shop-gateway/internal/config"
	"github.com/YagorX/shop-gateway/internal/observability"
	gateway_srv "github.com/YagorX/shop-gateway/internal/service/gateway"
	grpchandlers "github.com/YagorX/shop-gateway/internal/transport/grpc/v1/handlers"
	httpv1 "github.com/YagorX/shop-gateway/internal/transport/http/v1"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type App struct {
	logger     *slog.Logger
	httpApp    *httpapp.App
	grpcClient *catalog_client.Client

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

	shutdownTracing, err := observability.InitTracing(
		ctx,
		cfg.ServiceName,
		cfg.Version,
		cfg.Env,
		cfg.OTLP.Endpoint,
	)
	if err != nil {
		return nil, fmt.Errorf("init tracing: %w", err)
	}

	grpc_client, err := catalog_client.NewClient(runtimeLogger.Logger, cfg.CatalogGRPC.Addr, cfg.CatalogGRPC.Timeout)
	if err != nil {
		_ = shutdownTracing(context.Background())
		return nil, fmt.Errorf("create grpc client: %w", err)
	}

	catalog_adapter, err := adapter.NewRepository(grpc_client)
	if err != nil {
		_ = shutdownTracing(context.Background())
		return nil, fmt.Errorf("create catalog_adapter: %w", err)
	}

	gatewaySrv, err := gateway_srv.NewGatewayService(runtimeLogger.Logger, catalog_adapter)
	if err != nil {
		_ = shutdownTracing(context.Background())
		return nil, fmt.Errorf("create gateway_service: %w", err)
	}

	httpRouter := httpv1.NewRouter(httpv1.RouterDeps{
		LogLevelController: runtimeLogger,
		ReadinessChecker: grpchandlers.CatalogHealthChecker{
			Addr:    cfg.CatalogGRPC.Addr,
			Timeout: cfg.CatalogGRPC.Timeout,
		},
		ProductService: gatewaySrv,
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

	httpRuntime, err := httpapp.New(runtimeLogger.Logger, httpServer)
	if err != nil {
		_ = shutdownTracing(context.Background())
		return nil, fmt.Errorf("create http app: %w", err)
	}

	return &App{
		logger:          runtimeLogger.Logger,
		httpApp:         httpRuntime,
		shutdownTracing: shutdownTracing,
		grpcClient:      grpc_client,
		errCh:           make(chan error, 1),
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

	if a.grpcClient != nil {
		if err := a.grpcClient.Close(); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("stop grpc client: %w", err))
		}
	}

	a.logger.Info("gateway service stopped")

	return shutdownErr
}
