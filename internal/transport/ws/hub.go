package ws

import (
	"cmp"
	"log/slog"
	"slices"
	"sync"

	"github.com/coder/websocket"

	"gapu-chap/internal/domain"
)

// userConns — все соединения одного пользователя (несколько вкладок/устройств).
// Пользователь онлайн, пока у него есть хотя бы одно соединение.
type userConns struct {
	username string
	clients  map[*Client]struct{}
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
	rooms   map[int64]map[*Client]struct{}
	users   map[int64]*userConns
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),
		rooms:   make(map[int64]map[*Client]struct{}),
		users:   make(map[int64]*userConns),
	}
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[c] = struct{}{}

	if uc, ok := h.users[c.userID]; ok {
		uc.clients[c] = struct{}{}
		return
	}
	h.users[c.userID] = &userConns{username: c.username, clients: map[*Client]struct{}{c: {}}}
	h.notifyAllLocked(typePresence, presencePayload{UserID: c.userID, Username: c.username, Online: true})
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[c]; !ok {
		return
	}
	delete(h.clients, c)
	for roomID := range c.rooms {
		h.removeFromRoomLocked(roomID, c)
	}

	uc := h.users[c.userID]
	delete(uc.clients, c)
	if len(uc.clients) > 0 {
		return
	}
	delete(h.users, c.userID)
	h.notifyAllLocked(typePresence, presencePayload{UserID: c.userID, Username: c.username, Online: false})
}

func (h *Hub) subscribe(roomID int64, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.subscribeLocked(roomID, c)
}

func (h *Hub) subscribeLocked(roomID int64, c *Client) {
	if h.rooms[roomID] == nil {
		h.rooms[roomID] = make(map[*Client]struct{})
	}
	h.rooms[roomID][c] = struct{}{}
	c.rooms[roomID] = struct{}{}
}

func (h *Hub) unsubscribe(roomID int64, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.removeFromRoomLocked(roomID, c)
}

func (h *Hub) removeFromRoomLocked(roomID int64, c *Client) {
	delete(h.rooms[roomID], c)
	if len(h.rooms[roomID]) == 0 {
		delete(h.rooms, roomID)
	}
	delete(c.rooms, roomID)
}

func (h *Hub) broadcast(roomID int64, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.rooms[roomID] {
		c.deliver(msg)
	}
}

func (h *Hub) notifyAllLocked(eventType string, payload any) {
	data, err := encodeEvent(eventType, payload)
	if err != nil {
		slog.Error("encode websocket event", "type", eventType, "error", err)
		return
	}
	for c := range h.clients {
		c.deliver(data)
	}
}

func (h *Hub) PublishMessage(msg *domain.Message) {
	data, err := encodeEvent(typeMessageNew, toMessagePayload(msg))
	if err != nil {
		slog.Error("encode message event", "message_id", msg.ID, "error", err)
		return
	}
	h.broadcast(msg.RoomID, data)
}

func (h *Hub) PublishRead(r domain.ReadReceipt) {
	data, err := encodeEvent(typeMessageRead, readPayload{RoomID: r.RoomID, UserID: r.UserID, MessageID: r.MessageID})
	if err != nil {
		slog.Error("encode websocket event", "type", typeMessageRead, "error", err)
		return
	}
	h.broadcast(r.RoomID, data)
}

// MemberJoined подписывает все соединения пользователя на комнату и сообщает им об этом:
// после вступления (в том числе с другой вкладки) и при создании личного диалога.
func (h *Hub) MemberJoined(roomID, userID int64) {
	data, err := encodeEvent(typeRoomJoined, roomPayload{RoomID: roomID})
	if err != nil {
		slog.Error("encode websocket event", "type", typeRoomJoined, "error", err)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	uc, ok := h.users[userID]
	if !ok {
		return
	}
	for c := range uc.clients {
		h.subscribeLocked(roomID, c)
		c.deliver(data)
	}
}

func (h *Hub) MemberLeft(roomID, userID int64) {
	data, err := encodeEvent(typeRoomLeft, roomPayload{RoomID: roomID})
	if err != nil {
		slog.Error("encode websocket event", "type", typeRoomLeft, "error", err)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.rooms[roomID] {
		if c.userID == userID {
			h.removeFromRoomLocked(roomID, c)
			c.deliver(data)
		}
	}
}

func (h *Hub) RoomDeleted(roomID int64) {
	data, err := encodeEvent(typeRoomDeleted, roomPayload{RoomID: roomID})
	if err != nil {
		slog.Error("encode websocket event", "type", typeRoomDeleted, "error", err)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.rooms[roomID] {
		delete(c.rooms, roomID)
		c.deliver(data)
	}
	delete(h.rooms, roomID)
}

func (h *Hub) OnlineUsers() []domain.OnlineUser {
	h.mu.RLock()
	users := make([]domain.OnlineUser, 0, len(h.users))
	for id, uc := range h.users {
		users = append(users, domain.OnlineUser{ID: id, Username: uc.username})
	}
	h.mu.RUnlock()

	slices.SortFunc(users, func(a, b domain.OnlineUser) int {
		return cmp.Compare(a.Username, b.Username)
	})
	return users
}

// StatusBanned — код закрытия из диапазона приложений (4000–4999): клиент по нему
// понимает, что переподключаться бессмысленно.
const StatusBanned websocket.StatusCode = 4003

func (h *Hub) DisconnectUser(userID int64) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if uc, ok := h.users[userID]; ok {
		for c := range uc.clients {
			c.close(StatusBanned, "banned")
		}
	}
}

func (h *Hub) CloseAll() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.clients {
		c.close(websocket.StatusGoingAway, "server shutting down")
	}
}
