package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gapu-chap/internal/auth"
	"gapu-chap/internal/domain"
)

type fakeTokenParser struct{}

// Токен → id пользователя: valid — обычный, banned — заблокирован, deleted — удалён из базы.
var tokenUsers = map[string]int64{"valid-token": 7, "banned-token": 8, "deleted-token": 9}

func (fakeTokenParser) Parse(token string) (*auth.Claims, error) {
	id, ok := tokenUsers[token]
	if !ok {
		return nil, errors.New("invalid token")
	}
	return &auth.Claims{UserID: id, Username: "from-token"}, nil
}

type fakeAccounts struct{}

func (fakeAccounts) GetByID(_ context.Context, id int64) (*domain.User, error) {
	bannedAt := time.Now()
	switch id {
	case 7:
		return &domain.User{ID: 7, Username: "behruz", Role: domain.RoleModerator}, nil
	case 8:
		return &domain.User{ID: 8, Username: "banned", BannedAt: &bannedAt}, nil
	default:
		return nil, domain.ErrNotFound
	}
}

func TestRequireAuth(t *testing.T) {
	var gotUser AuthUser
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, _ = authUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAuth(fakeTokenParser{}, fakeAccounts{})(next)

	tests := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic dXNlcjpwYXNz", http.StatusUnauthorized},
		{"empty bearer", "Bearer ", http.StatusUnauthorized},
		{"invalid token", "Bearer bad-token", http.StatusUnauthorized},
		{"valid token", "Bearer valid-token", http.StatusOK},
		{"banned user", "Bearer banned-token", http.StatusForbidden},
		{"deleted user", "Bearer deleted-token", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUser = AuthUser{}
			req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}
			// Имя и роль берутся из базы, а не из токена.
			if tt.wantStatus == http.StatusOK &&
				(gotUser.ID != 7 || gotUser.Username != "behruz" || gotUser.Role != domain.RoleModerator) {
				t.Errorf("expected moderator behruz (7) in context, got %+v", gotUser)
			}
		})
	}
}
