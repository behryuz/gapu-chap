package domain

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrConflict           = errors.New("already exists")
	ErrValidation         = errors.New("validation failed")
	ErrInvalidCredentials = errors.New("invalid username or password")
)
