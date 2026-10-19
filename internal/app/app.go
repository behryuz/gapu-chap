// Package app собирает приложение из слоёв (composition root). Вынесено из main,
// чтобы интеграционные тесты поднимали ровно ту же конфигурацию, что и продакшен.
package app

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"gapu-chap/internal/auth"
	"gapu-chap/internal/repository/postgres"
	"gapu-chap/internal/service"
	httptransport "gapu-chap/internal/transport/http"
	"gapu-chap/internal/transport/ws"
)

type Config struct {
	JWTSecret  string
	JWTTTL     time.Duration
	BcryptCost int
}

type App struct {
	Handler http.Handler
	// Hub нужен снаружи, чтобы при остановке сервера закрыть WebSocket-соединения.
	Hub *ws.Hub
}

func New(pool *pgxpool.Pool, cfg Config) *App {
	userRepo := postgres.NewUserRepository(pool)
	roomRepo := postgres.NewRoomRepository(pool)
	messageRepo := postgres.NewMessageRepository(pool)

	hasher := auth.NewBcryptHasher(cfg.BcryptCost)
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)

	hub := ws.NewHub()

	authService := service.NewAuthService(userRepo, hasher, tokens)
	userService := service.NewUserService(userRepo)
	roomService := service.NewRoomService(roomRepo, hub)
	messageService := service.NewMessageService(messageRepo, roomRepo, roomRepo, hub)
	moderationService := service.NewModerationService(userRepo, hub)

	router := httptransport.NewRouter(httptransport.RouterDeps{
		DB:         pool,
		Tokens:     tokens,
		Accounts:   userService,
		Auth:       httptransport.NewAuthHandler(authService),
		Users:      httptransport.NewUserHandler(userService),
		Rooms:      httptransport.NewRoomHandler(roomService),
		Messages:   httptransport.NewMessageHandler(messageService),
		Presence:   httptransport.NewPresenceHandler(hub),
		Moderation: httptransport.NewModerationHandler(moderationService),
		WS:         ws.NewServer(hub, tokens, userService, messageService, roomService),
	})

	return &App{Handler: router, Hub: hub}
}
