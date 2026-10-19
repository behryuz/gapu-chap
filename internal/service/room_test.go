package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gapu-chap/internal/domain"
	"gapu-chap/internal/service"
)

type fakeRoomRepo struct {
	rooms   map[int64]*domain.Room
	members map[int64]map[int64]bool
	nextID  int64
}

func newFakeRoomRepo() *fakeRoomRepo {
	return &fakeRoomRepo{
		rooms:   make(map[int64]*domain.Room),
		members: make(map[int64]map[int64]bool),
	}
}

type fakeRoomNotifier struct {
	joined  [][2]int64
	left    [][2]int64
	deleted []int64
}

func (n *fakeRoomNotifier) MemberJoined(roomID, userID int64) {
	n.joined = append(n.joined, [2]int64{roomID, userID})
}

func (n *fakeRoomNotifier) MemberLeft(roomID, userID int64) {
	n.left = append(n.left, [2]int64{roomID, userID})
}

func (n *fakeRoomNotifier) RoomDeleted(roomID int64) {
	n.deleted = append(n.deleted, roomID)
}

func (r *fakeRoomRepo) ListForUser(_ context.Context, userID int64) ([]domain.UserRoom, error) {
	var result []domain.UserRoom
	for roomID, members := range r.members {
		if members[userID] {
			result = append(result, domain.UserRoom{Room: *r.rooms[roomID]})
		}
	}
	return result, nil
}

func (r *fakeRoomRepo) IsMember(_ context.Context, roomID, userID int64) (bool, error) {
	return r.isMember(roomID, userID), nil
}

func (r *fakeRoomRepo) CreateDirect(_ context.Context, userID, peerID int64) (*domain.Room, bool, error) {
	key := domain.DirectKey(userID, peerID)
	for _, room := range r.rooms {
		if room.IsDirect() && room.Name == key {
			found := *room
			return &found, false, nil
		}
	}
	r.nextID++
	room := &domain.Room{ID: r.nextID, Name: key, OwnerID: userID, Kind: domain.RoomKindDirect}
	r.rooms[room.ID] = room
	r.members[room.ID] = map[int64]bool{userID: true, peerID: true}
	created := *room
	return &created, true, nil
}

func (r *fakeRoomRepo) Create(_ context.Context, room *domain.Room) error {
	r.nextID++
	room.ID = r.nextID
	room.Kind = domain.RoomKindPublic
	room.CreatedAt = time.Now()

	stored := *room
	r.rooms[room.ID] = &stored
	r.members[room.ID] = map[int64]bool{room.OwnerID: true}
	return nil
}

func (r *fakeRoomRepo) GetByID(_ context.Context, id int64) (*domain.Room, error) {
	room, ok := r.rooms[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	found := *room
	return &found, nil
}

func (r *fakeRoomRepo) List(_ context.Context) ([]domain.Room, error) {
	list := make([]domain.Room, 0, len(r.rooms))
	for _, room := range r.rooms {
		list = append(list, *room)
	}
	return list, nil
}

func (r *fakeRoomRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.rooms[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.rooms, id)
	delete(r.members, id)
	return nil
}

func (r *fakeRoomRepo) AddMember(_ context.Context, roomID, userID int64) error {
	members, ok := r.members[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	members[userID] = true
	return nil
}

func (r *fakeRoomRepo) RemoveMember(_ context.Context, roomID, userID int64) error {
	delete(r.members[roomID], userID)
	return nil
}

func (r *fakeRoomRepo) ListMembers(_ context.Context, roomID int64) ([]domain.Member, error) {
	members := make([]domain.Member, 0, len(r.members[roomID]))
	for userID := range r.members[roomID] {
		members = append(members, domain.Member{UserID: userID})
	}
	return members, nil
}

func (r *fakeRoomRepo) isMember(roomID, userID int64) bool {
	return r.members[roomID][userID]
}

const (
	ownerID     int64 = 1
	otherUserID int64 = 2
)

func setupRoom(t *testing.T) (*service.RoomService, *fakeRoomRepo, *domain.Room) {
	t.Helper()

	repo := newFakeRoomRepo()
	svc := service.NewRoomService(repo, &fakeRoomNotifier{})

	room, err := svc.Create(t.Context(), ownerID, "general")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	return svc, repo, room
}

func TestRoomCreate_Success(t *testing.T) {
	repo := newFakeRoomRepo()
	svc := service.NewRoomService(repo, &fakeRoomNotifier{})

	room, err := svc.Create(t.Context(), ownerID, "  general  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if room.Name != "general" {
		t.Errorf("expected trimmed name, got %q", room.Name)
	}
	if room.OwnerID != ownerID {
		t.Errorf("expected owner %d, got %d", ownerID, room.OwnerID)
	}
	if !repo.isMember(room.ID, ownerID) {
		t.Error("owner must become a member of the room")
	}
}

func TestRoomCreate_Validation(t *testing.T) {
	tests := []struct {
		name     string
		roomName string
	}{
		{"empty", ""},
		{"only spaces", "    "},
		{"too long", strings.Repeat("я", 65)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := service.NewRoomService(newFakeRoomRepo(), &fakeRoomNotifier{})

			_, err := svc.Create(t.Context(), ownerID, tt.roomName)
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("expected ErrValidation, got %v", err)
			}
		})
	}
}

