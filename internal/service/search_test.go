package service

import (
	"errors"
	"strings"
	"testing"

	"gapu-chap/internal/domain"
)

func TestBuildSearchQuery(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"привет", "привет:*"},
		{"  Привет,   МИР! ", "привет:* & мир:*"},
		{"go1.27", "go1:* & 27:*"},
		{"a & b | !c", "a:* & b:* & c:*"},
		{"'); DROP TABLE messages; --", "drop:* & table:* & messages:*"},
		{strings.Repeat("w ", 20), strings.TrimSuffix(strings.Repeat("w:* & ", 8), " & ")},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := buildSearchQuery(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSearchQuery_Invalid(t *testing.T) {
	for _, in := range []string{"", "   ", "!!! ???", strings.Repeat("я", 101)} {
		if _, err := buildSearchQuery(in); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("%q: expected ErrValidation, got %v", in, err)
		}
	}
}
