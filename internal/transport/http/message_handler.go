package http

import (
	"context"
	"net/http"
	"time"

	"gapu-chap/internal/domain"
	"gapu-chap/internal/service"
)

type MessageService interface {
	Send(ctx context.Context, roomID, authorID int64, body string) (*domain.Message, error)
	History(ctx context.Context, roomID, userID int64, q service.HistoryQuery) ([]domain.Message, error)
	MarkRead(ctx context.Context, roomID, userID, messageID int64) error
	Search(ctx context.Context, roomID, userID int64, query string, q service.HistoryQuery) ([]domain.Message, error)
}

type MessageHandler struct {
	messages MessageService
}

func NewMessageHandler(messages MessageService) *MessageHandler {
	return &MessageHandler{messages: messages}
}

type sendMessageRequest struct {
	Body string `json:"body"`
}

type markReadRequest struct {
	MessageID int64 `json:"message_id"`
}

type authorResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type messageResponse struct {
	ID        int64          `json:"id"`
	RoomID    int64          `json:"room_id"`
	Author    authorResponse `json:"author"`
	Body      string         `json:"body"`
	CreatedAt time.Time      `json:"created_at"`
}

type messageListResponse struct {
	Messages   []messageResponse `json:"messages"`
	NextBefore *int64            `json:"next_before,omitempty"`
}

func toMessageResponse(m *domain.Message) messageResponse {
	return messageResponse{
		ID:        m.ID,
		RoomID:    m.RoomID,
		Author:    authorResponse{ID: m.AuthorID, Username: m.AuthorUsername},
		Body:      m.Body,
		CreatedAt: m.CreatedAt,
	}
}

func (h *MessageHandler) Send(w http.ResponseWriter, r *http.Request) {
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

	var req sendMessageRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	msg, err := h.messages.Send(r.Context(), roomID, authUser.ID, req.Body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toMessageResponse(msg))
}

func (h *MessageHandler) History(w http.ResponseWriter, r *http.Request) {
	h.listMessages(w, r, func(ctx context.Context, roomID, userID int64, q service.HistoryQuery) ([]domain.Message, error) {
		return h.messages.History(ctx, roomID, userID, q)
	})
}

// Search — GET /api/rooms/{id}/messages/search?q=...&before=&limit=
func (h *MessageHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	h.listMessages(w, r, func(ctx context.Context, roomID, userID int64, q service.HistoryQuery) ([]domain.Message, error) {
		return h.messages.Search(ctx, roomID, userID, query, q)
	})
}

type listFunc func(ctx context.Context, roomID, userID int64, q service.HistoryQuery) ([]domain.Message, error)

// listMessages — общая часть истории и поиска: разбор пути, курсора и лимита, формат ответа.
func (h *MessageHandler) listMessages(w http.ResponseWriter, r *http.Request, list listFunc) {
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

	before, err := queryInt64(r, "before")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := queryInt64(r, "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	messages, err := list(r.Context(), roomID, authUser.ID, service.HistoryQuery{
		BeforeID: before,
		Limit:    int(min(limit, 1000)),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	resp := messageListResponse{Messages: make([]messageResponse, 0, len(messages))}
	for i := range messages {
		resp.Messages = append(resp.Messages, toMessageResponse(&messages[i]))
	}

	if len(messages) > 0 {
		last := messages[len(messages)-1].ID
		resp.NextBefore = &last
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *MessageHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
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

	var req markReadRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.messages.MarkRead(r.Context(), roomID, authUser.ID, req.MessageID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
