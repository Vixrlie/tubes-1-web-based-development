package session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

func defaultCfg() session.Config {
	return session.Config{
		IdleTimeout:     30 * time.Minute,
		AbsoluteTimeout: 8 * time.Hour,
		RotateInterval:  15 * time.Minute,
		GracePeriod:     30 * time.Second,
	}
}

// ---- harness ----

type env struct {
	t     *testing.T
	m     *session.Manager
	store *fakeStore
	clock *fakeClock
}

func newEnv(t *testing.T, mutate func(*session.Config)) *env {
	t.Helper()
	cfg := defaultCfg()
	if mutate != nil {
		mutate(&cfg)
	}
	store := newFakeStore()
	store.addUser(1, "budi", false)
	store.addUser(2, "sari", false)
	clock := newFakeClock()
	m, err := session.NewManager(store, cfg, clock)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return &env{t: t, m: m, store: store, clock: clock}
}

func (e *env) login(userID int64) string {
	e.t.Helper()
	tok, err := e.m.Create(context.Background(), userID)
	if err != nil {
		e.t.Fatalf("Create: %v", err)
	}
	return tok
}

func (e *env) validate(token string) (session.Info, error) {
	return e.m.Validate(context.Background(), token)
}

func (e *env) mustValidate(token string) session.Info {
	e.t.Helper()
	info, err := e.validate(token)
	if err != nil {
		e.t.Fatalf("Validate harus berhasil, got %v", err)
	}
	return info
}

// row mengembalikan session pertama yang dibuat (id 1).
func (e *env) row() *session.Record { return e.store.rows[1] }

func wantInvalid(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, session.ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}

// ---- NewManager ----

func TestNewManager_ConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*session.Config)
	}{
		{"idle timeout nol", func(c *session.Config) { c.IdleTimeout = 0 }},
		{"absolute timeout nol", func(c *session.Config) { c.AbsoluteTimeout = 0 }},
		{"rotate interval nol", func(c *session.Config) { c.RotateInterval = 0 }},
		{"grace period nol", func(c *session.Config) { c.GracePeriod = 0 }},
		{"durasi negatif", func(c *session.Config) { c.IdleTimeout = -time.Minute }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultCfg()
			tc.mutate(&cfg)
			if _, err := session.NewManager(newFakeStore(), cfg, newFakeClock()); err == nil {
				t.Fatal("want error, got nil")
			}
		})
	}

	t.Run("konfigurasi valid dan clock nil memakai waktu sistem", func(t *testing.T) {
		if _, err := session.NewManager(newFakeStore(), defaultCfg(), nil); err != nil {
			t.Fatalf("NewManager: %v", err)
		}
	})
}

// ---- Create ----

func TestCreate(t *testing.T) {
	t.Run("hanya hash token yang disimpan", func(t *testing.T) {
		e := newEnv(t, nil)
		tok := e.login(1)

		if len(tok) != 43 {
			t.Errorf("panjang token = %d, want 43", len(tok))
		}
		row := e.row()
		if row.TokenHash != session.HashToken(tok) {
			t.Error("TokenHash harus SHA-256 dari token")
		}
		if row.TokenHash == tok {
			t.Error("token asli tidak boleh disimpan")
		}
		if row.CSRFSecret == "" || row.CSRFSecret == tok {
			t.Errorf("CSRFSecret harus terisi dan berbeda dari token: %q", row.CSRFSecret)
		}
	})

	t.Run("absolute expiry = waktu login + absolute timeout", func(t *testing.T) {
		e := newEnv(t, nil)
		e.login(1)
		want := e.clock.Now().Add(8 * time.Hour)
		if !e.row().AbsoluteExpiresAt.Equal(want) {
			t.Errorf("AbsoluteExpiresAt = %v, want %v", e.row().AbsoluteExpiresAt, want)
		}
	})

	t.Run("dua login menghasilkan token dan CSRF berbeda", func(t *testing.T) {
		e := newEnv(t, nil)
		a, b := e.login(1), e.login(1)
		if a == b {
			t.Fatal("token kembar")
		}
		if e.store.rows[1].CSRFSecret == e.store.rows[2].CSRFSecret {
			t.Fatal("CSRF secret kembar")
		}
	})

	t.Run("kegagalan store diteruskan", func(t *testing.T) {
		e := newEnv(t, nil)
		boom := errors.New("db mati")
		e.store.insertErr = boom
		if _, err := e.m.Create(context.Background(), 1); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})
}

