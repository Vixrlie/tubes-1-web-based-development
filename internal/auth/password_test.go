package auth_test

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

func TestHashAndVerifyPassword(t *testing.T) {
	const plain = "rahasia-banget-123"

	hash, err := auth.HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	t.Run("hash bukan plaintext", func(t *testing.T) {
		if hash == plain {
			t.Fatal("hash sama dengan plaintext")
		}
	})

	t.Run("memakai bcrypt cost 12 (sesuai keputusan di README)", func(t *testing.T) {
		cost, err := bcrypt.Cost([]byte(hash))
		if err != nil {
			t.Fatalf("bcrypt.Cost: %v", err)
		}
		if cost != 12 {
			t.Fatalf("cost = %d, want 12", cost)
		}
	})

	t.Run("password benar lolos", func(t *testing.T) {
		if !auth.VerifyPassword(hash, plain) {
			t.Fatal("VerifyPassword menolak password yang benar")
		}
	})

	t.Run("password salah ditolak", func(t *testing.T) {
		if auth.VerifyPassword(hash, "salah-total") {
			t.Fatal("VerifyPassword menerima password yang salah")
		}
	})

	t.Run("hash kosong atau rusak ditolak", func(t *testing.T) {
		if auth.VerifyPassword("", plain) {
			t.Fatal("hash kosong diterima")
		}
		if auth.VerifyPassword("bukan-hash-bcrypt", plain) {
			t.Fatal("hash rusak diterima")
		}
	})

	t.Run("password sama menghasilkan hash berbeda (salt)", func(t *testing.T) {
		other, err := auth.HashPassword(plain)
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		if other == hash {
			t.Fatal("dua hash dari password yang sama identik, salt tidak bekerja")
		}
	})
}
