package ws

import (
	"sync"
	"testing"
)

func newTestClient(userID int64) *Client {
	return newClient(nil, userID, "user")
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

func TestHub_ConcurrentAccess(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Go(func() {
			c := newTestClient(int64(i))
			h.register(c)
			h.subscribe(1, c)
			h.broadcast(1, []byte("x"))
			h.unregister(c)
		})
	}
	wg.Wait()

	if len(h.clients) != 0 || len(h.rooms) != 0 {
		t.Errorf("hub must be empty, got clients=%d rooms=%d", len(h.clients), len(h.rooms))
	}
}
