package session

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeClock adalah waktu yang bisa digeser test.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time                  { return c.t }
func (c *fakeClock) advance(d time.Duration)         { c.t = c.t.Add(d) }

// fakeStore menyimpan Record di memori, meniru perilaku PGStore.
type fakeStore struct {
	records map[int64]Record
	nextID  int64
}

func newFakeStore() *fakeStore { return &fakeStore{records: map[int64]Record{}} }

func (f *fakeStore) Insert(ctx context.Context, n NewRecord) error {
	f.nextID++
	f.records[f.nextID] = Record{
		ID:                 f.nextID,
		UserID:             n.UserID,
		Username:           "alice",
		IsActive:           true,
		TokenHash:          n.TokenHash,
		CSRFSecret:         n.CSRFSecret,
		LastSeenAt:         n.Now,
		RotatedAt:          n.Now,
		AbsoluteExpiresAt:  n.AbsoluteExpiresAt,
	}
	return nil
}

func (f *fakeStore) FindByTokenHash(ctx context.Context, hash string) (Record, error) {
	for _, r := range f.records {
		if r.TokenHash == hash || (r.PreviousTokenHash != "" && r.PreviousTokenHash == hash) {
			return r, nil
		}
	}
	return Record{}, ErrNotFound
}

func (f *fakeStore) Touch(ctx context.Context, id int64, at time.Time) error {
	r := f.records[id]
	r.LastSeenAt = at
	f.records[id] = r
	return nil
}

func (f *fakeStore) Rotate(ctx context.Context, id int64, oldHash, newHash string, at, previousValidUntil time.Time) (bool, error) {
	r := f.records[id]
	if r.TokenHash != oldHash || !r.RevokedAt.IsZero() {
		return false, nil
	}
	r.PreviousTokenHash = oldHash
	r.PreviousValidUntil = previousValidUntil
	r.TokenHash = newHash
	r.RotatedAt = at
	r.LastSeenAt = at
	f.records[id] = r
	return true, nil
}

func (f *fakeStore) Revoke(ctx context.Context, tokenHash string, at time.Time) error {
	for id, r := range f.records {
		if r.TokenHash == tokenHash || (r.PreviousTokenHash != "" && r.PreviousTokenHash == tokenHash) {
			r.RevokedAt = at
			f.records[id] = r
		}
	}
	return nil
}

func (f *fakeStore) RevokeByUser(ctx context.Context, userID int64, at time.Time) error {
	for id, r := range f.records {
		if r.UserID == userID {
			r.RevokedAt = at
			f.records[id] = r
		}
	}
	return nil
}

func (f *fakeStore) ForceRotateByUser(ctx context.Context, userID int64) error {
	for id, r := range f.records {
		if r.UserID == userID && r.RevokedAt.IsZero() {
			r.ForceRotate = true
			f.records[id] = r
		}
	}
	return nil
}

var testCfg = Config{
	IdleTimeout:     30 * time.Minute,
	AbsoluteTimeout: 8 * time.Hour,
	RotateInterval:  15 * time.Minute,
	GracePeriod:     30 * time.Second,
}

func newTestManager(t *testing.T) (*Manager, *fakeStore, *fakeClock) {
	t.Helper()
	store := newFakeStore()
	clock := &fakeClock{t: time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)}
	m, err := NewManager(store, testCfg, clock)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m, store, clock
}

func TestNewManagerRejectsInvalidConfig(t *testing.T) {
	bad := []Config{
		{IdleTimeout: 0, AbsoluteTimeout: time.Hour, RotateInterval: time.Minute, GracePeriod: time.Second},
		{IdleTimeout: time.Minute, AbsoluteTimeout: 0, RotateInterval: time.Minute, GracePeriod: time.Second},
		{IdleTimeout: time.Minute, AbsoluteTimeout: time.Hour, RotateInterval: 0, GracePeriod: time.Second},
		{IdleTimeout: time.Minute, AbsoluteTimeout: time.Hour, RotateInterval: time.Minute, GracePeriod: 0},
	}
	for i, cfg := range bad {
		if _, err := NewManager(newFakeStore(), cfg, nil); err == nil {
			t.Errorf("konfigurasi invalid %d diterima", i)
		}
	}
}