// ---- Validate: berhasil ----

func TestValidate_Success(t *testing.T) {
	t.Run("token valid: info benar dan last_seen_at naik", func(t *testing.T) {
		e := newEnv(t, nil)
		tok := e.login(1)
		e.clock.Advance(time.Minute)

		info := e.mustValidate(tok)

		if info.UserID != 1 || info.Username != "budi" || info.IsPlatformAdmin {
			t.Errorf("info tidak sesuai: %+v", info)
		}
		if info.CSRFToken == "" || info.CSRFToken != e.row().CSRFSecret {
			t.Errorf("CSRFToken = %q, want secret milik session", info.CSRFToken)
		}
		if info.NewToken != "" {
			t.Error("belum waktunya rotasi, NewToken harus kosong")
		}
		if !e.row().LastSeenAt.Equal(e.clock.Now()) {
			t.Errorf("LastSeenAt = %v, want %v", e.row().LastSeenAt, e.clock.Now())
		}
	})

	t.Run("flag Platform Admin diteruskan", func(t *testing.T) {
		e := newEnv(t, nil)
		e.store.addUser(3, "root", true)
		tok := e.login(3)
		if info := e.mustValidate(tok); !info.IsPlatformAdmin {
			t.Fatal("IsPlatformAdmin harus true")
		}
	})

	t.Run("aktivitas memperpanjang idle timeout", func(t *testing.T) {
		e := newEnv(t, func(c *session.Config) { c.RotateInterval = 24 * time.Hour })
		tok := e.login(1)
		e.clock.Advance(20 * time.Minute)
		e.mustValidate(tok)
		e.clock.Advance(20 * time.Minute) // 40 menit sejak login, 20 menit sejak aktivitas terakhir
		e.mustValidate(tok)
	})

	t.Run("aktif terus sampai sebelum absolute expiration", func(t *testing.T) {
		e := newEnv(t, func(c *session.Config) {
			c.IdleTimeout = 24 * time.Hour
			c.RotateInterval = 24 * time.Hour
		})
		tok := e.login(1)
		for i := 0; i < 23; i++ { // sampai 7 jam 40 menit
			e.clock.Advance(20 * time.Minute)
			e.mustValidate(tok)
		}
	})
}

// ---- Validate: ditolak ----

func TestValidate_Rejected(t *testing.T) {
	longLived := func(c *session.Config) {
		c.IdleTimeout = 24 * time.Hour
		c.RotateInterval = 24 * time.Hour
	}

	tests := []struct {
		name   string
		mutate func(*session.Config)
		setup  func(e *env) string // mengembalikan token yang akan divalidasi
	}{
		{
			name: "lewat idle timeout (31 menit)",
			setup: func(e *env) string {
				tok := e.login(1)
				e.clock.Advance(31 * time.Minute)
				return tok
			},
		},
		{
			name: "tepat di batas idle timeout (30 menit) sudah kedaluwarsa",
			setup: func(e *env) string {
				tok := e.login(1)
				e.clock.Advance(30 * time.Minute)
				return tok
			},
		},
		{
			name:   "lewat absolute expiration meski terus aktif",
			mutate: longLived,
			setup: func(e *env) string {
				tok := e.login(1)
				for i := 0; i < 23; i++ {
					e.clock.Advance(20 * time.Minute)
					e.mustValidate(tok) // aktif terus
				}
				e.clock.Advance(20 * time.Minute) // tepat 8 jam sejak login
				return tok
			},
		},
		{
			name: "session dicabut (logout)",
			setup: func(e *env) string {
				tok := e.login(1)
				if err := e.m.Revoke(context.Background(), tok); err != nil {
					e.t.Fatalf("Revoke: %v", err)
				}
				return tok
			},
		},
		{
			name: "session dicabut lewat RevokeSessionsByUser (akses dicabut)",
			setup: func(e *env) string {
				tok := e.login(1)
				if err := e.m.RevokeSessionsByUser(context.Background(), 1); err != nil {
					e.t.Fatalf("RevokeSessionsByUser: %v", err)
				}
				return tok
			},
		},
		{
			name: "user dinonaktifkan",
			setup: func(e *env) string {
				tok := e.login(1)
				e.store.users[1] = fakeUser{username: "budi", active: false}
				return tok
			},
		},
		{
			name: "token acak tidak dikenal",
			setup: func(e *env) string {
				e.login(1)
				return "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ"
			},
		},
		{
			name:  "token kosong",
			setup: func(e *env) string { e.login(1); return "" },
		},
		{
			name: "token lama setelah grace period habis (31 detik)",
			setup: func(e *env) string {
				old := e.login(1)
				e.clock.Advance(16 * time.Minute)
				e.mustValidate(old) // memicu rotasi
				e.clock.Advance(31 * time.Second)
				return old
			},
		},
		{
			name: "token lama tepat di batas grace period (30 detik) sudah tidak berlaku",
			setup: func(e *env) string {
				old := e.login(1)
				e.clock.Advance(16 * time.Minute)
				e.mustValidate(old)
				e.clock.Advance(30 * time.Second)
				return old
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, tc.mutate)
			tok := tc.setup(e)
			_, err := e.validate(tok)
			wantInvalid(t, err)
		})
	}

	t.Run("kegagalan store bukan ErrInvalid", func(t *testing.T) {
		e := newEnv(t, nil)
		tok := e.login(1)
		boom := errors.New("db mati")
		e.store.findErr = boom
		_, err := e.validate(tok)
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
		if errors.Is(err, session.ErrInvalid) {
			t.Fatal("kegagalan infrastruktur tidak boleh dianggap session tidak valid")
		}
	})
}

