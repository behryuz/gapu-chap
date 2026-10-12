package http

import (
	"errors"
	"log/slog"
	"net/http"

	"gapu-chap/internal/domain"
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
		writeError(w, http.StatusConflict, domain.ErrConflict.Error())
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		slog.Error("internal server error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
