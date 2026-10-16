package domain

import "time"

type Room struct {
	ID        int64
	Name      string
	OwnerID   int64
	CreatedAt time.Time
}

func (r *Room) IsOwnedBy(userID int64) bool {
	return r.OwnerID == userID
}

type Member struct {
	UserID            int64
	Username          string
	FirstName         string
	LastName          string
	JoinedAt          time.Time
	LastReadMessageID int64
}

type UserRoom struct {
	Room
	LastReadMessageID int64
	UnreadCount       int64
}
