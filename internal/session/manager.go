package session

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrInvalid return untuk semua alasan invalid session
var ErrInvalid = errors.New("session tidak valid")

type Config struct {
	IdleTimeout     time.Duration
	AbsoluteTimeout time.Duration
	RotateInterval  time.Duration
	GracePeriod     time.Duration
}

func (c Config) validate() error {
	switch {
	case c.IdleTimeout <= 0:
		return errors.New("idle timeout harus > 0")
	case c.AbsoluteTimeout <= 0:
		return errors.New("absolute timeout harus > 0")
	case c.RotateInterval <= 0:
		return errors.New("rotate interval harus > 0")
	case c.GracePeriod <= 0:
		return errors.New("grace period harus > 0")
	}
	return nil
}

// Info adalah hasil validasi. Packet auth translate ke auth.SessionInfo
type Info struct {
	UserID          int64
	Username        string
	IsPlatformAdmin bool
	CSRFToken       string
	// NewToken terisi jika session baru dirotasi
	NewToken string
}

type Manager struct {
	store Store
	cfg   Config
	clock Clock
}

var _ Revoker = (*Manager)(nil)

// NewManager membuat Manager. clock bisa nil
func NewManager(store Store, cfg Config, clock Clock) (*Manager, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("konfigurasi session: %w", err)
	}
	if clock == nil {
		clock = systemClock{}
	}
	return &Manager{store: store, cfg: cfg, clock: clock}, nil
}

// Create buat session baru untuk user dan return token yang
// harus ditaruh di cookie. Pemanggil wajib sudah verify
// kredensial dan memastikan user aktif.
func (m *Manager) Create(ctx context.Context, userID int64) (string, error) {
	token, err := NewToken()
	if err != nil {
		return "", err
	}
	csrf, err := randomString(32)
	if err != nil {
		return "", err
	}
	now := m.clock.Now()
	err = m.store.Insert(ctx, NewRecord{
		TokenHash:         HashToken(token),
		UserID:            userID,
		CSRFSecret:        csrf,
		Now:               now,
		AbsoluteExpiresAt: now.Add(m.cfg.AbsoluteTimeout),
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// Validate memeriksa token. Mengembalikan ErrInvalid untuk session yang tidak
// sah, dan error lain hanya untuk kegagalan infrastruktur.
func (m *Manager) Validate(ctx context.Context, token string) (Info, error) {
	if token == "" {
		return Info{}, ErrInvalid
	}
	now := m.clock.Now()
	hash := HashToken(token)

	rec, err := m.store.FindByTokenHash(ctx, hash)
	if errors.Is(err, ErrNotFound) {
		return Info{}, ErrInvalid
	}
	if err != nil {
		return Info{}, err
	}

	if !rec.RevokedAt.IsZero() || !rec.IsActive {
		return Info{}, ErrInvalid
	}
	if !now.Before(rec.AbsoluteExpiresAt) {
		return Info{}, ErrInvalid
	}
	if !now.Before(rec.LastSeenAt.Add(m.cfg.IdleTimeout)) {
		return Info{}, ErrInvalid
	}

	// Token yang cocok adalah token lama (sudah dirotasi)? Hanya valid di masa grace.
	usingPrevious := rec.TokenHash != hash
	if usingPrevious {
		if rec.PreviousValidUntil.IsZero() || !now.Before(rec.PreviousValidUntil) {
			return Info{}, ErrInvalid
		}
	}

	info := Info{
		UserID:          rec.UserID,
		Username:        rec.Username,
		IsPlatformAdmin: rec.IsPlatformAdmin,
		CSRFToken:       rec.CSRFSecret,
	}

	// Force rotate setelah privilege escalation, tanpa logout
	// Hanya dari current token saat ini
	if !usingPrevious && rec.ForceRotate {
		newToken, err := NewToken()
		if err != nil {
			return Info{}, err
		}
		rotated, err := m.store.Rotate(ctx, rec.ID, hash, HashToken(newToken), now, now.Add(m.cfg.GracePeriod))
		if err != nil {
			return Info{}, err
		}
		if rotated {
			info.NewToken = newToken
			return info, nil
		}
		// Keduluan request lain: lanjut dengan token lama selama masih dalam grace.
	}

	// Rotasi berkala: hanya dari current token, dan tidak ubah absolute expiration.
	if !usingPrevious && !now.Before(rec.RotatedAt.Add(m.cfg.RotateInterval)) {
		newToken, err := NewToken()
		if err != nil {
			return Info{}, err
		}
		rotated, err := m.store.Rotate(ctx, rec.ID, hash, HashToken(newToken), now, now.Add(m.cfg.GracePeriod))
		if err != nil {
			return Info{}, err
		}
		if rotated {
			info.NewToken = newToken
			return info, nil
		}
		// Keduluan request lain: token kita kini token lama di masa grace. Lanjut biasa.
	}

	if err := m.store.Touch(ctx, rec.ID, now); err != nil {
		return Info{}, err
	}
	return info, nil
}

// Revoke menonaktifkan session (logout)
func (m *Manager) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return m.store.Revoke(ctx, HashToken(token), m.clock.Now())
}

// RevokeSessionsByUser cabut semua session aktif milik user
func (m *Manager) RevokeSessionsByUser(ctx context.Context, userID int64) error {
	return m.store.RevokeByUser(ctx, userID, m.clock.Now())
}

// ForceRotateSessionsByUser marks session user spy dirotasi di next req
func (m *Manager) ForceRotateSessionsByUser(ctx context.Context, userID int64) error {
	return m.store.ForceRotateByUser(ctx, userID)
}
