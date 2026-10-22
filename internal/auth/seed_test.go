package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

func TestSeedAdmin_Success(t *testing.T) {
	ctx := context.Background()

	t.Run("membuat admin bila belum ada", func(t *testing.T) {
		f := newFakeStore()
		if err := auth.SeedAdmin(ctx, f, "Admin", "password-awal"); err != nil {
			t.Fatalf("SeedAdmin: %v", err)
		}
		acc, ok := f.byName["admin"] // dinormalisasi
		if !ok {
			t.Fatal("admin tidak dibuat")
		}
		if !acc.IsPlatformAdmin || !acc.IsActive {
			t.Errorf("admin harus aktif dan Platform Admin: %+v", acc)
		}
		if acc.PasswordHash == "password-awal" || !auth.VerifyPassword(acc.PasswordHash, "password-awal") {
			t.Error("password admin harus tersimpan sebagai hash yang valid")
		}
	})

	t.Run("username sudah ada: tidak membuat dan tidak menimpa", func(t *testing.T) {
		f := newFakeStore()
		f.byName["admin"] = auth.Account{
			User:         auth.User{ID: 7, Username: "admin", IsPlatformAdmin: true, IsActive: true},
			PasswordHash: "hash-lama",
		}
		if err := auth.SeedAdmin(ctx, f, "admin", "password-baru"); err != nil {
			t.Fatalf("SeedAdmin: %v", err)
		}
		if f.createCalls != 0 {
			t.Errorf("Create dipanggil %d kali, want 0", f.createCalls)
		}
		if got := f.byName["admin"].PasswordHash; got != "hash-lama" {
			t.Errorf("password admin lama tertimpa: %q", got)
		}
	})

	t.Run("keduluan proses lain (Create mengembalikan ErrUsernameTaken) dianggap sukses", func(t *testing.T) {
		f := newFakeStore()
		f.createErr = auth.ErrUsernameTaken
		if err := auth.SeedAdmin(ctx, f, "admin", "password-awal"); err != nil {
			t.Fatalf("SeedAdmin: %v, want nil", err)
		}
	})

	t.Run("kedua variabel kosong: seed dilewati", func(t *testing.T) {
		f := newFakeStore()
		if err := auth.SeedAdmin(ctx, f, "", ""); err != nil {
			t.Fatalf("SeedAdmin: %v, want nil", err)
		}
		if f.createCalls != 0 || len(f.byName) != 0 {
			t.Fatal("seed membuat akun padahal konfigurasi kosong")
		}
	})
}

func TestSeedAdmin_Failure(t *testing.T) {
	ctx := context.Background()

	t.Run("hanya username terisi", func(t *testing.T) {
		f := newFakeStore()
		if err := auth.SeedAdmin(ctx, f, "admin", ""); err == nil {
			t.Fatal("want error, got nil")
		}
		if len(f.byName) != 0 {
			t.Fatal("akun terbuat padahal konfigurasi setengah terisi")
		}
	})

	t.Run("hanya password terisi", func(t *testing.T) {
		f := newFakeStore()
		if err := auth.SeedAdmin(ctx, f, "", "password-awal"); err == nil {
			t.Fatal("want error, got nil")
		}
		if len(f.byName) != 0 {
			t.Fatal("akun terbuat padahal konfigurasi setengah terisi")
		}
	})

	t.Run("password terlalu pendek", func(t *testing.T) {
		f := newFakeStore()
		err := auth.SeedAdmin(ctx, f, "admin", "pendek")
		if !errors.Is(err, auth.ErrPasswordTooShort) {
			t.Fatalf("error = %v, want ErrPasswordTooShort", err)
		}
		if len(f.byName) != 0 {
			t.Fatal("akun terbuat padahal password tidak valid")
		}
	})

	t.Run("username tidak valid", func(t *testing.T) {
		f := newFakeStore()
		err := auth.SeedAdmin(ctx, f, "a b", "password-awal")
		if !errors.Is(err, auth.ErrUsernameInvalid) {
			t.Fatalf("error = %v, want ErrUsernameInvalid", err)
		}
	})

	t.Run("error dari store diteruskan, bukan dianggap 'belum ada'", func(t *testing.T) {
		boom := errors.New("db mati")
		f := newFakeStore()
		f.getErr = boom
		err := auth.SeedAdmin(ctx, f, "admin", "password-awal")
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
		if f.createCalls != 0 {
			t.Fatal("Create dipanggil padahal pengecekan user gagal")
		}
	})
}
