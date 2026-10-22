package session_test

import (
	"context"
	"sync"
	"time"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

// ---- fake clock ----

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// ---- fake store ----

type fakeUser struct {
	username string
	admin    bool
	active   bool
}

// fakeStore meniru perilaku PGStore di memori, termasuk join ke users,
// pencarian lewat token lama, dan Rotate yang bersyarat.
type fakeStore struct {
	mu     sync.Mutex
	nextID int64
	users  map[int64]fakeUser
	rows   map[int64]*session.Record

	insertErr      error
	findErr        error
	rotateConflict bool // Rotate selalu kalah dari request lain
}

var _ session.Store = (*fakeStore)(nil)

func newFakeStore() *fakeStore {
	return &fakeStore{
		nextID: 1,
		users:  map[int64]fakeUser{},
		rows:   map[int64]*session.Record{},
	}
}

func (f *fakeStore) addUser(id int64, username string, admin bool) {
	f.users[id] = fakeUser{username: username, admin: admin, active: true}
}

func (f *fakeStore) Insert(_ context.Context, n session.NewRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return f.insertErr
	}
	f.rows[f.nextID] = &session.Record{
		ID:                f.nextID,
		UserID:            n.UserID,
		TokenHash:         n.TokenHash,
		CSRFSecret:        n.CSRFSecret,
		LastSeenAt:        n.Now,
		RotatedAt:         n.Now,
		AbsoluteExpiresAt: n.AbsoluteExpiresAt,
	}
	f.nextID++
	return nil
}

func (f *fakeStore) FindByTokenHash(_ context.Context, hash string) (session.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.findErr != nil {
		return session.Record{}, f.findErr
	}
	for _, r := range f.rows {
		if r.TokenHash != hash && (r.PreviousTokenHash == "" || r.PreviousTokenHash != hash) {
			continue
		}
		u, ok := f.users[r.UserID]
		if !ok {
			continue // seperti INNER JOIN
		}
		out := *r
		out.Username = u.username
		out.IsPlatformAdmin = u.admin
		out.IsActive = u.active
		return out, nil
	}
	return session.Record{}, session.ErrNotFound
}

func (f *fakeStore) Touch(_ context.Context, id int64, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[id]; ok {
		r.LastSeenAt = at
	}
	return nil
}

func (f *fakeStore) Rotate(_ context.Context, id int64, oldHash, newHash string, at, previousValidUntil time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rotateConflict {
		return false, nil
	}
	r, ok := f.rows[id]
	if !ok || r.TokenHash != oldHash || !r.RevokedAt.IsZero() {
		return false, nil
	}
	r.PreviousTokenHash = oldHash
	r.PreviousValidUntil = previousValidUntil
	r.TokenHash = newHash
	r.RotatedAt = at
	r.LastSeenAt = at
	return true, nil
}

func (f *fakeStore) Revoke(_ context.Context, tokenHash string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if (r.TokenHash == tokenHash || r.PreviousTokenHash == tokenHash) && r.RevokedAt.IsZero() {
			r.RevokedAt = at
		}
	}
	return nil
}

func (f *fakeStore) RevokeByUser(_ context.Context, userID int64, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.UserID == userID && r.RevokedAt.IsZero() {
			r.RevokedAt = at
		}
	}
	return nil
}
