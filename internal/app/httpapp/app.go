package httpapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

type App struct {
	log        *slog.Logger
	server     *http.Server
	tlsEnabled bool
	certFile   string
	keyFile    string
}

func New(
	log *slog.Logger,
	server *http.Server,
	tlsEnabled bool,
	certFile string,
	keyFile string,
) (*App, error) {
	if log == nil {
		return nil, fmt.Errorf("logger is nil")
	}
	if server == nil {
		return nil, fmt.Errorf("http server is nil")
	}
	if server.Addr == "" {
		return nil, fmt.Errorf("http server addr is empty")
	}
	if tlsEnabled {
		if certFile == "" {
			return nil, fmt.Errorf("certFile is empty")
		}
		if keyFile == "" {
			return nil, fmt.Errorf("keyFile is empty")
		}
	}

	return &App{
		log:        log,
		server:     server,
		tlsEnabled: tlsEnabled,
		certFile:   certFile,
		keyFile:    keyFile,
	}, nil
}

func (a *App) Run() error {
	const op = "httpapp.Run"

	log := a.log.With(
		slog.String("op", op),
		slog.String("addr", a.server.Addr),
	)

	if a.tlsEnabled {
		if err := a.server.ListenAndServeTLS(a.certFile, a.keyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("%s: %w", op, err)
		}
		log.Info("https server started")
		return nil
	}

	if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%s: %w", op, err)
	}
	log.Info("http server started")

	return nil
}

func (a *App) Stop(ctx context.Context) error {
	const op = "httpapp.Stop"

	a.log.With(
		slog.String("op", op),
		slog.String("addr", a.server.Addr),
	).Info("stopping http server")

	if err := a.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (a *App) Addr() string {
	if a == nil || a.server == nil {
		return ""
	}
	return a.server.Addr
}
