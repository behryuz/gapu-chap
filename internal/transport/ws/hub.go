package ws

import (
	"log/slog"
	"sync"

	"github.com/coder/websocket"

	"gapu-chap/internal/domain"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
	rooms   map[int64]map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),
		rooms:   make(map[int64]map[*Client]struct{}),
	}
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[c] = struct{}{}
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.clients, c)
	for roomID := range c.rooms {
		h.removeFromRoomLocked(roomID, c)
	}
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
		if !c.enqueue(msg) {
			c.close(websocket.StatusPolicyViolation, "connection too slow")
		}
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

func (h *Hub) CloseAll() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.clients {
		c.close(websocket.StatusGoingAway, "server shutting down")
	}
}