func TestCreateAndValidate(t *testing.T) {
	m, _, _ := newTestManager(t)
	token, err := m.Create(context.Background(), 42)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(token) < 40 {
		t.Errorf("token terlalu pendek: %d karakter", len(token))
	}

	info, err := m.Validate(context.Background(), token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if info.UserID != 42 || info.Username != "alice" {
		t.Errorf("info = %+v, want user 42/alice", info)
	}
	if info.CSRFToken == "" {
		t.Error("CSRF token kosong")
	}
	if info.NewToken != "" {
		t.Error("rotasi terjadi padahal belum jatuh tempo")
	}
}

func TestValidateRejectsEmptyAndUnknown(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.Validate(context.Background(), ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("token kosong: err = %v, want ErrInvalid", err)
	}
	if _, err := m.Validate(context.Background(), "token-ngawur"); !errors.Is(err, ErrInvalid) {
		t.Errorf("token tak dikenal: err = %v, want ErrInvalid", err)
	}
}

func TestIdleTimeout(t *testing.T) {
	m, _, clock := newTestManager(t)
	token, _ := m.Create(context.Background(), 1)

	clock.advance(29 * time.Minute)
	if _, err := m.Validate(context.Background(), token); err != nil {
		t.Fatalf("29 menit (idle 30) seharusnya masih valid: %v", err)
	}
	clock.advance(2 * time.Minute) // total 31 menit tanpa aktivitas
	if _, err := m.Validate(context.Background(), token); !errors.Is(err, ErrInvalid) {
		t.Fatalf("31 menit: err = %v, want ErrInvalid (idle timeout)", err)
	}
}

func TestIdleTimeoutResetByActivity(t *testing.T) {
	m, _, clock := newTestManager(t)
	token, _ := m.Create(context.Background(), 1)

	// Aktif tiap 20 menit selama 3 jam: tidak pernah idle, tapi masih dalam
	// absolute 8 jam. Ikuti token baru saat rotasi (seperti cookie klien).
	for i := 0; i < 9; i++ {
		clock.advance(20 * time.Minute)
		info, err := m.Validate(context.Background(), token)
		if err != nil {
			t.Fatalf("aktif %d: %v", i, err)
		}
		if info.NewToken != "" {
			token = info.NewToken
		}
	}
}

func TestAbsoluteExpiration(t *testing.T) {
	m, _, clock := newTestManager(t)
	token, _ := m.Create(context.Background(), 1)

	// Aktif terus tiap 20 menit melewati 8 jam dengan mengikuti rotasi:
	// absolute expiration tetap menghentikan meski session aktif.
	deadline := clock.t.Add(testCfg.AbsoluteTimeout)
	for clock.advance(20 * time.Minute); clock.t.Before(deadline.Add(time.Hour)); clock.advance(20 * time.Minute) {
		info, err := m.Validate(context.Background(), token)
		if err == nil && info.NewToken != "" {
			token = info.NewToken
		}
		if clock.t.Before(deadline) {
			if err != nil {
				t.Fatalf("sebelum absolute (%s): %v", clock.t, err)
			}
			continue
		}
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("setelah absolute: err = %v, want ErrInvalid", err)
		}
		break
	}
}

func TestPeriodicRotation(t *testing.T) {
	m, store, clock := newTestManager(t)
	clock0 := clock.t
	token, _ := m.Create(context.Background(), 1)
	oldHash := HashToken(token)

	clock.advance(16 * time.Minute) // melewati rotate interval 15 menit
	info, err := m.Validate(context.Background(), token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if info.NewToken == "" {
		t.Fatal("rotasi berkala tidak terjadi")
	}
	if info.NewToken == token {
		t.Fatal("token baru identik dengan token lama")
	}

	// Token lama masih valid selama grace period 30 detik.
	if _, err := m.Validate(context.Background(), token); err != nil {
		t.Fatalf("token lama di masa grace: %v", err)
	}
	// Token baru langsung valid.
	if _, err := m.Validate(context.Background(), info.NewToken); err != nil {
		t.Fatalf("token baru: %v", err)
	}

	// Lewati grace period: token lama mati.
	clock.advance(31 * time.Second)
	if _, err := m.Validate(context.Background(), token); !errors.Is(err, ErrInvalid) {
		t.Fatalf("token lama lewat grace: err = %v, want ErrInvalid", err)
	}

	// Rotasi tidak memperpanjang absolute expiration.
	var rec Record
	for _, r := range store.records {
		rec = r
	}
	wantAbsolute := clock0.Add(testCfg.AbsoluteTimeout)
	if !rec.AbsoluteExpiresAt.Equal(wantAbsolute) {
		t.Errorf("absolute = %v, want %v (tidak berubah saat rotasi)", rec.AbsoluteExpiresAt, wantAbsolute)
	}
	if rec.PreviousTokenHash != oldHash {
		t.Errorf("previous hash tidak tersimpan")
	}
}

func TestRevokeOnLogout(t *testing.T) {
	m, _, _ := newTestManager(t)
	token, _ := m.Create(context.Background(), 1)

	if err := m.Revoke(context.Background(), token); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := m.Validate(context.Background(), token); !errors.Is(err, ErrInvalid) {
		t.Fatalf("setelah revoke: err = %v, want ErrInvalid", err)
	}
	// Logout dua kali bukan error (idempoten).
	if err := m.Revoke(context.Background(), token); err != nil {
		t.Errorf("revoke kedua: %v", err)
	}
	if err := m.Revoke(context.Background(), ""); err != nil {
		t.Errorf("revoke token kosong: %v", err)
	}
}

func TestRevokeSessionsByUser(t *testing.T) {
	m, _, clock := newTestManager(t)
	tok1, _ := m.Create(context.Background(), 7)
	tok2, _ := m.Create(context.Background(), 7)
	tokOther, _ := m.Create(context.Background(), 8)

	if err := m.RevokeSessionsByUser(context.Background(), 7); err != nil {
		t.Fatalf("RevokeSessionsByUser: %v", err)
	}
	for _, tok := range []string{tok1, tok2} {
		if _, err := m.Validate(context.Background(), tok); !errors.Is(err, ErrInvalid) {
			t.Errorf("session user 7 masih valid setelah pencabutan")
		}
	}
	if _, err := m.Validate(context.Background(), tokOther); err != nil {
		t.Errorf("session user 8 ikut tercabut: %v", err)
	}
	_ = clock
}

func TestInactiveUserRejected(t *testing.T) {
	m, store, _ := newTestManager(t)
	token, _ := m.Create(context.Background(), 1)

	for id, r := range store.records {
		r.IsActive = false
		store.records[id] = r
	}
	if _, err := m.Validate(context.Background(), token); !errors.Is(err, ErrInvalid) {
		t.Fatalf("user nonaktif: err = %v, want ErrInvalid", err)
	}
}
