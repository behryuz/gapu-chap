package domain

import "time"

type Message struct {
	ID             int64
	RoomID         int64
	AuthorID       int64
	AuthorUsername string
	Body           string
	CreatedAt      time.Time
}
