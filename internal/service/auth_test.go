package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gapu-chap/internal/domain"
	"gapu-chap/internal/service"
)

type fakeUserRepo struct {
	byUsername map[string]*domain.User
	nextID     int64
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byUsername: make(map[string]*domain.User)}
}

func (r *fakeUserRepo) Create(_ context.Context, u *domain.User) error {
	key := strings.ToLower(u.Username)
	if _, exists := r.byUsername[key]; exists {
		return domain.ErrConflict
	}
	r.nextID++
	u.ID = r.nextID
	u.CreatedAt = time.Now()

	stored := *u
	r.byUsername[key] = &stored
	return nil
}

func (r *fakeUserRepo) GetByUsername(_ context.Context, username string) (*domain.User, error) {
	u, ok := r.byUsername[strings.ToLower(username)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	found := *u
	return &found, nil
}

func (r *fakeUserRepo) GetByID(_ context.Context, id int64) (*domain.User, error) {
	for _, u := range r.byUsername {
		if u.ID == id {
			found := *u
			return &found, nil
		}
	}
	return nil, domain.ErrNotFound
}

type fakeHasher struct{}

func (fakeHasher) Hash(p string) (string, error) {
	return "hashed:" + p, nil
}

func (fakeHasher) Compare(hash, p string) bool {
	return hash == "hashed:"+p
}

type fakeTokens struct{}

func (fakeTokens) Issue(id int64, username string) (string, error) {
	return fmt.Sprintf("token:%d:%s", id, username), nil
}

func newTestAuthService() *service.AuthService {
	return service.NewAuthService(newFakeUserRepo(), fakeHasher{}, fakeTokens{})
}

func validInput() service.RegisterInput {
	return service.RegisterInput{FirstName: "John", Username: "john", Password: "secret123"}
}

func TestRegister_Success(t *testing.T) {
	svc := newTestAuthService()

	in := validInput()
	in.FirstName = "  John  "
	in.Username = "   john"

	user, err := svc.Register(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID == 0 {
		t.Errorf("user id should be set")
	}
	if user.FirstName != "John" || user.Username != "john" {
		t.Errorf("expected trimmed fields, got first_name=%q, user_name=%q", user.FirstName, user.Username)
	}
	if user.PasswordHash != "hashed:secret123" {
		t.Errorf("password should be hashed, got %q", user.PasswordHash)
	}
}

func TestRegister_Validation(t *testing.T) {
	tests := []struct {
		name   string
		modify func(in *service.RegisterInput)
	}{
		{"short username", func(in *service.RegisterInput) { in.Username = "jn" }},
		{"username with space", func(in *service.RegisterInput) { in.Username = "jo jn" }},
		{"cyrillic username", func(in *service.RegisterInput) { in.Username = "джон " }},
		{"empty first name", func(in *service.RegisterInput) { in.FirstName = "   " }},
		{"short password", func(in *service.RegisterInput) { in.Password = "1234567" }},
		{"password over 72 bytes", func(in *service.RegisterInput) { in.Password = strings.Repeat("a", 73) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc := newTestAuthService()
			in := validInput()
			test.modify(&in)

			_, err := svc.Register(t.Context(), in)
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("expected ErrValidation, got %v", err)
			}
		})
	}
}

func TestRegister_DuplicateUsernameCaseInsensitive(t *testing.T) {
	svc := newTestAuthService()

	if _, err := svc.Register(t.Context(), validInput()); err != nil {
		t.Fatalf("first register: %v", err)
	}

	dup := validInput()
	dup.Username = strings.ToUpper(dup.Username)
	_, err := svc.Register(t.Context(), dup)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name      string
		username  string
		password  string
		wantErr   error
		wantToken string
	}{
		{"success", "john", "secret123", nil, "token:1:john"},
		{"username in other case", "JOHN", "secret123", nil, "token:1:john"},
		{"wrong password", "john", "wrong-pass", domain.ErrInvalidCredentials, ""},
		{"unknown user", "nobody", "secret123", domain.ErrInvalidCredentials, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestAuthService()
			if _, err := svc.Register(t.Context(), validInput()); err != nil {
				t.Fatalf("register: %v", err)
			}

			token, err := svc.Login(t.Context(), tt.username, tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			if token != tt.wantToken {
				t.Errorf("expected token %q, got %q", tt.wantToken, token)
			}
		})
	}
}
