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
}

type RoomService struct {
	rooms RoomRepository
}

func NewRoomService(rooms RoomRepository) *RoomService {
	return &RoomService{rooms: rooms}
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

func (s *RoomService) Get(ctx context.Context, id int64) (*domain.Room, error) {
	room, err := s.rooms.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get room %d: %w", id, err)
	}
	return room, nil
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
	return nil
}

func (s *RoomService) Join(ctx context.Context, roomID, userID int64) error {
	if _, err := s.rooms.GetByID(ctx, roomID); err != nil {
		return fmt.Errorf("get room %d: %w", roomID, err)
	}
	if err := s.rooms.AddMember(ctx, roomID, userID); err != nil {
		return fmt.Errorf("add member to room %d: %w", roomID, err)
	}
	return nil
}

func (s *RoomService) Leave(ctx context.Context, roomID, userID int64) error {
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		return fmt.Errorf("get room %d: %w", roomID, err)
	}

	if room.IsOwnedBy(userID) {
		return fmt.Errorf("%w: the owner cannot leave room, delete it instead", domain.ErrForbidden)
	}

	if err := s.rooms.RemoveMember(ctx, roomID, userID); err != nil {
		return fmt.Errorf("remove member from room %d: %w", roomID, err)
	}
	return nil
}

func (s *RoomService) Members(ctx context.Context, roomID int64) ([]domain.Member, error) {
	if _, err := s.rooms.GetByID(ctx, roomID); err != nil {
		return nil, fmt.Errorf("get room %d: %w", roomID, err)
	}

	members, err := s.rooms.ListMembers(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("list members of room %d: %w", roomID, err)
	}
	return members, nil
}
