package service

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"gapu-chap/internal/domain"
)

const (
	maxSearchQueryLength = 100
	maxSearchTerms       = 8
)

var searchWordRe = regexp.MustCompile(`[\p{L}\p{N}]+`)

// buildSearchQuery превращает ввод пользователя в tsquery для PostgreSQL:
// «Привет  мир!» → «привет:* & мир:*» — все слова, каждое по началу.
//
// Синтаксис tsquery (&, |, !, скобки, кавычки) пользователю не доступен: мы оставляем
// только буквы и цифры, поэтому ни ошибку синтаксиса, ни «инъекцию» в запрос получить нельзя.
func buildSearchQuery(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(raw); n == 0 || n > maxSearchQueryLength {
		return "", fmt.Errorf("%w: search query must be 1-%d characters", domain.ErrValidation, maxSearchQueryLength)
	}

	words := searchWordRe.FindAllString(strings.ToLower(raw), maxSearchTerms)
	if len(words) == 0 {
		return "", fmt.Errorf("%w: search query must contain letters or digits", domain.ErrValidation)
	}

	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = w + ":*"
	}
	return strings.Join(terms, " & "), nil
}
