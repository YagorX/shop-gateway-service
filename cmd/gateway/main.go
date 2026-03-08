package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/YagorX/shop-gateway/internal/app"
	"github.com/YagorX/shop-gateway/internal/config"
)

func fetchConfigPath() string {
	var path string

	flag.StringVar(&path, "config", "", "path to config file")
	flag.Parse()

	if path == "" {
		path = os.Getenv("CONFIG_PATH")
	}

	return path
}

func main() {
	configPath := fetchConfigPath()
	if configPath == "" {
		log.Fatal("config path is empty: use --config or CONFIG_PATH")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	application, err := app.New(context.Background(), cfg)
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}

	if err := application.Run(); err != nil {
		log.Fatalf("failed to run app: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-application.Errors():
		if err != nil {
			slog.Error("application runtime error", slog.String("error", err.Error()))
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := application.Shutdown(shutdownCtx); err != nil {
		slog.Error("application shutdown failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("gateway stopped")
}
