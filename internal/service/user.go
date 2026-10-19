package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"gapu-chap/internal/domain"
)

type UserDirectory interface {
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	Search(ctx context.Context, query string, limit int) ([]domain.User, error)
}

type UserService struct {
	users UserDirectory
}

func NewUserService(users UserDirectory) *UserService {
	return &UserService{users: users}
}

const (
	maxUserSearchLength = 32
	userSearchLimit     = 20
)

func (s *UserService) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user %d: %w", id, err)
	}
	return user, nil
}

func (s *UserService) Search(ctx context.Context, query string) ([]domain.User, error) {
	query = strings.TrimSpace(query)
	if n := utf8.RuneCountInString(query); n == 0 || n > maxUserSearchLength {
		return nil, fmt.Errorf("%w: search query must be 1-%d characters", domain.ErrValidation, maxUserSearchLength)
	}

	users, err := s.users.Search(ctx, query, userSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	return users, nil
}
