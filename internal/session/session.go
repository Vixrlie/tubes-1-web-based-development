package session

import "context"

type csrfKey struct{}

// WithCSRFToken menyimpan token CSRF session ke context (dipanggil middleware).
func WithCSRFToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfKey{}, token)
}

// CSRFToken dipakai template untuk mengisi hidden field di form POST.
func CSRFToken(ctx context.Context) string {
	t, _ := ctx.Value(csrfKey{}).(string)
	return t
}

// Revoker dipakai Vincent saat akses atau keanggotaan group berubah,
// dan saat admin menonaktifkan user.
type Revoker interface {
	RevokeSessionsByUser(ctx context.Context, userID int64) error
}
