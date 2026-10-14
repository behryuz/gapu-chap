package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/sync/errgroup"

	"gapu-chap/internal/auth"
	"gapu-chap/internal/domain"
)

type TokenParser interface {
	Parse(token string) (*auth.Claims, error)
}

type MessageSender interface {
	Send(ctx context.Context, roomID, authorID int64, body string) (*domain.Message, error)
}

type RoomAccess interface {
	RequireMember(ctx context.Context, roomID, userID int64) error
}

const eventTimeout = 5 * time.Second

type Server struct {
	hub      *Hub
	tokens   TokenParser
	messages MessageSender
	rooms    RoomAccess
}

func NewServer(hub *Hub, tokens TokenParser, messages MessageSender, rooms RoomAccess) *Server {
	return &Server{hub: hub, tokens: tokens, messages: messages, rooms: rooms}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	claims, err := s.tokens.Parse(r.URL.Query().Get("token"))
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		slog.Warn("websocket accept failed", "error", err)
		return
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(maxMessageSize)

	c := newClient(conn, claims.UserID, claims.Username)
	s.hub.register(c)
	defer s.hub.unregister(c)

	slog.Info("websocket connected", "user_id", c.userID)

	g, ctx := errgroup.WithContext(r.Context())
	g.Go(func() error { return s.readLoop(ctx, c) })
	g.Go(func() error { return c.writePump(ctx) })
	err = g.Wait()

	slog.Info("websocket disconnected",
		"user_id", c.userID,
		"close_status", websocket.CloseStatus(err),
		"reason", err,
	)
}

func (s *Server) readLoop(ctx context.Context, c *Client) error {
	for {
		typ, data, err := c.conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		if typ != websocket.MessageText {
			c.sendError("only text messages are supported")
			continue
		}

		var event inboundEvent
		if err := json.Unmarshal(data, &event); err != nil {
			c.sendError("invalid json")
			continue
		}

		s.dispatch(ctx, c, event)
	}
}

func (s *Server) dispatch(ctx context.Context, c *Client, event inboundEvent) {
	ctx, cancel := context.WithTimeout(ctx, eventTimeout)
	defer cancel()

	switch event.Type {
	case typeRoomSubscribe:
		s.handleSubscribe(ctx, c, event.Payload)
	case typeRoomUnsubscribe:
		s.handleUnsubscribe(c, event.Payload)
	case typeMessageSend:
		s.handleSend(ctx, c, event.Payload)
	default:
		c.sendError(fmt.Sprintf("unknown event type %q", event.Type))
	}
}

func (s *Server) handleSubscribe(ctx context.Context, c *Client, raw json.RawMessage) {
	var p roomPayload
	if err := decodePayload(raw, &p); err != nil || p.RoomID <= 0 {
		c.sendError("invalid payload")
		return
	}

	if err := s.rooms.RequireMember(ctx, p.RoomID, c.userID); err != nil {
		c.sendError(publicError(err))
		return
	}

	s.hub.subscribe(p.RoomID, c)
	c.sendEvent(typeRoomSubscribed, p)
}

func (s *Server) handleUnsubscribe(c *Client, raw json.RawMessage) {
	var p roomPayload
	if err := decodePayload(raw, &p); err != nil || p.RoomID <= 0 {
		c.sendError("invalid payload")
		return
	}
	s.hub.unsubscribe(p.RoomID, c)
}

func (s *Server) handleSend(ctx context.Context, c *Client, raw json.RawMessage) {
	var p sendMessagePayload
	if err := decodePayload(raw, &p); err != nil || p.RoomID <= 0 {
		c.sendError("invalid payload")
		return
	}

	if _, err := s.messages.Send(ctx, p.RoomID, c.userID, p.Body); err != nil {
		c.sendError(publicError(err))
	}
}

func publicError(err error) string {
	switch {
	case errors.Is(err, domain.ErrValidation), errors.Is(err, domain.ErrForbidden):
		return err.Error()
	case errors.Is(err, domain.ErrNotFound):
		return domain.ErrNotFound.Error()
	default:
		slog.Error("websocket event failed", "error", err)
		return "internal error"
	}
}
