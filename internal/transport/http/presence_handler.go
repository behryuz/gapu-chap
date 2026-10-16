package http

import (
	"net/http"

	"gapu-chap/internal/domain"
)

type PresenceReader interface {
	OnlineUsers() []domain.OnlineUser
}

type PresenceHandler struct {
	presence PresenceReader
}

func NewPresenceHandler(presence PresenceReader) *PresenceHandler {
	return &PresenceHandler{presence: presence}
}

type onlineUserResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type onlineUserListResponse struct {
	Users []onlineUserResponse `json:"users"`
}

func (h *PresenceHandler) Online(w http.ResponseWriter, _ *http.Request) {
	users := h.presence.OnlineUsers()

	resp := onlineUserListResponse{Users: make([]onlineUserResponse, 0, len(users))}
	for _, u := range users {
		resp.Users = append(resp.Users, onlineUserResponse{ID: u.ID, Username: u.Username})
	}
	writeJSON(w, http.StatusOK, resp)
}
