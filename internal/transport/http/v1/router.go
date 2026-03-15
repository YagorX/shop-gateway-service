package v1

import (
	"net/http"

	"github.com/YagorX/shop-gateway/internal/transport/http/v1/contracts"
	"github.com/YagorX/shop-gateway/internal/transport/http/v1/handlers"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type RouterDeps struct {
	LogLevelController contracts.LogLevelController
	ReadinessChecker   contracts.ReadinessChecker
	ProductService     contracts.ProductService
	AuthService        contracts.AuthService
}

func NewRouter(deps RouterDeps) http.Handler {
	mux := http.NewServeMux()

	healthHandler := handlers.NewHealthHandler(deps.ReadinessChecker)
	adminHandler := handlers.NewAdminHandler(deps.LogLevelController)
	productsHandler := handlers.NewProductsHandler(deps.ProductService)
	authHandler := handlers.NewAuthHandler(deps.AuthService)

	mux.HandleFunc("/health", healthHandler.Health)
	mux.HandleFunc("/ready", healthHandler.Ready)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/admin/log-level", adminHandler.LogLevel)
	mux.HandleFunc("/products", productsHandler.List)
	mux.HandleFunc("/products/", productsHandler.GetByID)

	mux.HandleFunc("/auth/register", authHandler.Register)
	mux.HandleFunc("/auth/login", authHandler.Login)
	mux.HandleFunc("/auth/validate", authHandler.Validate)
	mux.HandleFunc("/auth/refresh", authHandler.Refresh)
	mux.HandleFunc("/auth/logout", authHandler.Logout)
	mux.HandleFunc("/auth/is-admin", authHandler.IsAdmin)

	return mux
}
