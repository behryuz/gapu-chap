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

type ReadReceipt struct {
	RoomID    int64
	UserID    int64
	MessageID int64
}
