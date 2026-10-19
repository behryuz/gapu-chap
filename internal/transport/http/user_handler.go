package http

import (
	"context"
	"net/http"
	"time"

	"gapu-chap/internal/domain"
)

type UserService interface {
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	Search(ctx context.Context, query string) ([]domain.User, error)
}

type UserHandler struct {
	userService UserService
}

func NewUserHandler(userService UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	authUser, ok := authUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.userService.GetByID(r.Context(), authUser.ID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toUserResponse(user))
}

type userResponse struct {
	ID        int64     `json:"id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name,omitempty"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	IsBanned  bool      `json:"is_banned"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:        u.ID,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Username:  u.Username,
		Role:      string(u.Role),
		IsBanned:  u.IsBanned(),
		CreatedAt: u.CreatedAt,
	}
}

type userListResponse struct {
	Users []userResponse `json:"users"`
}

func (h *UserHandler) Search(w http.ResponseWriter, r *http.Request) {
	users, err := h.userService.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	resp := userListResponse{Users: make([]userResponse, 0, len(users))}
	for i := range users {
		resp.Users = append(resp.Users, toUserResponse(&users[i]))
	}
	writeJSON(w, http.StatusOK, resp)
}
