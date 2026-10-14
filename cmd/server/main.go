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

	"gapu-chap/internal/auth"
	"gapu-chap/internal/config"
	"gapu-chap/internal/repository/postgres"
	"gapu-chap/internal/service"
	httptransport "gapu-chap/internal/transport/http"
	"gapu-chap/internal/transport/ws"

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

	userRepo := postgres.NewUserRepository(pool)
	roomRepo := postgres.NewRoomRepository(pool)
	messageRepo := postgres.NewMessageRepository(pool)

	hasher := auth.NewBcryptHasher(bcrypt.DefaultCost)
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)

	hub := ws.NewHub()

	authService := service.NewAuthService(userRepo, hasher, tokens)
	userService := service.NewUserService(userRepo)
	roomService := service.NewRoomService(roomRepo)
	messageService := service.NewMessageService(messageRepo, roomRepo, hub)

	router := httptransport.NewRouter(httptransport.RouterDeps{
		DB:       pool,
		Tokens:   tokens,
		Auth:     httptransport.NewAuthHandler(authService),
		Users:    httptransport.NewUserHandler(userService),
		Rooms:    httptransport.NewRoomHandler(roomService),
		Messages: httptransport.NewMessageHandler(messageService),
		WS:       ws.NewServer(hub, tokens, messageService, roomService),
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}
	srv.RegisterOnShutdown(hub.CloseAll)

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
