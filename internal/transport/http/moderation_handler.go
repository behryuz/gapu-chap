package http

import (
	"context"
	"net/http"
)

type ModerationService interface {
	Ban(ctx context.Context, actorID, targetID int64) error
	Unban(ctx context.Context, actorID, targetID int64) error
}

type ModerationHandler struct {
	moderation ModerationService
}

func NewModerationHandler(moderation ModerationService) *ModerationHandler {
	return &ModerationHandler{moderation: moderation}
}

func (h *ModerationHandler) Ban(w http.ResponseWriter, r *http.Request) {
	h.apply(w, r, h.moderation.Ban)
}

func (h *ModerationHandler) Unban(w http.ResponseWriter, r *http.Request) {
	h.apply(w, r, h.moderation.Unban)
}

func (h *ModerationHandler) apply(w http.ResponseWriter, r *http.Request, action func(ctx context.Context, actorID, targetID int64) error) {
	authUser, ok := authUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	targetID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := action(r.Context(), authUser.ID, targetID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
