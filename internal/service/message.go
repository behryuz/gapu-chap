package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"gapu-chap/internal/domain"
)

type MessageRepository interface {
	Create(ctx context.Context, msg *domain.Message) error
	ListByRoom(ctx context.Context, roomID, beforeID int64, limit int) ([]domain.Message, error)
}

type MembershipChecker interface {
	IsMember(ctx context.Context, roomID, userID int64) (bool, error)
}

type MessageService struct {
	messages MessageRepository
	members  MembershipChecker
}

func NewMessageService(messages MessageRepository, members MembershipChecker) *MessageService {
	return &MessageService{messages: messages, members: members}
}

const (
	maxMessageLength    = 4000
	defaultHistoryLimit = 50
	maxHistoryLimit     = 100
)

func (s *MessageService) Send(ctx context.Context, roomID, authorID int64, body string) (*domain.Message, error) {
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n == 0 || n > maxMessageLength {
		return nil, fmt.Errorf("%w: message must be 1-%d characters", domain.ErrValidation, maxMessageLength)
	}

	if err := s.ensureMember(ctx, roomID, authorID); err != nil {
		return nil, err
	}

	msg := &domain.Message{RoomID: roomID, AuthorID: authorID, Body: body}
	if err := s.messages.Create(ctx, msg); err != nil {
		return nil, fmt.Errorf("create message in room %d: %w", roomID, err)
	}
	return msg, nil
}

type HistoryQuery struct {
	BeforeID int64
	Limit    int
}

func (s *MessageService) History(ctx context.Context, roomID, userID int64, q HistoryQuery) ([]domain.Message, error) {
	if err := s.ensureMember(ctx, roomID, userID); err != nil {
		return nil, err
	}

	if q.Limit <= 0 {
		q.Limit = defaultHistoryLimit
	}
	q.Limit = min(q.Limit, maxHistoryLimit)

	messages, err := s.messages.ListByRoom(ctx, roomID, q.BeforeID, q.Limit)
	if err != nil {
		return nil, fmt.Errorf("list messages in room %d: %w", roomID, err)
	}
	return messages, nil
}

func (s *MessageService) ensureMember(ctx context.Context, roomID, userID int64) error {
	ok, err := s.members.IsMember(ctx, roomID, userID)
	if err != nil {
		return fmt.Errorf("check membership in room %d: %w", roomID, err)
	}
	if !ok {
		return fmt.Errorf("%w: you are not a member of this room", domain.ErrForbidden)
	}
	return nil
}
