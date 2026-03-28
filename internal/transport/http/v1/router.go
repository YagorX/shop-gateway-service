package v1

import (
	"log/slog"
	"net/http"

	"github.com/YagorX/shop-gateway/internal/transport/http/v1/contracts"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/handlers"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type RouterDeps struct {
	LogLevelController contracts.LogLevelController
	ReadinessChecker   contracts.ReadinessChecker
	ProductService     contracts.ProductService
	AuthService        contracts.AuthService
	SwaggerHandler     *handlers.SwaggerHandler
	StatusHandler      *handlers.StatusHandler
}

func NewRouter(deps RouterDeps) http.Handler {
	mux := http.NewServeMux()
	logger := slog.Default()
	if deps.LogLevelController != nil && deps.LogLevelController.GetSlog() != nil {
		logger = deps.LogLevelController.GetSlog()
	}
	authMiddleware := middleware.Auth(logger, deps.AuthService)

	healthHandler := handlers.NewHealthHandler(deps.ReadinessChecker)
	adminHandler := handlers.NewAdminHandler(deps.LogLevelController)
	productsHandler := handlers.NewProductsHandler(deps.ProductService)
	authHandler := handlers.NewAuthHandler(deps.AuthService)

	mux.HandleFunc("/health", healthHandler.Health)
	mux.HandleFunc("/ready", healthHandler.Ready)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/admin/log-level", adminHandler.LogLevel)
	mux.Handle(
		"/products",
		authMiddleware(http.HandlerFunc(productsHandler.List)),
	)

	mux.Handle(
		"/products/stream",
		authMiddleware(http.HandlerFunc(productsHandler.StreamProducts)),
	)

	mux.Handle(
		"/products/",
		authMiddleware(http.HandlerFunc(productsHandler.GetByID)),
	)

	mux.HandleFunc("/auth/register", authHandler.Register)
	mux.HandleFunc("/auth/login", authHandler.Login)
	mux.HandleFunc("/auth/validate", authHandler.Validate)
	mux.HandleFunc("/auth/refresh", authHandler.Refresh)
	mux.HandleFunc("/auth/logout", authHandler.Logout)
	mux.Handle(
		"/auth/is-admin",
		authMiddleware(http.HandlerFunc(authHandler.IsAdmin)),
	)

	if deps.SwaggerHandler != nil {
		mux.HandleFunc("/swagger/", deps.SwaggerHandler.UI)
		mux.HandleFunc("/swagger/openapi.yaml", deps.SwaggerHandler.OpenAPI)
	}

	mux.Handle(
		"/swagger/spec/",
		http.StripPrefix("/swagger/spec/", http.FileServer(http.Dir("/app/openapi"))),
	)

	mux.HandleFunc("/status", deps.StatusHandler.Page)

	return middleware.Chain(mux,
		middleware.Recovery(logger))
}
