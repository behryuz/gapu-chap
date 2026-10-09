package http

import (
	"context"
	"errors"
	"gapu-chap/internal/domain"
	"gapu-chap/internal/service"
	"net/http"
	"time"
)

type AuthService interface {
	Register(ctx context.Context, in service.RegisterInput) (*domain.User, error)
	Login(ctx context.Context, username, password string) (string, error)
}

type AuthHandler struct {
	auth AuthService
}

func NewAuthHandler(auth AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

type registerRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userResponse struct {
	ID        int64     `json:"id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name,omitempty"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

type tokenResponse struct {
	Token string `json:"token"`
}

func toUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:        u.ID,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Username:  u.Username,
		CreatedAt: u.CreatedAt,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.auth.Register(r.Context(), service.RegisterInput{
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Username:  req.Username,
		Password:  req.Password,
	})
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "username already taken")
			return
		}
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toUserResponse(user))
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token, err := h.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{Token: token})
}
