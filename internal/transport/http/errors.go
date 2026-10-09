package http

import (
	"errors"
	"gapu-chap/internal/domain"
	"log/slog"
	"net/http"
)

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, domain.ErrInvalidCredentials.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, domain.ErrNotFound.Error())
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusInternalServerError, domain.ErrConflict.Error())
	default:
		slog.Error("internal server error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
