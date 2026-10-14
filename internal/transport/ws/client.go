package ws

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	sendBufferSize = 64
	writeTimeout   = 10 * time.Second
	pingInterval   = 30 * time.Second
	pongTimeout    = 10 * time.Second
	maxMessageSize = 32 << 10 //32KB
)

type Client struct {
	conn      *websocket.Conn
	userID    int64
	username  string
	send      chan []byte
	rooms     map[int64]struct{}
	closeOnce sync.Once
}

func newClient(conn *websocket.Conn, userID int64, username string) *Client {
	return &Client{
		conn:     conn,
		userID:   userID,
		username: username,
		send:     make(chan []byte, sendBufferSize),
		rooms:    make(map[int64]struct{}),
	}
}

func (c *Client) enqueue(msg []byte) bool {
	select {
	case c.send <- msg:
		return true
	default:
		return false
	}
}

func (c *Client) close(code websocket.StatusCode, reason string) {
	c.closeOnce.Do(func() {
		go func() { _ = c.conn.Close(code, reason) }()
	})
}

func (c *Client) sendEvent(eventType string, payload any) {
	data, err := encodeEvent(eventType, payload)
	if err != nil {
		slog.Error("encode websocket event", "type", eventType, "error", err)
		return
	}
	if !c.enqueue(data) {
		c.close(websocket.StatusPolicyViolation, "connection too slow")
	}
}

func (c *Client) sendError(msg string) {
	c.sendEvent(typeError, errorPayload{Message: msg})
}

func (c *Client) writePump(ctx context.Context) error {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-c.send:
			if err := c.write(ctx, msg); err != nil {
				return fmt.Errorf("write: %w", err)
			}
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, pongTimeout)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return fmt.Errorf("ping: %w", err)
			}
		}
	}
}

func (c *Client) write(ctx context.Context, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return c.conn.Write(ctx, websocket.MessageText, msg)
}
