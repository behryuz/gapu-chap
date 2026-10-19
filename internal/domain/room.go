package domain

import (
	"fmt"
	"time"
)

type RoomKind string

const (
	RoomKindPublic RoomKind = "public"
	RoomKindDirect RoomKind = "direct"
)

type Room struct {
	ID        int64
	Name      string
	OwnerID   int64
	Kind      RoomKind
	CreatedAt time.Time
}

func (r *Room) IsOwnedBy(userID int64) bool {
	return r.OwnerID == userID
}

func (r *Room) IsDirect() bool {
	return r.Kind == RoomKindDirect
}

// DirectKey — канонический ключ пары: порядок участников не важен,
// поэтому DirectKey(3, 7) == DirectKey(7, 3).
func DirectKey(a, b int64) string {
	return fmt.Sprintf("dm:%d:%d", min(a, b), max(a, b))
}

type Member struct {
	UserID            int64
	Username          string
	FirstName         string
	LastName          string
	JoinedAt          time.Time
	LastReadMessageID int64
}

type UserRef struct {
	ID       int64
	Username string
}

type UserRoom struct {
	Room
	LastReadMessageID int64
	UnreadCount       int64
	// Peer — собеседник; заполнен только для личных диалогов.
	Peer *UserRef
}
