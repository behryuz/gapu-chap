package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"gapu-chap/internal/domain"
	"gapu-chap/internal/repository/postgres"
	"gapu-chap/internal/testutil/testdb"
)

type repos struct {
	users    *postgres.UserRepository
	rooms    *postgres.RoomRepository
	messages *postgres.MessageRepository
}

func setup(t *testing.T) repos {
	t.Helper()
	t.Parallel()
	pool := testdb.New(t)
	return newRepos(pool)
}

func newRepos(pool *pgxpool.Pool) repos {
	return repos{
		users:    postgres.NewUserRepository(pool),
		rooms:    postgres.NewRoomRepository(pool),
		messages: postgres.NewMessageRepository(pool),
	}
}

func (r repos) user(t *testing.T, username string) *domain.User {
	t.Helper()
	u := &domain.User{FirstName: "Test", Username: username, PasswordHash: "hash"}
	if err := r.users.Create(t.Context(), u); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return u
}

func (r repos) room(t *testing.T, owner *domain.User, name string) *domain.Room {
	t.Helper()
	room := &domain.Room{Name: name, OwnerID: owner.ID}
	if err := r.rooms.Create(t.Context(), room); err != nil {
		t.Fatalf("create room %s: %v", name, err)
	}
	return room
}

func (r repos) message(t *testing.T, room *domain.Room, author *domain.User, body string) *domain.Message {
	t.Helper()
	m := &domain.Message{RoomID: room.ID, AuthorID: author.ID, Body: body}
	if err := r.messages.Create(t.Context(), m); err != nil {
		t.Fatalf("create message: %v", err)
	}
	return m
}

func TestUserRepository(t *testing.T) {
	r := setup(t)
	ctx := t.Context()
	alice := r.user(t, "alice_dev")

	if alice.Role != domain.RoleUser {
		t.Errorf("new user must get role %q by default, got %q", domain.RoleUser, alice.Role)
	}

	t.Run("username is unique case-insensitively", func(t *testing.T) {
		err := r.users.Create(ctx, &domain.User{FirstName: "X", Username: "ALICE_DEV", PasswordHash: "h"})
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected ErrConflict, got %v", err)
		}
	})

	t.Run("lookup by username ignores case", func(t *testing.T) {
		u, err := r.users.GetByUsername(ctx, "Alice_Dev")
		if err != nil || u.ID != alice.ID {
			t.Fatalf("expected alice, got %+v, %v", u, err)
		}
	})

	t.Run("search treats _ and % literally", func(t *testing.T) {
		r.user(t, "aliceXdev")
		found, err := r.users.Search(ctx, "alice_", 10)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(found) != 1 || found[0].ID != alice.ID {
			t.Errorf("expected only alice_dev, got %+v", found)
		}
	})

	t.Run("ban and unban", func(t *testing.T) {
		if err := r.users.SetBanned(ctx, alice.ID, true); err != nil {
			t.Fatalf("ban: %v", err)
		}
		u, _ := r.users.GetByID(ctx, alice.ID)
		if !u.IsBanned() {
			t.Fatal("user must be banned")
		}
		if err := r.users.SetBanned(ctx, alice.ID, false); err != nil {
			t.Fatalf("unban: %v", err)
		}
		u, _ = r.users.GetByID(ctx, alice.ID)
		if u.IsBanned() {
			t.Fatal("user must be unbanned")
		}
		if err := r.users.SetBanned(ctx, 999_999, true); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("unknown user: expected ErrNotFound, got %v", err)
		}
	})
}

func TestRoomRepository_PublicRooms(t *testing.T) {
	r := setup(t)
	ctx := t.Context()
	owner := r.user(t, "owner")
	room := r.room(t, owner, "General")

	ok, err := r.rooms.IsMember(ctx, room.ID, owner.ID)
	if err != nil || !ok {
		t.Fatalf("owner must become a member in the same transaction, ok=%v err=%v", ok, err)
	}
	if room.Kind != domain.RoomKindPublic {
		t.Errorf("expected public room, got %q", room.Kind)
	}

	err = r.rooms.Create(ctx, &domain.Room{Name: "general", OwnerID: owner.ID})
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("room names must be unique case-insensitively, got %v", err)
	}

	if err := r.rooms.AddMember(ctx, 999_999, owner.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("joining a missing room: expected ErrNotFound, got %v", err)
	}
}

