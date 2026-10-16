package ws

import (
	"cmp"
	"log/slog"
	"slices"
	"sync"

	"github.com/coder/websocket"

	"gapu-chap/internal/domain"
)

type presence struct {
	username string
	conns    int
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
	rooms   map[int64]map[*Client]struct{}
	online  map[int64]*presence
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),
		rooms:   make(map[int64]map[*Client]struct{}),
		online:  make(map[int64]*presence),
	}
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[c] = struct{}{}

	if p, ok := h.online[c.userID]; ok {
		p.conns++
		return
	}

	h.online[c.userID] = &presence{username: c.username, conns: 1}
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

	p := h.online[c.userID]
	p.conns--
	if p.conns > 0 {
		return
	}
	delete(h.online, c.userID)
	h.notifyAllLocked(typePresence, presencePayload{UserID: c.userID, Username: c.username, Online: false})
}

func (h *Hub) subscribe(roomID int64, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

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
	users := make([]domain.OnlineUser, 0, len(h.online))
	for id, p := range h.online {
		users = append(users, domain.OnlineUser{ID: id, Username: p.username})
	}
	h.mu.RUnlock()

	slices.SortFunc(users, func(a, b domain.OnlineUser) int {
		return cmp.Compare(a.Username, b.Username)
	})
	return users
}

func (h *Hub) CloseAll() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.clients {
		c.close(websocket.StatusGoingAway, "server shutting down")
	}
}
