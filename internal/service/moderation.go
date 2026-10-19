package service

import (
	"context"
	"fmt"

	"gapu-chap/internal/domain"
)

type ModerationRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	SetBanned(ctx context.Context, id int64, banned bool) error
}

// SessionTerminator обрывает живые соединения пользователя (WebSocket).
// REST-запросы забаненного отсекает middleware, а открытый сокет нужно закрыть явно.
type SessionTerminator interface {
	DisconnectUser(userID int64)
}

type ModerationService struct {
	users    ModerationRepository
	sessions SessionTerminator
}

func NewModerationService(users ModerationRepository, sessions SessionTerminator) *ModerationService {
	return &ModerationService{users: users, sessions: sessions}
}

func (s *ModerationService) Ban(ctx context.Context, actorID, targetID int64) error {
	if err := s.checkCanModerate(ctx, actorID, targetID); err != nil {
		return err
	}
	if err := s.users.SetBanned(ctx, targetID, true); err != nil {
		return fmt.Errorf("ban user %d: %w", targetID, err)
	}

	s.sessions.DisconnectUser(targetID)
	return nil
}

func (s *ModerationService) Unban(ctx context.Context, actorID, targetID int64) error {
	if err := s.checkCanModerate(ctx, actorID, targetID); err != nil {
		return err
	}
	if err := s.users.SetBanned(ctx, targetID, false); err != nil {
		return fmt.Errorf("unban user %d: %w", targetID, err)
	}
	return nil
}

// Права проверяем здесь, а не только в HTTP-слое: роль читаем из базы (а не из токена),
// поэтому снятие роли модератора действует сразу.
func (s *ModerationService) checkCanModerate(ctx context.Context, actorID, targetID int64) error {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("get moderator %d: %w", actorID, err)
	}
	if !actor.IsModerator() {
		return fmt.Errorf("%w: only moderators can do this", domain.ErrForbidden)
	}
	if actorID == targetID {
		return fmt.Errorf("%w: you cannot moderate yourself", domain.ErrValidation)
	}

	target, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return fmt.Errorf("get user %d: %w", targetID, err)
	}
	if target.IsModerator() {
		return fmt.Errorf("%w: moderators cannot be banned", domain.ErrForbidden)
	}
	return nil
}
