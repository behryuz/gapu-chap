package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"gapu-chap/internal/domain"
)

// statusClientClosedRequest — нестандартный код (как в nginx): клиент закрыл соединение
// раньше, чем мы ответили. Это не сбой сервера, поэтому не 500 и не ERROR в логе.
const statusClientClosedRequest = 499

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		w.WriteHeader(statusClientClosedRequest)
	case errors.Is(err, domain.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, domain.ErrInvalidCredentials.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, domain.ErrNotFound.Error())
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, domain.ErrConflict.Error())
	case errors.Is(err, domain.ErrBanned):
		writeError(w, http.StatusForbidden, domain.ErrBanned.Error())
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		slog.Error("internal server error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
