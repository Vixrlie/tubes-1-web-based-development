package auth

import (
	"context"
	"errors"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

// SessionSource adalah bagian dari session.Manager yang dibutuhkan adapter.
type SessionSource interface {
	Validate(ctx context.Context, token string) (session.Info, error)
}

type sessionAdapter struct {
	src SessionSource
}

// NewSessionValidator membungkus session.Manager menjadi SessionValidator
// untuk AuthMiddleware. Paket session tidak meng-import auth, jadi
// penerjemahan tipe dilakukan di sini.
func NewSessionValidator(src SessionSource) SessionValidator {
	return sessionAdapter{src: src}
}

func (a sessionAdapter) Validate(ctx context.Context, token string) (SessionInfo, error) {
	info, err := a.src.Validate(ctx, token)
	if errors.Is(err, session.ErrInvalid) {
		return SessionInfo{}, ErrInvalidSession
	}
	if err != nil {
		return SessionInfo{}, err
	}
	return SessionInfo{
		User: User{
			ID:              info.UserID,
			Username:        info.Username,
			IsPlatformAdmin: info.IsPlatformAdmin,
			IsActive:        true, // Manager sudah menolak user nonaktif
		},
		CSRFToken: info.CSRFToken,
		NewToken:  info.NewToken,
	}, nil
}