// ---- Validate: rotasi ----

func TestValidate_Rotation(t *testing.T) {
	t.Run("sebelum interval: tidak dirotasi", func(t *testing.T) {
		e := newEnv(t, nil)
		tok := e.login(1)
		e.clock.Advance(14*time.Minute + 59*time.Second)

		info := e.mustValidate(tok)
		if info.NewToken != "" {
			t.Fatal("belum waktunya rotasi")
		}
		if e.row().TokenHash != session.HashToken(tok) {
			t.Fatal("token tidak boleh berubah")
		}
	})

	t.Run("setelah interval: token baru terbit, absolute expiry tidak berubah", func(t *testing.T) {
		e := newEnv(t, nil)
		old := e.login(1)
		abs := e.row().AbsoluteExpiresAt
		e.clock.Advance(16 * time.Minute)

		info := e.mustValidate(old)

		if info.NewToken == "" || info.NewToken == old {
			t.Fatalf("NewToken = %q, harus terisi dan berbeda dari token lama", info.NewToken)
		}
		row := e.row()
		if row.TokenHash != session.HashToken(info.NewToken) {
			t.Error("TokenHash harus hash dari token baru")
		}
		if row.PreviousTokenHash != session.HashToken(old) {
			t.Error("PreviousTokenHash harus hash dari token lama")
		}
		if !row.PreviousValidUntil.Equal(e.clock.Now().Add(30 * time.Second)) {
			t.Errorf("PreviousValidUntil = %v, want now+30s", row.PreviousValidUntil)
		}
		if !row.RotatedAt.Equal(e.clock.Now()) {
			t.Errorf("RotatedAt = %v, want %v", row.RotatedAt, e.clock.Now())
		}
		if !row.AbsoluteExpiresAt.Equal(abs) {
			t.Errorf("AbsoluteExpiresAt berubah dari %v ke %v; rotasi tidak boleh memperpanjang", abs, row.AbsoluteExpiresAt)
		}
	})

	t.Run("tepat di interval (15 menit) sudah dirotasi", func(t *testing.T) {
		e := newEnv(t, nil)
		tok := e.login(1)
		e.clock.Advance(15 * time.Minute)
		if info := e.mustValidate(tok); info.NewToken == "" {
			t.Fatal("harus dirotasi tepat di batas interval")
		}
	})

	t.Run("token baru bisa dipakai", func(t *testing.T) {
		e := newEnv(t, nil)
		old := e.login(1)
		e.clock.Advance(16 * time.Minute)
		newTok := e.mustValidate(old).NewToken

		info := e.mustValidate(newTok)
		if info.UserID != 1 || info.NewToken != "" {
			t.Fatalf("info tidak sesuai: %+v", info)
		}
	})

	t.Run("token lama dalam grace period tetap valid dan tidak memicu rotasi lagi", func(t *testing.T) {
		e := newEnv(t, nil)
		old := e.login(1)
		e.clock.Advance(16 * time.Minute)
		newTok := e.mustValidate(old).NewToken
		e.clock.Advance(29 * time.Second)

		info := e.mustValidate(old)
		if info.UserID != 1 {
			t.Errorf("UserID = %d", info.UserID)
		}
		if info.NewToken != "" {
			t.Error("token lama tidak boleh memicu rotasi baru")
		}
		if e.row().TokenHash != session.HashToken(newTok) {
			t.Error("token aktif harus tetap token baru")
		}
	})

	t.Run("rotasi berulang tidak memperpanjang absolute expiration", func(t *testing.T) {
		e := newEnv(t, func(c *session.Config) { c.IdleTimeout = 24 * time.Hour })
		tok := e.login(1)
		abs := e.row().AbsoluteExpiresAt

		for i := 0; i < 10; i++ {
			e.clock.Advance(16 * time.Minute)
			info := e.mustValidate(tok)
			if info.NewToken == "" {
				t.Fatalf("iterasi %d: harus dirotasi", i)
			}
			tok = info.NewToken
			if !e.row().AbsoluteExpiresAt.Equal(abs) {
				t.Fatalf("iterasi %d: absolute expiry berubah", i)
			}
		}

		e.clock.Advance(8 * time.Hour)
		_, err := e.validate(tok)
		wantInvalid(t, err)
	})

	t.Run("keduluan request lain: tetap valid tanpa token baru", func(t *testing.T) {
		e := newEnv(t, nil)
		tok := e.login(1)
		e.store.rotateConflict = true
		e.clock.Advance(16 * time.Minute)

		info := e.mustValidate(tok)
		if info.NewToken != "" {
			t.Fatal("rotasi kalah, NewToken harus kosong")
		}
		if !e.row().LastSeenAt.Equal(e.clock.Now()) {
			t.Error("LastSeenAt harus tetap diperbarui")
		}
	})

	t.Run("session idle tidak dirotasi", func(t *testing.T) {
		e := newEnv(t, nil)
		tok := e.login(1)
		e.clock.Advance(31 * time.Minute) // lewat idle sekaligus lewat interval rotasi

		_, err := e.validate(tok)
		wantInvalid(t, err)
		if e.row().TokenHash != session.HashToken(tok) {
			t.Fatal("session yang sudah tidak sah tidak boleh dirotasi")
		}
	})

	t.Run("token lama tidak bisa dipakai setelah session dicabut", func(t *testing.T) {
		e := newEnv(t, nil)
		old := e.login(1)
		e.clock.Advance(16 * time.Minute)
		newTok := e.mustValidate(old).NewToken

		if err := e.m.Revoke(context.Background(), newTok); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		_, err := e.validate(old) // masih dalam grace, tapi session sudah dicabut
		wantInvalid(t, err)
	})
}

