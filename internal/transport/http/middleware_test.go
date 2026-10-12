package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"gapu-chap/internal/auth"
)

type fakeTokenParser struct{}

func (fakeTokenParser) Parse(token string) (*auth.Claims, error) {
	if token != "valid-token" {
		return nil, errors.New("invalid token")
	}
	return &auth.Claims{UserID: 7, Username: "behruz"}, nil
}

func TestRequireAuth(t *testing.T) {
	var gotUser AuthUser
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, _ = authUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAuth(fakeTokenParser{})(next)

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
			if tt.wantStatus == http.StatusOK && gotUser.ID != 7 {
				t.Errorf("expected user 7 in context, got %+v", gotUser)
			}
		})
	}
}
