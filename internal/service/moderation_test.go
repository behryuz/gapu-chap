package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gapu-chap/internal/domain"
	"gapu-chap/internal/service"
)

type fakeModerationRepo map[int64]*domain.User

func (r fakeModerationRepo) GetByID(_ context.Context, id int64) (*domain.User, error) {
	u, ok := r[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	found := *u
	return &found, nil
}

func (r fakeModerationRepo) SetBanned(_ context.Context, id int64, banned bool) error {
	u, ok := r[id]
	if !ok {
		return domain.ErrNotFound
	}
	if banned {
		now := time.Now()
		u.BannedAt = &now
	} else {
		u.BannedAt = nil
	}
	return nil
}

type fakeSessions struct{ disconnected []int64 }

func (s *fakeSessions) DisconnectUser(userID int64) {
	s.disconnected = append(s.disconnected, userID)
}

const (
	moderatorID   int64 = 1
	regularUserID int64 = 2
	otherModID    int64 = 3
)

func newModeration() (*service.ModerationService, fakeModerationRepo, *fakeSessions) {
	repo := fakeModerationRepo{
		moderatorID:   {ID: moderatorID, Role: domain.RoleModerator},
		regularUserID: {ID: regularUserID, Role: domain.RoleUser},
		otherModID:    {ID: otherModID, Role: domain.RoleModerator},
	}
	sessions := &fakeSessions{}
	return service.NewModerationService(repo, sessions), repo, sessions
}

func TestBan_AndUnban(t *testing.T) {
	svc, repo, sessions := newModeration()

	if err := svc.Ban(t.Context(), moderatorID, regularUserID); err != nil {
		t.Fatalf("ban: %v", err)
	}
	if !repo[regularUserID].IsBanned() {
		t.Error("user must be banned")
	}
	if len(sessions.disconnected) != 1 || sessions.disconnected[0] != regularUserID {
		t.Errorf("banned user's sessions must be closed, got %v", sessions.disconnected)
	}

	if err := svc.Unban(t.Context(), moderatorID, regularUserID); err != nil {
		t.Fatalf("unban: %v", err)
	}
	if repo[regularUserID].IsBanned() {
		t.Error("user must be unbanned")
	}
}

func TestBan_Rules(t *testing.T) {
	tests := []struct {
		name     string
		actorID  int64
		targetID int64
		wantErr  error
	}{
		{"regular user cannot ban", regularUserID, otherModID, domain.ErrForbidden},
		{"moderator cannot ban another moderator", moderatorID, otherModID, domain.ErrForbidden},
		{"moderator cannot ban himself", moderatorID, moderatorID, domain.ErrValidation},
		{"unknown user", moderatorID, 999, domain.ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, sessions := newModeration()
			if err := svc.Ban(t.Context(), tt.actorID, tt.targetID); !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if len(sessions.disconnected) != 0 {
				t.Error("failed ban must not close sessions")
			}
		})
	}
}
