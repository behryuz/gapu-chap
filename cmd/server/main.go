package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gapu-chap/internal/app"
	"gapu-chap/internal/config"
	"gapu-chap/internal/repository/postgres"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("application stopped with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()
	slog.Info("database connection established")

	if cfg.MigrateOnStart {
		if err := postgres.Migrate(ctx, cfg.DatabaseURL); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
		slog.Info("database migrations applied")
	}

	application := app.New(pool, app.Config{
		JWTSecret:  cfg.JWTSecret,
		JWTTTL:     cfg.JWTTTL,
		BcryptCost: bcrypt.DefaultCost,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           application.Handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	srv.RegisterOnShutdown(application.Hub.CloseAll)

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("starting http server", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down server")
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("http server error: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	slog.Info("server stopped gracefully")
	return nil
}
