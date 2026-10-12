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
	DB     HealthChecker
	Tokens TokenParser
	Auth   *AuthHandler
	Users  *UserHandler
	Rooms  *RoomHandler
}

func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()
	authMW := requireAuth(d.Tokens)

	protected := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, authMW(h))
	}

	// public
	mux.HandleFunc("GET /health", handleHealth(d.DB))
	mux.HandleFunc("POST /api/auth/register", d.Auth.Register)
	mux.HandleFunc("POST /api/auth/login", d.Auth.Login)

	// protected
	protected("GET /api/me", d.Users.Me)

	protected("POST /api/rooms", d.Rooms.Create)
	protected("GET /api/rooms", d.Rooms.List)
	protected("GET /api/rooms/{id}", d.Rooms.Get)
	protected("DELETE /api/rooms/{id}", d.Rooms.Delete)
	protected("POST /api/rooms/{id}/join", d.Rooms.Join)
	protected("POST /api/rooms/{id}/leave", d.Rooms.Leave)
	protected("GET /api/rooms/{id}/members", d.Rooms.Members)

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
