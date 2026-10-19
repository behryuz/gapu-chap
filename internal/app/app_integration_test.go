package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"gapu-chap/internal/app"
	"gapu-chap/internal/testutil/testdb"
)

// Сквозные тесты: настоящий HTTP-сервер + настоящий PostgreSQL + WebSocket-клиент.
// Проверяют требования задания целиком, через публичный API — так, как их увидит клиент.

type env struct {
	t      *testing.T
	server *httptest.Server
	pool   *pgxpool.Pool
}

func newEnv(t *testing.T) *env {
	t.Helper()
	t.Parallel()
	pool := testdb.New(t)
	application := app.New(pool, app.Config{
		JWTSecret:  strings.Repeat("s", 32),
		JWTTTL:     time.Hour,
		BcryptCost: 4, // минимальная стоимость: тестам не нужна защита от перебора
	})
	server := httptest.NewServer(application.Handler)
	t.Cleanup(func() {
		application.Hub.CloseAll()
		server.Close()
	})
	return &env{t: t, server: server, pool: pool}
}

type user struct {
	env      *env
	id       int64
	username string
	token    string
}

func (e *env) register(username string) *user {
	e.t.Helper()
	resp := e.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"first_name": "Test", "username": username, "password": "password123",
	})
	resp.expect(http.StatusCreated)

	u := &user{env: e, username: username, id: int64(resp.json()["id"].(float64))}
	u.token = e.login(username).expect(http.StatusOK).json()["token"].(string)
	return u
}

func (e *env) login(username string) *response {
	return e.do(http.MethodPost, "/api/auth/login", "", map[string]string{
		"username": username, "password": "password123",
	})
}

func (u *user) do(method, path string, body any) *response {
	return u.env.do(method, path, u.token, body)
}

type response struct {
	t      *testing.T
	status int
	body   []byte
}

func (e *env) do(method, path, token string, body any) *response {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, e.server.URL+path, reader)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := e.server.Client().Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	data, _ := io.ReadAll(res.Body)
	return &response{t: e.t, status: res.StatusCode, body: data}
}

func (r *response) expect(status int) *response {
	r.t.Helper()
	if r.status != status {
		r.t.Fatalf("expected status %d, got %d: %s", status, r.status, r.body)
	}
	return r
}

func (r *response) json() map[string]any {
	r.t.Helper()
	var v map[string]any
	if err := json.Unmarshal(r.body, &v); err != nil {
		r.t.Fatalf("decode %s: %v", r.body, err)
	}
	return v
}

func (u *user) createRoom(name string) int64 {
	u.env.t.Helper()
	return int64(u.do(http.MethodPost, "/api/rooms", map[string]string{"name": name}).
		expect(http.StatusCreated).json()["id"].(float64))
}

// --- WebSocket ---

type wsClient struct {
	t    *testing.T
	conn *websocket.Conn
}

func (u *user) connect() *wsClient {
	u.env.t.Helper()
	url := "ws" + strings.TrimPrefix(u.env.server.URL, "http") + "/ws?token=" + u.token
	conn, _, err := websocket.Dial(u.env.t.Context(), url, nil)
	if err != nil {
		u.env.t.Fatalf("dial websocket: %v", err)
	}
	u.env.t.Cleanup(func() { _ = conn.CloseNow() })
	return &wsClient{t: u.env.t, conn: conn}
}

func (c *wsClient) send(eventType string, payload any) {
	c.t.Helper()
	data, _ := json.Marshal(map[string]any{"type": eventType, "payload": payload})
	if err := c.conn.Write(c.t.Context(), websocket.MessageText, data); err != nil {
		c.t.Fatalf("ws write: %v", err)
	}
}

type event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// waitFor читает события, пропуская посторонние (presence и т.п.), пока не встретит нужный тип.
func (c *wsClient) waitFor(eventType string) event {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(c.t.Context(), 3*time.Second)
	defer cancel()
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			c.t.Fatalf("waiting for %q: %v", eventType, err)
		}
		var ev event
		if err := json.Unmarshal(data, &ev); err != nil {
			c.t.Fatalf("decode event: %v", err)
		}
		if ev.Type == eventType {
			return ev
		}
	}
}

