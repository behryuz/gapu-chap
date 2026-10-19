package ws

import (
	"encoding/json"
	"sync"
	"testing"

	"gapu-chap/internal/domain"
)

// Клиент без реального соединения. Буфер большой, чтобы в тестах
// никогда не сработало закрытие «медленного клиента» (conn здесь nil).
func newTestClient(userID int64) *Client {
	return &Client{
		userID:   userID,
		username: "user",
		send:     make(chan []byte, 1024),
		rooms:    make(map[int64]struct{}),
	}
}

// drain забирает всё, что накопилось в очереди клиента, и возвращает типы событий.
func drain(t *testing.T, c *Client) []string {
	t.Helper()

	var types []string
	for {
		select {
		case data := <-c.send:
			var ev struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(data, &ev); err != nil {
				types = append(types, string(data))
				continue
			}
			types = append(types, ev.Type)
		default:
			return types
		}
	}
}

func TestHub_BroadcastOnlyToSubscribers(t *testing.T) {
	h := NewHub()
	alice, bob, carol := newTestClient(1), newTestClient(2), newTestClient(3)
	for _, c := range []*Client{alice, bob, carol} {
		h.register(c)
	}
	h.subscribe(1, alice)
	h.subscribe(1, bob)
	h.subscribe(2, carol)
	for _, c := range []*Client{alice, bob, carol} {
		drain(t, c)
	}

	h.broadcast(1, []byte("hello"))

	if len(alice.send) != 1 || len(bob.send) != 1 {
		t.Errorf("room 1 subscribers must receive the message: alice=%d bob=%d", len(alice.send), len(bob.send))
	}
	if len(carol.send) != 0 {
		t.Errorf("subscriber of another room must not receive the message, got %d", len(carol.send))
	}
}

func TestHub_UnsubscribeAndUnregister(t *testing.T) {
	h := NewHub()
	c := newTestClient(1)
	h.register(c)
	h.subscribe(1, c)
	h.subscribe(2, c)
	drain(t, c)

	h.unsubscribe(1, c)
	h.broadcast(1, []byte("hello"))
	if len(c.send) != 0 {
		t.Error("unsubscribed client must not receive messages")
	}

	h.unregister(c)
	if len(h.clients) != 0 {
		t.Errorf("expected no clients, got %d", len(h.clients))
	}
	if len(h.rooms) != 0 {
		t.Errorf("empty rooms must be removed, got %d", len(h.rooms))
	}
}

func TestHub_Presence_MultipleConnections(t *testing.T) {
	h := NewHub()
	observer := newTestClient(100)
	h.register(observer)
	drain(t, observer)

	tab1, tab2 := newTestClient(1), newTestClient(1)

	h.register(tab1)
	h.register(tab2)
	if got := drain(t, observer); len(got) != 1 || got[0] != typePresence {
		t.Fatalf("expected exactly one presence event for two tabs, got %v", got)
	}
	if n := len(h.OnlineUsers()); n != 2 {
		t.Fatalf("expected 2 online users, got %d", n)
	}

	h.unregister(tab1)
	if got := drain(t, observer); len(got) != 0 {
		t.Fatalf("user still has an open tab, expected no events, got %v", got)
	}

	h.unregister(tab2)
	if got := drain(t, observer); len(got) != 1 || got[0] != typePresence {
		t.Fatalf("expected offline presence event, got %v", got)
	}
	if n := len(h.OnlineUsers()); n != 1 {
		t.Fatalf("expected only observer online, got %d", n)
	}
}

func TestHub_UnregisterTwice(t *testing.T) {
	h := NewHub()
	tab1, tab2 := newTestClient(1), newTestClient(1)
	h.register(tab1)
	h.register(tab2)

	h.unregister(tab1)
	h.unregister(tab1)

	if n := len(h.OnlineUsers()); n != 1 {
		t.Fatalf("repeated unregister must not mark user offline, online=%d", n)
	}
}

