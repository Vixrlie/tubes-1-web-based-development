package session_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

func newPGEnv(t *testing.T, mutate func(*session.Config)) (*session.Manager, *session.PGStore, *pgxpool.Pool, *fakeClock) {
	t.Helper()
	pool := testutil.NewPool(t) // melewati test bila -short
	cfg := defaultCfg()
	if mutate != nil {
		mutate(&cfg)
	}
	clock := newFakeClock()
	store := session.NewPGStore(pool)
	m, err := session.NewManager(store, cfg, clock)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m, store, pool, clock
}

func insertUser(t *testing.T, pool *pgxpool.Pool, username string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash) VALUES ($1, 'hash-dummy') RETURNING id`,
		username).Scan(&id)
	if err != nil {
		t.Fatalf("insert user %s: %v", username, err)
	}
	return id
}

func scalar[T any](t *testing.T, pool *pgxpool.Pool, q string, args ...any) T {
	t.Helper()
	var v T
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&v); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return v
}

func TestPG_Create_OnlyHashStored(t *testing.T) {
	ctx := context.Background()
	m, _, pool, clock := newPGEnv(t, nil)
	uid := insertUser(t, pool, "budi")

	tok, err := m.Create(ctx, uid)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	stored := scalar[string](t, pool, `SELECT token_hash FROM sessions WHERE user_id = $1`, uid)
	if stored != session.HashToken(tok) {
		t.Errorf("token_hash = %q, want SHA-256 dari token", stored)
	}
	if n := scalar[int64](t, pool, `SELECT count(*) FROM sessions WHERE token_hash = $1`, tok); n != 0 {
		t.Error("token asli ditemukan di kolom token_hash")
	}

	csrf := scalar[string](t, pool, `SELECT csrf_secret FROM sessions WHERE user_id = $1`, uid)
	if csrf == "" || csrf == tok {
		t.Errorf("csrf_secret harus terisi dan berbeda dari token: %q", csrf)
	}

	abs := scalar[time.Time](t, pool, `SELECT absolute_expires_at FROM sessions WHERE user_id = $1`, uid)
	if want := clock.Now().Add(8 * time.Hour); !abs.Equal(want) {
		t.Errorf("absolute_expires_at = %v, want %v", abs, want)
	}
}

func TestPG_Lifecycle_RotationAndGrace(t *testing.T) {
	ctx := context.Background()
	m, _, pool, clock := newPGEnv(t, nil)
	uid := insertUser(t, pool, "budi")

	old, err := m.Create(ctx, uid)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	absBefore := scalar[time.Time](t, pool, `SELECT absolute_expires_at FROM sessions WHERE user_id = $1`, uid)

	info, err := m.Validate(ctx, old)
	if err != nil || info.UserID != uid || info.Username != "budi" {
		t.Fatalf("Validate awal: info=%+v err=%v", info, err)
	}

	// Lewat interval rotasi.
	clock.Advance(16 * time.Minute)
	info, err = m.Validate(ctx, old)
	if err != nil || info.NewToken == "" {
		t.Fatalf("Validate harus merotasi: info=%+v err=%v", info, err)
	}
	newTok := info.NewToken

	cur := scalar[string](t, pool, `SELECT token_hash FROM sessions WHERE user_id = $1`, uid)
	prev := scalar[*string](t, pool, `SELECT previous_token_hash FROM sessions WHERE user_id = $1`, uid)
	absAfter := scalar[time.Time](t, pool, `SELECT absolute_expires_at FROM sessions WHERE user_id = $1`, uid)
	if cur != session.HashToken(newTok) {
		t.Error("token_hash harus hash dari token baru")
	}
	if prev == nil || *prev != session.HashToken(old) {
		t.Error("previous_token_hash harus hash dari token lama")
	}
	if !absAfter.Equal(absBefore) {
		t.Errorf("absolute_expires_at berubah (%v -> %v)", absBefore, absAfter)
	}

	// Dalam grace period: token lama dan baru sama-sama valid.
	clock.Advance(10 * time.Second)
	if info, err := m.Validate(ctx, old); err != nil || info.NewToken != "" {
		t.Fatalf("token lama dalam grace: info=%+v err=%v", info, err)
	}
	if _, err := m.Validate(ctx, newTok); err != nil {
		t.Fatalf("token baru: %v", err)
	}

	// Setelah grace period: token lama mati, token baru tetap hidup.
	clock.Advance(21 * time.Second) // 31 detik sejak rotasi
	_, err = m.Validate(ctx, old)
	wantInvalid(t, err)
	if _, err := m.Validate(ctx, newTok); err != nil {
		t.Fatalf("token baru harus tetap valid: %v", err)
	}
}

func TestPG_Validate_Rejected(t *testing.T) {
	ctx := context.Background()

	t.Run("lewat idle timeout", func(t *testing.T) {
		m, _, pool, clock := newPGEnv(t, nil)
		tok, _ := m.Create(ctx, insertUser(t, pool, "budi"))
		clock.Advance(31 * time.Minute)
		_, err := m.Validate(ctx, tok)
		wantInvalid(t, err)
	})

	t.Run("absolute expiration: valid sebelum, ditolak tepat di batas", func(t *testing.T) {
		m, _, pool, clock := newPGEnv(t, func(c *session.Config) {
			c.IdleTimeout = 24 * time.Hour
			c.RotateInterval = 24 * time.Hour
		})
		tok, _ := m.Create(ctx, insertUser(t, pool, "budi"))

		clock.Advance(7*time.Hour + 59*time.Minute)
		if _, err := m.Validate(ctx, tok); err != nil {
			t.Fatalf("harus valid di 7j59m: %v", err)
		}
		clock.Advance(time.Minute)
		_, err := m.Validate(ctx, tok)
		wantInvalid(t, err)
	})

	t.Run("user dinonaktifkan di database", func(t *testing.T) {
		m, _, pool, _ := newPGEnv(t, nil)
		uid := insertUser(t, pool, "budi")
		tok, _ := m.Create(ctx, uid)
		if _, err := m.Validate(ctx, tok); err != nil {
			t.Fatalf("sebelum dinonaktifkan: %v", err)
		}
		if _, err := pool.Exec(ctx, `UPDATE users SET is_active = false WHERE id = $1`, uid); err != nil {
			t.Fatalf("update user: %v", err)
		}
		_, err := m.Validate(ctx, tok)
		wantInvalid(t, err)
	})

	t.Run("token tidak dikenal dan kosong", func(t *testing.T) {
		m, _, pool, _ := newPGEnv(t, nil)
		_, _ = m.Create(ctx, insertUser(t, pool, "budi"))
		_, err := m.Validate(ctx, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ")
		wantInvalid(t, err)
		_, err = m.Validate(ctx, "")
		wantInvalid(t, err)
	})
}

func TestPG_Revoke(t *testing.T) {
	ctx := context.Background()

	t.Run("logout mencabut session dan idempotent", func(t *testing.T) {
		m, _, pool, _ := newPGEnv(t, nil)
		uid := insertUser(t, pool, "budi")
		tok, _ := m.Create(ctx, uid)

		if err := m.Revoke(ctx, tok); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		if err := m.Revoke(ctx, tok); err != nil {
			t.Fatalf("Revoke kedua: %v", err)
		}
		if err := m.Revoke(ctx, "token-asing"); err != nil {
			t.Fatalf("Revoke token asing: %v", err)
		}

		revoked := scalar[*time.Time](t, pool, `SELECT revoked_at FROM sessions WHERE user_id = $1`, uid)
		if revoked == nil {
			t.Fatal("revoked_at harus terisi")
		}
		_, err := m.Validate(ctx, tok)
		wantInvalid(t, err)
	})

	t.Run("RevokeSessionsByUser hanya mencabut milik user itu", func(t *testing.T) {
		m, _, pool, _ := newPGEnv(t, nil)
		budi := insertUser(t, pool, "budi")
		sari := insertUser(t, pool, "sari")
		b1, _ := m.Create(ctx, budi)
		b2, _ := m.Create(ctx, budi)
		s1, _ := m.Create(ctx, sari)

		if err := m.RevokeSessionsByUser(ctx, budi); err != nil {
			t.Fatalf("RevokeSessionsByUser: %v", err)
		}

		if n := scalar[int64](t, pool,
			`SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NOT NULL`, budi); n != 2 {
			t.Errorf("session budi yang dicabut = %d, want 2", n)
		}
		_, err := m.Validate(ctx, b1)
		wantInvalid(t, err)
		_, err = m.Validate(ctx, b2)
		wantInvalid(t, err)
		if _, err := m.Validate(ctx, s1); err != nil {
			t.Fatalf("session sari tidak boleh terdampak: %v", err)
		}
	})
}

func TestPGStore_Rotate_Conditional(t *testing.T) {
	ctx := context.Background()
	_, store, pool, clock := newPGEnv(t, nil)
	uid := insertUser(t, pool, "budi")
	now := clock.Now()

	if err := store.Insert(ctx, session.NewRecord{
		TokenHash: "h-old", UserID: uid, CSRFSecret: "c", Now: now, AbsoluteExpiresAt: now.Add(8 * time.Hour),
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	rec, err := store.FindByTokenHash(ctx, "h-old")
	if err != nil {
		t.Fatalf("FindByTokenHash: %v", err)
	}

	t.Run("rotasi pertama menang, yang kedua dengan token lama kalah", func(t *testing.T) {
		ok, err := store.Rotate(ctx, rec.ID, "h-old", "h-new1", now, now.Add(30*time.Second))
		if err != nil || !ok {
			t.Fatalf("Rotate pertama: ok=%v err=%v", ok, err)
		}
		ok, err = store.Rotate(ctx, rec.ID, "h-old", "h-new2", now, now.Add(30*time.Second))
		if err != nil || ok {
			t.Fatalf("Rotate kedua harus kalah: ok=%v err=%v", ok, err)
		}
		if cur := scalar[string](t, pool, `SELECT token_hash FROM sessions WHERE id = $1`, rec.ID); cur != "h-new1" {
			t.Fatalf("token_hash = %q, want h-new1", cur)
		}
	})

	t.Run("token lama masih bisa ditemukan sebagai previous", func(t *testing.T) {
		got, err := store.FindByTokenHash(ctx, "h-old")
		if err != nil {
			t.Fatalf("FindByTokenHash: %v", err)
		}
		if got.ID != rec.ID || got.TokenHash != "h-new1" || got.PreviousTokenHash != "h-old" {
			t.Fatalf("record tidak sesuai: %+v", got)
		}
	})

	t.Run("session yang dicabut tidak bisa dirotasi", func(t *testing.T) {
		if err := store.Revoke(ctx, "h-new1", now); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		ok, err := store.Rotate(ctx, rec.ID, "h-new1", "h-new3", now, now.Add(30*time.Second))
		if err != nil || ok {
			t.Fatalf("Rotate pada session dicabut: ok=%v err=%v", ok, err)
		}
	})
}

func TestPGStore_FindByTokenHash_Unknown(t *testing.T) {
	_, store, _, _ := newPGEnv(t, nil)
	_, err := store.FindByTokenHash(context.Background(), "tidak-ada")
	if err != session.ErrNotFound {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestPG_ConcurrentRotation(t *testing.T) {
	ctx := context.Background()
	m, _, pool, clock := newPGEnv(t, nil)
	tok, err := m.Create(ctx, insertUser(t, pool, "budi"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	clock.Advance(16 * time.Minute)

	const n = 8
	infos := make(chan session.Info, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info, err := m.Validate(ctx, tok)
			if err != nil {
				errs <- err
				return
			}
			infos <- info
		}()
	}
	wg.Wait()
	close(infos)
	close(errs)

	for err := range errs {
		t.Errorf("semua request paralel harus valid, got %v", err)
	}
	rotated := 0
	for info := range infos {
		if info.NewToken != "" {
			rotated++
		}
	}
	if rotated != 1 {
		t.Fatalf("jumlah request yang menang rotasi = %d, want 1", rotated)
	}
}