// --- тесты ---

func TestE2E_AuthorComesFromToken(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.register("alice"), e.register("bob")
	roomID := alice.createRoom("general")
	bob.do(http.MethodPost, fmt.Sprintf("/api/rooms/%d/join", roomID), nil).expect(http.StatusNoContent)

	path := fmt.Sprintf("/api/rooms/%d/messages", roomID)
	// Попытка написать от чужого имени: лишнее поле отклоняется целиком.
	bob.do(http.MethodPost, path, map[string]any{"body": "hi", "author_id": alice.id}).
		expect(http.StatusBadRequest)

	msg := bob.do(http.MethodPost, path, map[string]any{"body": "hi"}).expect(http.StatusCreated).json()
	author := msg["author"].(map[string]any)
	if int64(author["id"].(float64)) != bob.id || author["username"] != "bob" {
		t.Errorf("author must be taken from the token, got %v", author)
	}

	// Без токена писать нельзя.
	e.do(http.MethodPost, path, "", map[string]any{"body": "anonymous"}).expect(http.StatusUnauthorized)
}

func TestE2E_RoomPermissions(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.register("alice"), e.register("bob")
	roomID := alice.createRoom("general")
	room := fmt.Sprintf("/api/rooms/%d", roomID)

	bob.do(http.MethodPost, room+"/messages", map[string]any{"body": "hi"}).expect(http.StatusForbidden)
	bob.do(http.MethodGet, room+"/messages", nil).expect(http.StatusForbidden)

	bob.do(http.MethodPost, room+"/join", nil).expect(http.StatusNoContent)
	bob.do(http.MethodDelete, room, nil).expect(http.StatusForbidden)
	alice.do(http.MethodPost, room+"/leave", nil).expect(http.StatusForbidden)

	alice.do(http.MethodDelete, room, nil).expect(http.StatusNoContent)
	bob.do(http.MethodGet, room, nil).expect(http.StatusNotFound)
}

func TestE2E_RealtimeMessagesAndReads(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.register("alice"), e.register("bob")
	roomID := alice.createRoom("general")
	bob.do(http.MethodPost, fmt.Sprintf("/api/rooms/%d/join", roomID), nil).expect(http.StatusNoContent)

	aliceWS, bobWS := alice.connect(), bob.connect()
	for _, c := range []*wsClient{aliceWS, bobWS} {
		c.send("room.subscribe", map[string]any{"room_id": roomID})
		c.waitFor("room.subscribed")
	}

	alice.do(http.MethodPost, fmt.Sprintf("/api/rooms/%d/messages", roomID), map[string]any{"body": "через REST"}).
		expect(http.StatusCreated)
	var msg struct {
		ID     int64 `json:"id"`
		Author struct{ Username string }
		Body   string
	}
	_ = json.Unmarshal(bobWS.waitFor("message.new").Payload, &msg)
	if msg.Body != "через REST" || msg.Author.Username != "alice" {
		t.Fatalf("unexpected message: %+v", msg)
	}

	bobWS.send("message.read", map[string]any{"room_id": roomID, "message_id": msg.ID})
	var read struct {
		UserID    int64 `json:"user_id"`
		MessageID int64 `json:"message_id"`
	}
	_ = json.Unmarshal(aliceWS.waitFor("message.read").Payload, &read)
	if read.UserID != bob.id || read.MessageID != msg.ID {
		t.Errorf("alice must see that bob read the message, got %+v", read)
	}

	rooms := bob.do(http.MethodGet, "/api/me/rooms", nil).expect(http.StatusOK).json()["rooms"].([]any)
	if unread := rooms[0].(map[string]any)["unread_count"]; unread != float64(0) {
		t.Errorf("expected 0 unread after reading, got %v", unread)
	}
}