func TestRoomDelete(t *testing.T) {
	tests := []struct {
		name        string
		actorID     int64
		missingRoom bool
		wantErr     error
	}{
		{"owner deletes", ownerID, false, nil},
		{"other user is forbidden", otherUserID, false, domain.ErrForbidden},
		{"room not found", ownerID, true, domain.ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, room := setupRoom(t)

			roomID := room.ID
			if tt.missingRoom {
				roomID = 999
			}

			err := svc.Delete(t.Context(), roomID, tt.actorID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}

			_, exists := repo.rooms[room.ID]
			wantDeleted := tt.wantErr == nil
			if exists == wantDeleted {
				t.Errorf("room exists = %v, expected deleted = %v", exists, wantDeleted)
			}
		})
	}
}

func TestRoomJoinAndLeave(t *testing.T) {
	svc, repo, room := setupRoom(t)

	if err := svc.Join(t.Context(), room.ID, otherUserID); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := svc.Join(t.Context(), room.ID, otherUserID); err != nil {
		t.Fatalf("second join must be idempotent, got %v", err)
	}
	if !repo.isMember(room.ID, otherUserID) {
		t.Fatal("user must be a member after join")
	}

	if err := svc.Leave(t.Context(), room.ID, otherUserID); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if repo.isMember(room.ID, otherUserID) {
		t.Error("user must not be a member after leave")
	}
}

