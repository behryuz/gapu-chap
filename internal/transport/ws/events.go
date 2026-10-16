package ws

import (
	"bytes"
	"encoding/json"
	"time"

	"gapu-chap/internal/domain"
)

const (
	typeRoomSubscribe   = "room.subscribe"
	typeRoomUnsubscribe = "room.unsubscribe"
	typeMessageSend     = "message.send"
)

const (
	typeRoomSubscribed = "room.subscribed"
	typeRoomLeft       = "room.left"
	typeRoomDeleted    = "room.deleted"
	typeMessageNew     = "message.new"
	typePresence       = "presence"
	typeError          = "error"
)

type inboundEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type outboundEvent struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type roomPayload struct {
	RoomID int64 `json:"room_id"`
}

type sendMessagePayload struct {
	RoomID int64  `json:"room_id"`
	Body   string `json:"body"`
}

type presencePayload struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Online   bool   `json:"online"`
}

type errorPayload struct {
	Message string `json:"message"`
}

type authorPayload struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type messagePayload struct {
	ID        int64         `json:"id"`
	RoomID    int64         `json:"room_id"`
	Author    authorPayload `json:"author"`
	Body      string        `json:"body"`
	CreatedAt time.Time     `json:"created_at"`
}

func toMessagePayload(m *domain.Message) messagePayload {
	return messagePayload{
		ID:        m.ID,
		RoomID:    m.RoomID,
		Author:    authorPayload{ID: m.AuthorID, Username: m.AuthorUsername},
		Body:      m.Body,
		CreatedAt: m.CreatedAt,
	}
}

func encodeEvent(eventType string, payload any) ([]byte, error) {
	return json.Marshal(outboundEvent{Type: eventType, Payload: payload})
}

func decodePayload(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