// ---- Revoke ----

func TestRevoke(t *testing.T) {
	ctx := context.Background()

	t.Run("logout menonaktifkan session dan mencatat waktunya", func(t *testing.T) {
		e := newEnv(t, nil)
		tok := e.login(1)
		e.clock.Advance(time.Minute)

		if err := e.m.Revoke(ctx, tok); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		if !e.row().RevokedAt.Equal(e.clock.Now()) {
			t.Errorf("RevokedAt = %v, want %v", e.row().RevokedAt, e.clock.Now())
		}
		_, err := e.validate(tok)
		wantInvalid(t, err)
	})

	t.Run("logout idempotent; token tidak dikenal dan kosong bukan error", func(t *testing.T) {
		e := newEnv(t, nil)
		tok := e.login(1)
		for i := 0; i < 2; i++ {
			if err := e.m.Revoke(ctx, tok); err != nil {
				t.Fatalf("Revoke ke-%d: %v", i+1, err)
			}
		}
		if err := e.m.Revoke(ctx, "token-asing"); err != nil {
			t.Fatalf("Revoke token asing: %v", err)
		}
		if err := e.m.Revoke(ctx, ""); err != nil {
			t.Fatalf("Revoke token kosong: %v", err)
		}
	})

	t.Run("RevokeSessionsByUser hanya mencabut milik user itu", func(t *testing.T) {
		e := newEnv(t, nil)
		budi1, budi2 := e.login(1), e.login(1) // dua perangkat
		sari := e.login(2)

		if err := e.m.RevokeSessionsByUser(ctx, 1); err != nil {
			t.Fatalf("RevokeSessionsByUser: %v", err)
		}

		_, err := e.validate(budi1)
		wantInvalid(t, err)
		_, err = e.validate(budi2)
		wantInvalid(t, err)
		if info := e.mustValidate(sari); info.UserID != 2 {
			t.Fatalf("session user lain terkena dampak: %+v", info)
		}
	})
}
