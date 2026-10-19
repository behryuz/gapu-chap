package domain

import "time"

type Role string

const (
	RoleUser      Role = "user"
	RoleModerator Role = "moderator"
)

type User struct {
	ID           int64
	FirstName    string
	LastName     string
	Username     string
	PasswordHash string
	Role         Role
	BannedAt     *time.Time
	CreatedAt    time.Time
}

func (u *User) IsModerator() bool {
	return u.Role == RoleModerator
}

func (u *User) IsBanned() bool {
	return u.BannedAt != nil
}

type OnlineUser struct {
	ID       int64
	Username string
}
