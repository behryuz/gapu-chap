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

func NewRouter(db HealthChecker, auth *AuthHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handleHealth(db))

	mux.HandleFunc("POST /api/auth/register", auth.Register)
	mux.HandleFunc("POST /api/auth/login", auth.Login)

	return mux
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
