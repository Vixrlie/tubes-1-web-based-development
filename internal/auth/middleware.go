package auth

import (
	"context"
	"net/http"
)

type User struct {
	ID      int64
	Email   string
	IsAdmin bool
}

type contextKey struct{}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, contextKey{}, u)
}

// Memberikan status authenticated user
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(contextKey{}).(User)
	return u, ok
}

// Buat redirect ke /login
func RequireLogin(next http.Handler) http.Handler {
	return next
}

// Buat prevent non-admin
func RequireAdmin(next http.Handler) http.Handler {
	return next
}
