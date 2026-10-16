package domain

import "time"

type User struct {
	ID           int64
	FirstName    string
	LastName     string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

type OnlineUser struct {
	ID       int64
	Username string
}
