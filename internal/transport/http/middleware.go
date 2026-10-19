package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"gapu-chap/internal/auth"
	"gapu-chap/internal/domain"
)

type TokenParser interface {
	Parse(token string) (*auth.Claims, error)
}

type AccountLoader interface {
	GetByID(ctx context.Context, id int64) (*domain.User, error)
}

// requireAuth проверяет подпись и срок токена, а затем — сам аккаунт в базе.
// Один быстрый запрос по первичному ключу на каждый запрос — цена того, что бан
// и снятие роли модератора действуют сразу, а не когда истечёт токен.
func requireAuth(tokens TokenParser, accounts AccountLoader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || raw == "" {
				unauthorized(w, "missing or malformed auth header")
				return
			}

			claims, err := tokens.Parse(raw)
			if err != nil {
				unauthorized(w, "invalid or expired token")
				return
			}

			account, err := accounts.GetByID(r.Context(), claims.UserID)
			switch {
			case errors.Is(err, domain.ErrNotFound):
				unauthorized(w, "account not found")
				return
			case err != nil:
				writeServiceError(w, err)
				return
			case account.IsBanned():
				writeError(w, http.StatusForbidden, domain.ErrBanned.Error())
				return
			}

			ctx := withAuthUser(r.Context(), AuthUser{
				ID:       account.ID,
				Username: account.Username,
				Role:     account.Role,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, msg)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Unwrap() http.ResponseWriter {
	return rec.ResponseWriter
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(&rec, r)

		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds())
	})
}

func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.Error("panic recovered",
					"panic", v,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}
