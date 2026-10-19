package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"gapu-chap/internal/domain"
)

type RoomService interface {
	Create(ctx context.Context, ownerID int64, name string) (*domain.Room, error)
	List(ctx context.Context) ([]domain.Room, error)
	Get(ctx context.Context, roomID, userID int64) (*domain.Room, error)
	Delete(ctx context.Context, roomID, actorID int64) error
	Join(ctx context.Context, roomID, userID int64) error
	Leave(ctx context.Context, roomID, userID int64) error
	Members(ctx context.Context, roomID, userID int64) ([]domain.Member, error)
	OpenDirect(ctx context.Context, userID, peerID int64) (*domain.Room, bool, error)
	MyRooms(ctx context.Context, userID int64) ([]domain.UserRoom, error)
}

type RoomHandler struct {
	rooms RoomService
}

func NewRoomHandler(rooms RoomService) *RoomHandler {
	return &RoomHandler{rooms: rooms}
}

type createRoomRequest struct {
	Name string `json:"name"`
}

type roomResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	OwnerID   int64     `json:"owner_id"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}

type peerResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type roomListResponse struct {
	Rooms []roomResponse `json:"rooms"`
}

type userRoomResponse struct {
	roomResponse
	LastReadMessageID int64         `json:"last_read_message_id"`
	UnreadCount       int64         `json:"unread_count"`
	Peer              *peerResponse `json:"peer,omitempty"`
}

type userRoomListResponse struct {
	Rooms []userRoomResponse `json:"rooms"`
}

type memberResponse struct {
	UserID            int64     `json:"user_id"`
	Username          string    `json:"username"`
	FirstName         string    `json:"first_name"`
	LastName          string    `json:"last_name,omitempty"`
	JoinedAt          time.Time `json:"joined_at"`
	LastReadMessageID int64     `json:"last_read_message_id"`
}

type memberListResponse struct {
	Members []memberResponse `json:"members"`
}

func toRoomResponse(r *domain.Room) roomResponse {
	return roomResponse{ID: r.ID, Name: r.Name, OwnerID: r.OwnerID, Kind: string(r.Kind), CreatedAt: r.CreatedAt}
}

func (h *RoomHandler) Create(w http.ResponseWriter, r *http.Request) {
	authUser, ok := authUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createRoomRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	room, err := h.rooms.Create(r.Context(), authUser.ID, req.Name)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "room name already taken")
			return
		}
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toRoomResponse(room))
}

func (h *RoomHandler) List(w http.ResponseWriter, r *http.Request) {
	rooms, err := h.rooms.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	resp := roomListResponse{Rooms: make([]roomResponse, 0, len(rooms))}
	for i := range rooms {
		resp.Rooms = append(resp.Rooms, toRoomResponse(&rooms[i]))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *RoomHandler) Get(w http.ResponseWriter, r *http.Request) {
	authUser, ok := authUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	roomID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	room, err := h.rooms.Get(r.Context(), roomID, authUser.ID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRoomResponse(room))
}

func (h *RoomHandler) Delete(w http.ResponseWriter, r *http.Request) {
	authUser, ok := authUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	roomID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.rooms.Delete(r.Context(), roomID, authUser.ID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RoomHandler) Join(w http.ResponseWriter, r *http.Request) {
	authUser, ok := authUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	roomID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.rooms.Join(r.Context(), roomID, authUser.ID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RoomHandler) Leave(w http.ResponseWriter, r *http.Request) {
	authUser, ok := authUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	roomID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.rooms.Leave(r.Context(), roomID, authUser.ID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RoomHandler) Members(w http.ResponseWriter, r *http.Request) {
	authUser, ok := authUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	roomID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	members, err := h.rooms.Members(r.Context(), roomID, authUser.ID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	resp := memberListResponse{Members: make([]memberResponse, 0, len(members))}
	for _, m := range members {
		resp.Members = append(resp.Members, memberResponse{
			UserID:            m.UserID,
			Username:          m.Username,
			FirstName:         m.FirstName,
			LastName:          m.LastName,
			JoinedAt:          m.JoinedAt,
			LastReadMessageID: m.LastReadMessageID,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *RoomHandler) MyRooms(w http.ResponseWriter, r *http.Request) {
	authUser, ok := authUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rooms, err := h.rooms.MyRooms(r.Context(), authUser.ID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	resp := userRoomListResponse{Rooms: make([]userRoomResponse, 0, len(rooms))}
	for i := range rooms {
		item := userRoomResponse{
			roomResponse:      toRoomResponse(&rooms[i].Room),
			LastReadMessageID: rooms[i].LastReadMessageID,
			UnreadCount:       rooms[i].UnreadCount,
		}
		if peer := rooms[i].Peer; peer != nil {
			item.Peer = &peerResponse{ID: peer.ID, Username: peer.Username}
		}
		resp.Rooms = append(resp.Rooms, item)
	}
	writeJSON(w, http.StatusOK, resp)
}

// OpenDirect — POST /api/dm/{user_id}: 201, если диалог создан сейчас, 200 — если уже был.
func (h *RoomHandler) OpenDirect(w http.ResponseWriter, r *http.Request) {
	authUser, ok := authUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	peerID, err := pathID(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	room, created, err := h.rooms.OpenDirect(r.Context(), authUser.ID, peerID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, toRoomResponse(room))
}