func TestHub_MemberLeft(t *testing.T) {
	h := NewHub()
	tab1, tab2, other := newTestClient(1), newTestClient(1), newTestClient(2)
	for _, c := range []*Client{tab1, tab2, other} {
		h.register(c)
		h.subscribe(1, c)
	}
	for _, c := range []*Client{tab1, tab2, other} {
		drain(t, c)
	}

	h.MemberLeft(1, 1)

	for _, tab := range []*Client{tab1, tab2} {
		if got := drain(t, tab); len(got) != 1 || got[0] != typeRoomLeft {
			t.Errorf("each tab must receive room.left, got %v", got)
		}
	}

	h.broadcast(1, []byte("after leave"))
	if len(tab1.send) != 0 || len(tab2.send) != 0 {
		t.Error("user who left must not receive room messages")
	}
	if len(other.send) != 1 {
		t.Error("other members must still receive room messages")
	}
}

func TestHub_RoomDeleted(t *testing.T) {
	h := NewHub()
	a, b := newTestClient(1), newTestClient(2)
	for _, c := range []*Client{a, b} {
		h.register(c)
		h.subscribe(1, c)
	}
	for _, c := range []*Client{a, b} {
		drain(t, c)
	}

	h.RoomDeleted(1)

	for _, c := range []*Client{a, b} {
		if got := drain(t, c); len(got) != 1 || got[0] != typeRoomDeleted {
			t.Errorf("expected room.deleted, got %v", got)
		}
		if len(c.rooms) != 0 {
			t.Error("client must forget the deleted room")
		}
	}
	if len(h.rooms) != 0 {
		t.Errorf("deleted room must be removed from hub, got %d", len(h.rooms))
	}
}

func TestHub_ConcurrentAccess(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Go(func() {
			c := newTestClient(int64(i))
			h.register(c)
			h.subscribe(1, c)
			h.broadcast(1, []byte("x"))
			_ = h.OnlineUsers()
			h.unregister(c)
		})
	}
	wg.Wait()

	if len(h.clients) != 0 || len(h.rooms) != 0 || len(h.users) != 0 {
		t.Errorf("hub must be empty, got clients=%d rooms=%d online=%d", len(h.clients), len(h.rooms), len(h.users))
	}
}

func TestHub_PublishRead(t *testing.T) {
	h := NewHub()
	reader, other, outsider := newTestClient(1), newTestClient(2), newTestClient(3)
	for _, c := range []*Client{reader, other, outsider} {
		h.register(c)
	}
	h.subscribe(1, reader)
	h.subscribe(1, other)
	for _, c := range []*Client{reader, other, outsider} {
		drain(t, c)
	}

	h.PublishRead(domain.ReadReceipt{RoomID: 1, UserID: 1, MessageID: 42})

	for _, c := range []*Client{reader, other} {
		if got := drain(t, c); len(got) != 1 || got[0] != typeMessageRead {
			t.Errorf("room subscriber must receive message.read, got %v", got)
		}
	}
	if got := drain(t, outsider); len(got) != 0 {
		t.Errorf("non-subscriber must not receive message.read, got %v", got)
	}
}

func TestHub_MemberJoinedSubscribesAllTabs(t *testing.T) {
	h := NewHub()
	tab1, tab2, other := newTestClient(1), newTestClient(1), newTestClient(2)
	for _, c := range []*Client{tab1, tab2, other} {
		h.register(c)
	}
	for _, c := range []*Client{tab1, tab2, other} {
		drain(t, c)
	}

	h.MemberJoined(5, 1)

	for _, tab := range []*Client{tab1, tab2} {
		if got := drain(t, tab); len(got) != 1 || got[0] != typeRoomJoined {
			t.Errorf("each tab must receive room.joined, got %v", got)
		}
	}
	if got := drain(t, other); len(got) != 0 {
		t.Errorf("other users must not be notified, got %v", got)
	}

	h.broadcast(5, []byte("hello"))
	if len(tab1.send) != 1 || len(tab2.send) != 1 {
		t.Error("joined user must receive room messages in every tab")
	}
}
