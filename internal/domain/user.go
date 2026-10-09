package domain

import "time"

type User struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
	Password  string
	CreatedAt time.Time
}
