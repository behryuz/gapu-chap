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
	ExistsInRoom(ctx context.Context, roomID, messageID int64) (bool, error)
	Search(ctx context.Context, roomID int64, tsquery string, beforeID int64, limit int) ([]domain.Message, error)
}

type MembershipChecker interface {
	IsMember(ctx context.Context, roomID, userID int64) (bool, error)
}

type ReadMarker interface {
	MarkRead(ctx context.Context, roomID, userID, messageID int64) (bool, error)
}

type MessagePublisher interface {
	PublishMessage(msg *domain.Message)
	PublishRead(receipt domain.ReadReceipt)
}

type MessageService struct {
	messages  MessageRepository
	members   MembershipChecker
	reads     ReadMarker
	publisher MessagePublisher
}

func NewMessageService(
	messages MessageRepository,
	members MembershipChecker,
	reads ReadMarker,
	publisher MessagePublisher,
) *MessageService {
	return &MessageService{messages: messages, members: members, reads: reads, publisher: publisher}
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

	s.publisher.PublishMessage(msg)
	return msg, nil
}

type HistoryQuery struct {
	BeforeID int64
	Limit    int
}

func (q HistoryQuery) limit() int {
	if q.Limit <= 0 {
		return defaultHistoryLimit
	}
	return min(q.Limit, maxHistoryLimit)
}

func (s *MessageService) History(ctx context.Context, roomID, userID int64, q HistoryQuery) ([]domain.Message, error) {
	if err := s.ensureMember(ctx, roomID, userID); err != nil {
		return nil, err
	}

	messages, err := s.messages.ListByRoom(ctx, roomID, q.BeforeID, q.limit())
	if err != nil {
		return nil, fmt.Errorf("list messages in room %d: %w", roomID, err)
	}
	return messages, nil
}

// Search — поиск по истории комнаты. Те же права, что у истории (только участники),
// и та же курсорная пагинация: результаты от новых к старым.
func (s *MessageService) Search(ctx context.Context, roomID, userID int64, query string, q HistoryQuery) ([]domain.Message, error) {
	tsquery, err := buildSearchQuery(query)
	if err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, roomID, userID); err != nil {
		return nil, err
	}

	messages, err := s.messages.Search(ctx, roomID, tsquery, q.BeforeID, q.limit())
	if err != nil {
		return nil, fmt.Errorf("search messages in room %d: %w", roomID, err)
	}
	return messages, nil
}

func (s *MessageService) MarkRead(ctx context.Context, roomID, userID, messageID int64) error {
	if messageID <= 0 {
		return fmt.Errorf("%w: message_id must be positive", domain.ErrValidation)
	}

	if err := s.ensureMember(ctx, roomID, userID); err != nil {
		return err
	}

	exists, err := s.messages.ExistsInRoom(ctx, roomID, messageID)
	if err != nil {
		return fmt.Errorf("check messages %d in room %d: %w", messageID, roomID, err)
	}
	if !exists {
		return fmt.Errorf("%w: message %d in room %d", domain.ErrNotFound, messageID, roomID)
	}

	advanced, err := s.reads.MarkRead(ctx, roomID, userID, messageID)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	if advanced {
		s.publisher.PublishRead(domain.ReadReceipt{RoomID: roomID, UserID: userID, MessageID: messageID})
	}
	return nil
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
