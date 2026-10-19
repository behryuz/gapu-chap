package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type HealthChecker interface {
	Ping(ctx context.Context) error
}

type RouterDeps struct {
	DB         HealthChecker
	Tokens     TokenParser
	Accounts   AccountLoader
	Auth       *AuthHandler
	Users      *UserHandler
	Rooms      *RoomHandler
	Messages   *MessageHandler
	Presence   *PresenceHandler
	Moderation *ModerationHandler
	WS         http.Handler
}

func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()
	authMW := requireAuth(d.Tokens, d.Accounts)

	protected := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, authMW(h))
	}

	// public
	mux.HandleFunc("GET /health", handleHealth(d.DB))
	mux.HandleFunc("POST /api/auth/register", d.Auth.Register)
	mux.HandleFunc("POST /api/auth/login", d.Auth.Login)
	mux.Handle("GET /ws", d.WS)

	// protected
	protected("GET /api/me", d.Users.Me)
	protected("GET /api/me/rooms", d.Rooms.MyRooms)
	protected("GET /api/users", d.Users.Search)
	protected("GET /api/users/online", d.Presence.Online)
	protected("POST /api/dm/{user_id}", d.Rooms.OpenDirect)

	protected("POST /api/moderation/users/{id}/ban", d.Moderation.Ban)
	protected("POST /api/moderation/users/{id}/unban", d.Moderation.Unban)

	protected("POST /api/rooms", d.Rooms.Create)
	protected("GET /api/rooms", d.Rooms.List)
	protected("GET /api/rooms/{id}", d.Rooms.Get)
	protected("DELETE /api/rooms/{id}", d.Rooms.Delete)
	protected("POST /api/rooms/{id}/join", d.Rooms.Join)
	protected("POST /api/rooms/{id}/leave", d.Rooms.Leave)
	protected("GET /api/rooms/{id}/members", d.Rooms.Members)
	protected("POST /api/rooms/{id}/messages", d.Messages.Send)
	protected("GET /api/rooms/{id}/messages", d.Messages.History)
	protected("GET /api/rooms/{id}/messages/search", d.Messages.Search)
	protected("POST /api/rooms/{id}/read", d.Messages.MarkRead)

	return logRequest(recoverPanic(mux))
}

func handleHealth(db HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			slog.Error("health check: database ping failed", "error", err)
			writeError(w, http.StatusServiceUnavailable, "health check: database unavailable")
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