func TestRoomJoin_NotFound(t *testing.T) {
	svc := service.NewRoomService(newFakeRoomRepo(), &fakeRoomNotifier{})

	err := svc.Join(t.Context(), 999, otherUserID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRoomLeave_OwnerForbidden(t *testing.T) {
	svc, repo, room := setupRoom(t)

	err := svc.Leave(t.Context(), room.ID, ownerID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if !repo.isMember(room.ID, ownerID) {
		t.Error("owner must remain a member")
	}
}

func TestRoom_NotifiesHubOnLeaveAndDelete(t *testing.T) {
	repo := newFakeRoomRepo()
	notifier := &fakeRoomNotifier{}
	svc := service.NewRoomService(repo, notifier)

	room, err := svc.Create(t.Context(), ownerID, "general")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	if err := svc.Join(t.Context(), room.ID, otherUserID); err != nil {
		t.Fatalf("join: %v", err)
	}

	if err := svc.Leave(t.Context(), room.ID, ownerID); err == nil {
		t.Fatal("owner must not be able to leave")
	}
	if len(notifier.left) != 0 {
		t.Fatalf("failed leave must not notify, got %v", notifier.left)
	}

	if err := svc.Leave(t.Context(), room.ID, otherUserID); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if len(notifier.left) != 1 || notifier.left[0] != [2]int64{room.ID, otherUserID} {
		t.Errorf("expected MemberLeft(%d, %d), got %v", room.ID, otherUserID, notifier.left)
	}

	if err := svc.Delete(t.Context(), room.ID, otherUserID); err == nil {
		t.Fatal("non-owner must not be able to delete")
	}
	if len(notifier.deleted) != 0 {
		t.Fatalf("failed delete must not notify, got %v", notifier.deleted)
	}

	if err := svc.Delete(t.Context(), room.ID, ownerID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(notifier.deleted) != 1 || notifier.deleted[0] != room.ID {
		t.Errorf("expected RoomDeleted(%d), got %v", room.ID, notifier.deleted)
	}
}

const strangerUserID int64 = 3

func TestOpenDirect(t *testing.T) {
	repo := newFakeRoomRepo()
	notifier := &fakeRoomNotifier{}
	svc := service.NewRoomService(repo, notifier)

	room, created, err := svc.OpenDirect(t.Context(), ownerID, otherUserID)
	if err != nil || !created {
		t.Fatalf("first open must create the room, created=%v err=%v", created, err)
	}
	if !room.IsDirect() {
		t.Errorf("expected direct room, got kind %q", room.Kind)
	}
	if len(notifier.joined) != 2 {
		t.Errorf("both participants must be subscribed, got %v", notifier.joined)
	}

	// Собеседник открывает тот же диалог со своей стороны — второй не создаётся.
	again, created, err := svc.OpenDirect(t.Context(), otherUserID, ownerID)
	if err != nil || created || again.ID != room.ID {
		t.Fatalf("expected the same room %d, got %+v created=%v err=%v", room.ID, again, created, err)
	}
	if len(notifier.joined) != 2 {
		t.Errorf("reopening must not notify again, got %v", notifier.joined)
	}
}

func TestOpenDirect_WithYourself(t *testing.T) {
	svc := service.NewRoomService(newFakeRoomRepo(), &fakeRoomNotifier{})

	_, _, err := svc.OpenDirect(t.Context(), ownerID, ownerID)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestDirectRoom_Rules(t *testing.T) {
	repo := newFakeRoomRepo()
	svc := service.NewRoomService(repo, &fakeRoomNotifier{})
	room, _, err := svc.OpenDirect(t.Context(), ownerID, otherUserID)
	if err != nil {
		t.Fatalf("open direct: %v", err)
	}

	tests := []struct {
		name    string
		action  func() error
		wantErr error
	}{
		{"stranger cannot join", func() error { return svc.Join(t.Context(), room.ID, strangerUserID) }, domain.ErrNotFound},
		{"stranger does not see the room", func() error {
			_, err := svc.Get(t.Context(), room.ID, strangerUserID)
			return err
		}, domain.ErrNotFound},
		{"stranger does not see members", func() error {
			_, err := svc.Members(t.Context(), room.ID, strangerUserID)
			return err
		}, domain.ErrNotFound},
		{"participant cannot leave", func() error { return svc.Leave(t.Context(), room.ID, otherUserID) }, domain.ErrForbidden},
		{"participant sees the room", func() error {
			_, err := svc.Get(t.Context(), room.ID, otherUserID)
			return err
		}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.action(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
	if repo.isMember(room.ID, strangerUserID) {
		t.Error("stranger must not become a member of a direct room")
	}
}

func TestRoomJoin_NotifiesHub(t *testing.T) {
	_, _, room := setupRoom(t)
	notifier := &fakeRoomNotifier{}
	svc := service.NewRoomService(newFakeRoomRepoWith(room), notifier)

	if err := svc.Join(t.Context(), room.ID, otherUserID); err != nil {
		t.Fatalf("join: %v", err)
	}
	if len(notifier.joined) != 1 || notifier.joined[0] != [2]int64{room.ID, otherUserID} {
		t.Errorf("expected MemberJoined(%d, %d), got %v", room.ID, otherUserID, notifier.joined)
	}
}

func newFakeRoomRepoWith(room *domain.Room) *fakeRoomRepo {
	repo := newFakeRoomRepo()
	stored := *room
	repo.rooms[room.ID] = &stored
	repo.members[room.ID] = map[int64]bool{room.OwnerID: true}
	repo.nextID = room.ID
	return repo
}
