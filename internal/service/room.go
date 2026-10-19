package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"gapu-chap/internal/domain"
)

type RoomRepository interface {
	Create(ctx context.Context, room *domain.Room) error
	GetByID(ctx context.Context, roomID int64) (*domain.Room, error)
	List(ctx context.Context) ([]domain.Room, error)
	Delete(ctx context.Context, roomID int64) error
	AddMember(ctx context.Context, roomID, userID int64) error
	RemoveMember(ctx context.Context, roomID, userID int64) error
	ListMembers(ctx context.Context, roomID int64) ([]domain.Member, error)
	IsMember(ctx context.Context, roomID, userID int64) (bool, error)
	ListForUser(ctx context.Context, userID int64) ([]domain.UserRoom, error)
	CreateDirect(ctx context.Context, userID, peerID int64) (*domain.Room, bool, error)
}

type RoomNotifier interface {
	MemberJoined(roomID, userID int64)
	MemberLeft(roomID, userID int64)
	RoomDeleted(roomID int64)
}

type RoomService struct {
	rooms    RoomRepository
	notifier RoomNotifier
}

func NewRoomService(rooms RoomRepository, notifier RoomNotifier) *RoomService {
	return &RoomService{rooms: rooms, notifier: notifier}
}

const maxRoomNameLength = 64

func (s *RoomService) Create(ctx context.Context, ownerID int64, name string) (*domain.Room, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n == 0 || n > maxRoomNameLength {
		return nil, fmt.Errorf("%w: name is required and must be at most %d characters", domain.ErrValidation, maxRoomNameLength)
	}

	room := &domain.Room{Name: name, OwnerID: ownerID}
	err := s.rooms.Create(ctx, room)
	if err != nil {
		return nil, fmt.Errorf("create room: %w", err)
	}
	return room, nil
}

func (s *RoomService) List(ctx context.Context) ([]domain.Room, error) {
	rooms, err := s.rooms.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}
	return rooms, nil
}

// Get возвращает комнату. Чужой личный диалог для постороннего «не существует»:
// отвечаем 404, а не 403, чтобы не раскрывать сам факт переписки.
func (s *RoomService) Get(ctx context.Context, roomID, userID int64) (*domain.Room, error) {
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("get room %d: %w", roomID, err)
	}
	if err := s.hideForeignDirect(ctx, room, userID); err != nil {
		return nil, err
	}
	return room, nil
}

func (s *RoomService) hideForeignDirect(ctx context.Context, room *domain.Room, userID int64) error {
	if !room.IsDirect() {
		return nil
	}
	ok, err := s.rooms.IsMember(ctx, room.ID, userID)
	if err != nil {
		return fmt.Errorf("check membership in room %d: %w", room.ID, err)
	}
	if !ok {
		return fmt.Errorf("room %d: %w", room.ID, domain.ErrNotFound)
	}
	return nil
}

func (s *RoomService) Delete(ctx context.Context, roomID, actorID int64) error {
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		return fmt.Errorf("get room %d: %w", roomID, err)
	}

	if !room.IsOwnedBy(actorID) {
		return fmt.Errorf("%w: only owner can delete room", domain.ErrForbidden)
	}

	if err := s.rooms.Delete(ctx, roomID); err != nil {
		return fmt.Errorf("delete room %d: %w", roomID, err)
	}

	s.notifier.RoomDeleted(roomID)
	return nil
}

func (s *RoomService) Join(ctx context.Context, roomID, userID int64) error {
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		return fmt.Errorf("get room %d: %w", roomID, err)
	}
	// В чужой диалог вступить нельзя — и для постороннего его как будто нет.
	if room.IsDirect() {
		return fmt.Errorf("room %d: %w", roomID, domain.ErrNotFound)
	}
	if err := s.rooms.AddMember(ctx, roomID, userID); err != nil {
		return fmt.Errorf("add member to room %d: %w", roomID, err)
	}

	s.notifier.MemberJoined(roomID, userID)
	return nil
}

func (s *RoomService) Leave(ctx context.Context, roomID, userID int64) error {
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		return fmt.Errorf("get room %d: %w", roomID, err)
	}

	if room.IsDirect() {
		return fmt.Errorf("%w: you cannot leave a direct conversation, delete it instead", domain.ErrForbidden)
	}
	if room.IsOwnedBy(userID) {
		return fmt.Errorf("%w: the owner cannot leave room, delete it instead", domain.ErrForbidden)
	}

	if err := s.rooms.RemoveMember(ctx, roomID, userID); err != nil {
		return fmt.Errorf("remove member from room %d: %w", roomID, err)
	}

	s.notifier.MemberLeft(roomID, userID)
	return nil
}

func (s *RoomService) Members(ctx context.Context, roomID, userID int64) ([]domain.Member, error) {
	if _, err := s.Get(ctx, roomID, userID); err != nil {
		return nil, err
	}

	members, err := s.rooms.ListMembers(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("list members of room %d: %w", roomID, err)
	}
	return members, nil
}

func (s *RoomService) MyRooms(ctx context.Context, userID int64) ([]domain.UserRoom, error) {
	rooms, err := s.rooms.ListForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list rooms for user %d: %w", userID, err)
	}
	return rooms, nil
}

func (s *RoomService) RequireMember(ctx context.Context, roomID, userID int64) error {
	ok, err := s.rooms.IsMember(ctx, roomID, userID)
	if err != nil {
		return fmt.Errorf("check membership in room %d: %w", roomID, err)
	}
	if !ok {
		return fmt.Errorf("%w: you are not a member of this room", domain.ErrForbidden)
	}
	return nil
}

// OpenDirect открывает личный диалог с пользователем: создаёт при первом обращении,
// дальше возвращает тот же. created сообщает, был ли диалог создан сейчас.
func (s *RoomService) OpenDirect(ctx context.Context, userID, peerID int64) (*domain.Room, bool, error) {
	if peerID <= 0 {
		return nil, false, fmt.Errorf("%w: invalid user id", domain.ErrValidation)
	}
	if peerID == userID {
		return nil, false, fmt.Errorf("%w: you cannot start a conversation with yourself", domain.ErrValidation)
	}

	room, created, err := s.rooms.CreateDirect(ctx, userID, peerID)
	if err != nil {
		return nil, false, fmt.Errorf("open direct room with user %d: %w", peerID, err)
	}

	// Обоих участников сразу подписываем на новый диалог, если они онлайн:
	// иначе собеседник не увидит первое сообщение до перезагрузки.
	if created {
		s.notifier.MemberJoined(room.ID, userID)
		s.notifier.MemberJoined(room.ID, peerID)
	}
	return room, created, nil
}