func TestRoomRepository_DirectRooms(t *testing.T) {
	r := setup(t)
	ctx := t.Context()
	alice, bob := r.user(t, "alice"), r.user(t, "bob")

	room, created, err := r.rooms.CreateDirect(ctx, alice.ID, bob.ID)
	if err != nil || !created {
		t.Fatalf("create direct: created=%v err=%v", created, err)
	}

	again, created, err := r.rooms.CreateDirect(ctx, bob.ID, alice.ID)
	if err != nil || created || again.ID != room.ID {
		t.Fatalf("reverse order must return the same room: %+v created=%v err=%v", again, created, err)
	}

	t.Run("concurrent opens create exactly one room", func(t *testing.T) {
		carol, dave := r.user(t, "carol"), r.user(t, "dave")
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			ids     = map[int64]bool{}
			creates int
		)
		for i := range 10 {
			wg.Go(func() {
				a, b := carol.ID, dave.ID
				if i%2 == 1 {
					a, b = b, a
				}
				room, created, err := r.rooms.CreateDirect(context.Background(), a, b)
				if err != nil {
					t.Errorf("create direct: %v", err)
					return
				}
				mu.Lock()
				defer mu.Unlock()
				ids[room.ID] = true
				if created {
					creates++
				}
			})
		}
		wg.Wait()
		if len(ids) != 1 || creates != 1 {
			t.Errorf("expected one room created once, got rooms=%v creates=%d", ids, creates)
		}
	})

	t.Run("direct rooms are hidden from the public list", func(t *testing.T) {
		r.room(t, alice, "public-room")
		rooms, err := r.rooms.List(ctx)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, room := range rooms {
			if room.IsDirect() {
				t.Errorf("public list must not contain direct room %d", room.ID)
			}
		}
	})

	t.Run("my rooms show the peer", func(t *testing.T) {
		rooms, err := r.rooms.ListForUser(ctx, alice.ID)
		if err != nil {
			t.Fatalf("list for user: %v", err)
		}
		var dm *domain.UserRoom
		for i := range rooms {
			if rooms[i].ID == room.ID {
				dm = &rooms[i]
			}
		}
		if dm == nil || dm.Peer == nil || dm.Peer.ID != bob.ID || dm.Peer.Username != "bob" {
			t.Fatalf("expected peer bob in direct room, got %+v", dm)
		}
	})

	t.Run("missing peer", func(t *testing.T) {
		_, _, err := r.rooms.CreateDirect(ctx, alice.ID, 999_999)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestReadMarksAndUnreadCount(t *testing.T) {
	r := setup(t)
	ctx := t.Context()
	alice, bob := r.user(t, "alice"), r.user(t, "bob")
	room := r.room(t, alice, "chat")
	if err := r.rooms.AddMember(ctx, room.ID, bob.ID); err != nil {
		t.Fatalf("join: %v", err)
	}

	var last *domain.Message
	for i := range 3 {
		last = r.message(t, room, alice, fmt.Sprintf("m%d", i))
	}
	r.message(t, room, bob, "bob's own message is never unread for bob")

	unread := func() int64 {
		t.Helper()
		rooms, err := r.rooms.ListForUser(ctx, bob.ID)
		if err != nil || len(rooms) != 1 {
			t.Fatalf("list for user: %+v %v", rooms, err)
		}
		return rooms[0].UnreadCount
	}

	if got := unread(); got != 3 {
		t.Fatalf("expected 3 unread, got %d", got)
	}

	advanced, err := r.rooms.MarkRead(ctx, room.ID, bob.ID, last.ID)
	if err != nil || !advanced {
		t.Fatalf("mark read: advanced=%v err=%v", advanced, err)
	}
	if got := unread(); got != 0 {
		t.Errorf("expected 0 unread after reading, got %d", got)
	}

	advanced, err = r.rooms.MarkRead(ctx, room.ID, bob.ID, last.ID-1)
	if err != nil || advanced {
		t.Errorf("read marker must not move backwards: advanced=%v err=%v", advanced, err)
	}
}

func TestMessages_HistoryAndSearch(t *testing.T) {
	r := setup(t)
	ctx := t.Context()
	alice := r.user(t, "alice")
	room := r.room(t, alice, "chat")
	other := r.room(t, alice, "other")

	bodies := []string{"Привет всем", "Как дела?", "привет, Алиса", "Hello world", "прИВЕТствую"}
	for _, body := range bodies {
		r.message(t, room, alice, body)
	}
	r.message(t, other, alice, "привет из другой комнаты")

	t.Run("cursor pagination has no gaps or duplicates", func(t *testing.T) {
		var got []string
		before := int64(0)
		for {
			page, err := r.messages.ListByRoom(ctx, room.ID, before, 2)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(page) == 0 {
				break
			}
			for _, m := range page {
				if m.AuthorUsername != "alice" {
					t.Errorf("author username must be joined, got %q", m.AuthorUsername)
				}
				got = append(got, m.Body)
			}
			before = page[len(page)-1].ID
		}
		if len(got) != len(bodies) || got[0] != bodies[len(bodies)-1] {
			t.Errorf("expected all %d messages newest first, got %v", len(bodies), got)
		}
	})

	t.Run("search by word prefix, case-insensitive, within the room", func(t *testing.T) {
		found, err := r.messages.Search(ctx, room.ID, "прив:*", 0, 10)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(found) != 3 {
			t.Errorf("expected 3 matches in this room, got %d: %+v", len(found), found)
		}

		found, err = r.messages.Search(ctx, room.ID, "привет:* & алиса:*", 0, 10)
		if err != nil || len(found) != 1 || found[0].Body != "привет, Алиса" {
			t.Errorf("all words must match: %+v %v", found, err)
		}
	})

	t.Run("existence check is scoped to the room", func(t *testing.T) {
		foreign := r.message(t, other, alice, "x")
		ok, err := r.messages.ExistsInRoom(ctx, room.ID, foreign.ID)
		if err != nil || ok {
			t.Errorf("message from another room must not count: ok=%v err=%v", ok, err)
		}
	})
}
