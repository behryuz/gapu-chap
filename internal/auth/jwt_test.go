package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-at-least-32-bytes-long!!"

func TestTokenManager_IssueAndParse(t *testing.T) {
	m := NewTokenManager(testSecret, time.Hour)

	token, err := m.Issue(42, "john")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	claims, err := m.Parse(token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != 42 || claims.Username != "john" {
		t.Errorf("unexpected claims: %+v", claims)
	}
}

func TestTokenManager_Expired(t *testing.T) {
	m := NewTokenManager(testSecret, -time.Minute) // токен истёк ещё при выпуске

	token, err := m.Issue(1, "john")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	_, err = m.Parse(token)
	if !errors.Is(err, jwt.ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestTokenManager_WrongSecret(t *testing.T) {
	issuer := NewTokenManager(testSecret, time.Hour)
	attacker := NewTokenManager("another-secret-also-32-bytes-long!!!", time.Hour)

	token, err := attacker.Issue(1, "admin")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	_, err = issuer.Parse(token)
	if !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
		t.Fatalf("expected ErrTokenSignatureInvalid, got %v", err)
	}
}

func TestTokenManager_RejectsAlgNone(t *testing.T) {
	m := NewTokenManager(testSecret, time.Hour)

	claims := Claims{
		UserID:    1,
		Username:  "admin",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("build unsigned token: %v", err)
	}

	if _, err := m.Parse(token); err == nil {
		t.Fatal("token with alg=none must be rejected")
	}
}
