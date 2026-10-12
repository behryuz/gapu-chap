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
}

func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()
	authMW := requireAuth(d.Tokens)

	mux.HandleFunc("GET /health", handleHealth(d.DB))
	mux.HandleFunc("POST /api/auth/register", d.Auth.Register)
	mux.HandleFunc("POST /api/auth/login", d.Auth.Login)

	mux.Handle("GET /api/me", authMW(http.HandlerFunc(d.Users.Me)))

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