func TestE2E_DirectMessages(t *testing.T) {
	e := newEnv(t)
	alice, bob, carol := e.register("alice"), e.register("bob"), e.register("carol")
	bobWS := bob.connect()

	created := alice.do(http.MethodPost, fmt.Sprintf("/api/dm/%d", bob.id), nil).expect(http.StatusCreated).json()
	dmID := int64(created["id"].(float64))
	if created["kind"] != "direct" {
		t.Errorf("expected direct room, got %v", created["kind"])
	}

	// Собеседник онлайн — его сразу подписали на новый диалог, сообщение придёт без перезагрузки.
	bobWS.waitFor("room.joined")
	alice.do(http.MethodPost, fmt.Sprintf("/api/rooms/%d/messages", dmID), map[string]any{"body": "лично"}).
		expect(http.StatusCreated)
	bobWS.waitFor("message.new")

	reopened := bob.do(http.MethodPost, fmt.Sprintf("/api/dm/%d", alice.id), nil).expect(http.StatusOK).json()
	if int64(reopened["id"].(float64)) != dmID {
		t.Errorf("both sides must share one conversation")
	}

	// Посторонний не видит диалог, не может вступить и прочитать.
	dm := fmt.Sprintf("/api/rooms/%d", dmID)
	carol.do(http.MethodGet, dm, nil).expect(http.StatusNotFound)
	carol.do(http.MethodPost, dm+"/join", nil).expect(http.StatusNotFound)
	carol.do(http.MethodGet, dm+"/members", nil).expect(http.StatusNotFound)
	carol.do(http.MethodGet, dm+"/messages", nil).expect(http.StatusForbidden)

	rooms := bob.do(http.MethodGet, "/api/me/rooms", nil).expect(http.StatusOK).json()["rooms"].([]any)
	peer := rooms[0].(map[string]any)["peer"].(map[string]any)
	if peer["username"] != "alice" {
		t.Errorf("bob must see alice as the peer, got %v", peer)
	}

	alice.do(http.MethodPost, fmt.Sprintf("/api/dm/%d", alice.id), nil).expect(http.StatusBadRequest)
}

func TestE2E_Search(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice")
	roomID := alice.createRoom("general")
	path := fmt.Sprintf("/api/rooms/%d/messages", roomID)
	for _, body := range []string{"Встреча в пятницу", "пятничный релиз", "понедельник"} {
		alice.do(http.MethodPost, path, map[string]any{"body": body}).expect(http.StatusCreated)
	}

	found := alice.do(http.MethodGet, path+"/search?q=пятн", nil).expect(http.StatusOK).json()["messages"].([]any)
	if len(found) != 2 {
		t.Errorf("expected 2 results, got %d", len(found))
	}
	alice.do(http.MethodGet, path+"/search?q=%20", nil).expect(http.StatusBadRequest)
}

func TestE2E_ModeratorBansUser(t *testing.T) {
	e := newEnv(t)
	moderator, bob, carol := e.register("moderator"), e.register("bob"), e.register("carol")
	if _, err := e.pool.Exec(t.Context(), `UPDATE users SET role = 'moderator' WHERE id = $1`, moderator.id); err != nil {
		t.Fatalf("promote: %v", err)
	}
	ban := func(target *user) string { return fmt.Sprintf("/api/moderation/users/%d/ban", target.id) }

	carol.do(http.MethodPost, ban(bob), nil).expect(http.StatusForbidden)

	bobWS := bob.connect()
	moderator.do(http.MethodPost, ban(bob), nil).expect(http.StatusNoContent)

	// Открытое соединение закрывается сразу, с кодом «заблокирован».
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for {
		if _, _, err := bobWS.conn.Read(ctx); err != nil {
			if code := websocket.CloseStatus(err); code != 4003 {
				t.Fatalf("expected close code 4003, got %v (%v)", code, err)
			}
			break
		}
	}

	// Старый токен больше не работает, войти заново нельзя.
	bob.do(http.MethodGet, "/api/me", nil).expect(http.StatusForbidden)
	e.login("bob").expect(http.StatusForbidden)
	url := "ws" + strings.TrimPrefix(e.server.URL, "http") + "/ws?token=" + bob.token
	if _, _, err := websocket.Dial(t.Context(), url, nil); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("banned user must not reconnect, got %v", err)
	}

	moderator.do(http.MethodPost, fmt.Sprintf("/api/moderation/users/%d/unban", bob.id), nil).
		expect(http.StatusNoContent)
	e.login("bob").expect(http.StatusOK)
}
