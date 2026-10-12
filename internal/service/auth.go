package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"gapu-chap/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) bool
}

type TokenIssuer interface {
	Issue(userID int64, username string) (string, error)
}

type AuthService struct {
	users  UserRepository
	hasher PasswordHasher
	tokens TokenIssuer
}

func NewAuthService(users UserRepository, hasher PasswordHasher, tokens TokenIssuer) *AuthService {
	return &AuthService{users: users, hasher: hasher, tokens: tokens}
}

type RegisterInput struct {
	FirstName string
	LastName  string
	Username  string
	Password  string
}

var usernameRe = regexp.MustCompile("^[a-zA-Z0-9_]{3,32}$")

func (in *RegisterInput) normalize() {
	in.FirstName = strings.TrimSpace(in.FirstName)
	in.LastName = strings.TrimSpace(in.LastName)
	in.Username = strings.TrimSpace(in.Username)
}

func (in *RegisterInput) validate() error {
	switch {
	case !usernameRe.MatchString(in.Username):
		return fmt.Errorf("%w: username must be 3-32 characters: latin letters, digits, underscore", domain.ErrValidation)
	case in.FirstName == "" || utf8.RuneCountInString(in.FirstName) > 64:
		return fmt.Errorf("%w: first_name is required and must be at most 64 characters", domain.ErrValidation)
	case utf8.RuneCountInString(in.LastName) > 64:
		return fmt.Errorf("%w: last_name must not be longer than 64 characters", domain.ErrValidation)
	case utf8.RuneCountInString(in.Password) < 8:
		return fmt.Errorf("%w: password must contain at least 8 characters", domain.ErrValidation)
	case len(in.Password) > 72:
		return fmt.Errorf("%w: password must not exceed 72 bytes", domain.ErrValidation)
	}
	return nil
}

func (s *AuthService) Register(ctx context.Context, in RegisterInput) (*domain.User, error) {
	in.normalize()
	if err := in.validate(); err != nil {
		return nil, err
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	user := &domain.User{
		FirstName:    in.FirstName,
		LastName:     in.LastName,
		Username:     in.Username,
		PasswordHash: hash,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return user, nil
}

func (s *AuthService) Login(ctx context.Context, username, password string) (string, error) {
	user, err := s.users.GetByUsername(ctx, strings.TrimSpace(username))
	if errors.Is(err, domain.ErrNotFound) {
		return "", domain.ErrInvalidCredentials
	}
	if err != nil {
		return "", fmt.Errorf("getting user by username: %w", err)
	}

	if !s.hasher.Compare(user.PasswordHash, password) {
		return "", domain.ErrInvalidCredentials
	}

	token, err := s.tokens.Issue(user.ID, user.Username)
	if err != nil {
		return "", fmt.Errorf("issue token: %w", err)
	}
	return token, nil
}
