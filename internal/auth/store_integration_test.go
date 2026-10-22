package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

func setupStore(t *testing.T) (*auth.PGStore, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.NewPool(t) // melewati test bila -short
	return auth.NewPGStore(pool), pool
}

func countUsers(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	return n
}

func TestPGStore_Create_Success(t *testing.T) {
	ctx := context.Background()
	store, _ := setupStore(t)

	acc, err := store.Create(ctx, auth.NewAccount{
		Username:     "budi",
		FullName:     "Budi Santoso",
		Email:        "budi@example.com",
		PasswordHash: "hash-dummy", // store tidak meng-hash; itu tugas CreateUser
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if acc.ID == 0 {
		t.Error("ID tidak terisi dari RETURNING")
	}
	if acc.Username != "budi" || acc.FullName != "Budi Santoso" || acc.Email != "budi@example.com" {
		t.Errorf("data tidak sesuai: %+v", acc)
	}
	if acc.PasswordHash != "hash-dummy" {
		t.Errorf("PasswordHash = %q", acc.PasswordHash)
	}
	if !acc.IsActive {
		t.Error("user baru harus aktif (default skema)")
	}
	if acc.IsPlatformAdmin {
		t.Error("user tanpa IsPlatformAdmin tidak boleh menjadi admin")
	}

	t.Run("GetByUsername mengembalikan data yang sama", func(t *testing.T) {
		got, err := store.GetByUsername(ctx, "budi")
		if err != nil {
			t.Fatalf("GetByUsername: %v", err)
		}
		if got != acc {
			t.Fatalf("got %+v, want %+v", got, acc)
		}
	})

	t.Run("GetByID mengembalikan data yang sama", func(t *testing.T) {
		got, err := store.GetByID(ctx, acc.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got != acc {
			t.Fatalf("got %+v, want %+v", got, acc)
		}
	})
}

func TestPGStore_Create_Admin(t *testing.T) {
	ctx := context.Background()
	store, _ := setupStore(t)

	acc, err := store.Create(ctx, auth.NewAccount{
		Username: "root", PasswordHash: "h", IsPlatformAdmin: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !acc.IsPlatformAdmin {
		t.Fatal("IsPlatformAdmin tidak tersimpan")
	}
}

func TestPGStore_Create_OptionalFields(t *testing.T) {
	ctx := context.Background()
	store, _ := setupStore(t)

	// Email kosong disimpan sebagai NULL, jadi dua user tanpa email tidak bentrok di UNIQUE.
	a, err := store.Create(ctx, auth.NewAccount{Username: "satu", PasswordHash: "h"})
	if err != nil {
		t.Fatalf("Create satu: %v", err)
	}
	if _, err := store.Create(ctx, auth.NewAccount{Username: "dua", PasswordHash: "h"}); err != nil {
		t.Fatalf("Create dua (sama-sama tanpa email): %v", err)
	}
	if a.Email != "" || a.FullName != "" {
		t.Errorf("Email/FullName kosong harus kembali sebagai string kosong: %+v", a)
	}
}

func TestPGStore_Create_Duplicate(t *testing.T) {
	ctx := context.Background()
	store, pool := setupStore(t)

	if _, err := store.Create(ctx, auth.NewAccount{
		Username: "budi", Email: "budi@example.com", PasswordHash: "h",
	}); err != nil {
		t.Fatalf("Create awal: %v", err)
	}

	t.Run("username kembar", func(t *testing.T) {
		_, err := store.Create(ctx, auth.NewAccount{Username: "budi", PasswordHash: "h"})
		if !errors.Is(err, auth.ErrUsernameTaken) {
			t.Fatalf("error = %v, want ErrUsernameTaken", err)
		}
	})

	t.Run("email kembar", func(t *testing.T) {
		_, err := store.Create(ctx, auth.NewAccount{
			Username: "budi2", Email: "budi@example.com", PasswordHash: "h",
		})
		if !errors.Is(err, auth.ErrEmailTaken) {
			t.Fatalf("error = %v, want ErrEmailTaken", err)
		}
	})

	if n := countUsers(t, pool); n != 1 {
		t.Fatalf("jumlah user = %d, want 1 (percobaan duplikat tidak boleh menambah baris)", n)
	}
}

func TestPGStore_NotFound(t *testing.T) {
	ctx := context.Background()
	store, _ := setupStore(t)

	if _, err := store.Create(ctx, auth.NewAccount{Username: "budi", PasswordHash: "h"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("GetByUsername tidak ada", func(t *testing.T) {
		_, err := store.GetByUsername(ctx, "tidak-ada")
		if !errors.Is(err, auth.ErrUserNotFound) {
			t.Fatalf("error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("GetByID tidak ada", func(t *testing.T) {
		_, err := store.GetByID(ctx, 999999)
		if !errors.Is(err, auth.ErrUserNotFound) {
			t.Fatalf("error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("store tidak menormalisasi: pencarian harus persis", func(t *testing.T) {
		// Normalisasi adalah tugas CreateUser/handler, bukan store.
		_, err := store.GetByUsername(ctx, "BUDI")
		if !errors.Is(err, auth.ErrUserNotFound) {
			t.Fatalf("error = %v, want ErrUserNotFound", err)
		}
	})
}

func TestCreateUser_WithPGStore(t *testing.T) {
	ctx := context.Background()
	store, _ := setupStore(t)

	acc, err := auth.CreateUser(ctx, store, auth.NewUser{
		Username: "Budi", Password: "rahasia123",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if acc.IsPlatformAdmin {
		t.Error("user biasa tidak boleh menjadi admin")
	}

	got, err := store.GetByUsername(ctx, "budi")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if got.PasswordHash == "rahasia123" || !auth.VerifyPassword(got.PasswordHash, "rahasia123") {
		t.Error("password harus tersimpan sebagai hash yang valid")
	}

	// "BUDI" dinormalisasi menjadi "budi" sehingga harus bentrok.
	_, err = auth.CreateUser(ctx, store, auth.NewUser{Username: "BUDI", Password: "rahasia123"})
	if !errors.Is(err, auth.ErrUsernameTaken) {
		t.Fatalf("error = %v, want ErrUsernameTaken", err)
	}
}

func TestSeedAdmin_WithPGStore(t *testing.T) {
	ctx := context.Background()

	t.Run("start pertama membuat Platform Admin", func(t *testing.T) {
		store, pool := setupStore(t)
		if err := auth.SeedAdmin(ctx, store, "Admin", "password-awal"); err != nil {
			t.Fatalf("SeedAdmin: %v", err)
		}
		acc, err := store.GetByUsername(ctx, "admin")
		if err != nil {
			t.Fatalf("GetByUsername: %v", err)
		}
		if !acc.IsPlatformAdmin || !acc.IsActive {
			t.Errorf("admin harus aktif dan Platform Admin: %+v", acc)
		}
		if acc.PasswordHash == "password-awal" || !auth.VerifyPassword(acc.PasswordHash, "password-awal") {
			t.Error("password harus tersimpan sebagai hash yang valid")
		}
		if n := countUsers(t, pool); n != 1 {
			t.Errorf("jumlah user = %d, want 1", n)
		}
	})

	t.Run("start berikutnya idempotent dan tidak menimpa password", func(t *testing.T) {
		store, pool := setupStore(t)
		if err := auth.SeedAdmin(ctx, store, "admin", "password-awal"); err != nil {
			t.Fatalf("SeedAdmin pertama: %v", err)
		}
		// .env diubah, lalu aplikasi di-restart.
		if err := auth.SeedAdmin(ctx, store, "admin", "password-diganti"); err != nil {
			t.Fatalf("SeedAdmin kedua: %v", err)
		}
		// Variasi huruf besar tetap dianggap admin yang sama.
		if err := auth.SeedAdmin(ctx, store, "ADMIN", "password-diganti"); err != nil {
			t.Fatalf("SeedAdmin ketiga: %v", err)
		}

		if n := countUsers(t, pool); n != 1 {
			t.Fatalf("jumlah user = %d, want 1", n)
		}
		acc, err := store.GetByUsername(ctx, "admin")
		if err != nil {
			t.Fatalf("GetByUsername: %v", err)
		}
		if !auth.VerifyPassword(acc.PasswordHash, "password-awal") {
			t.Error("password awal harus tetap berlaku")
		}
		if auth.VerifyPassword(acc.PasswordHash, "password-diganti") {
			t.Error("password dari seed berikutnya tidak boleh menimpa")
		}
	})

	t.Run("konfigurasi tidak valid tidak membuat akun", func(t *testing.T) {
		store, pool := setupStore(t)

		if err := auth.SeedAdmin(ctx, store, "admin", "pendek"); err == nil {
			t.Error("password pendek harus ditolak")
		}
		if err := auth.SeedAdmin(ctx, store, "admin", ""); err == nil {
			t.Error("konfigurasi setengah terisi harus ditolak")
		}
		if n := countUsers(t, pool); n != 0 {
			t.Fatalf("jumlah user = %d, want 0", n)
		}
	})

	t.Run("kedua variabel kosong: seed dilewati", func(t *testing.T) {
		store, pool := setupStore(t)
		if err := auth.SeedAdmin(ctx, store, "", ""); err != nil {
			t.Fatalf("SeedAdmin: %v", err)
		}
		if n := countUsers(t, pool); n != 0 {
			t.Fatalf("jumlah user = %d, want 0", n)
		}
	})
}
