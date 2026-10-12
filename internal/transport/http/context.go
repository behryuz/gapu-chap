package http

import "context"

type AuthUser struct {
	ID       int64
	Username string
}

type ctxKey int

const authUserKey ctxKey = iota

func withAuthUser(ctx context.Context, u AuthUser) context.Context {
	return context.WithValue(ctx, authUserKey, u)
}

func authUserFromContext(ctx context.Context) (AuthUser, bool) {
	u, ok := ctx.Value(authUserKey).(AuthUser)
	return u, ok
}
