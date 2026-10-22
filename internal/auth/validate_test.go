package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

func TestNormalizeUsername(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"huruf besar jadi kecil", "Admin", "admin"},
		{"spasi di tepi dibuang", "  budi  ", "budi"},
		{"sudah normal tidak berubah", "budi.s_1", "budi.s_1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := auth.NormalizeUsername(tc.in); got != tc.want {
				t.Errorf("NormalizeUsername(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr error
	}{
		{"valid sederhana", "admin", nil},
		{"valid dengan titik, garis bawah, angka", "a.b_c1", nil},
		{"batas bawah 3 karakter", "abc", nil},
		{"batas atas 32 karakter", strings.Repeat("a", 32), nil},

		{"terlalu pendek", "ab", auth.ErrUsernameInvalid},
		{"terlalu panjang", strings.Repeat("a", 33), auth.ErrUsernameInvalid},
		{"kosong", "", auth.ErrUsernameInvalid},
		{"huruf besar (harus dinormalisasi dulu)", "Admin", auth.ErrUsernameInvalid},
		{"mengandung spasi", "bud i", auth.ErrUsernameInvalid},
		{"mengandung tanda hubung", "bud-i", auth.ErrUsernameInvalid},
		{"mengandung @", "bud@i", auth.ErrUsernameInvalid},
		{"non-ASCII", "budé", auth.ErrUsernameInvalid},
		{"newline di akhir", "budi\n", auth.ErrUsernameInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := auth.ValidateUsername(tc.in)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateUsername(%q) error = %v, want nil", tc.in, err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ValidateUsername(%q) error = %v, want %v", tc.in, err, tc.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr error
	}{
		{"valid 8 karakter", "12345678", nil},
		{"valid tepat 72 byte", strings.Repeat("a", 72), nil},
		{"valid 8 karakter multibyte", strings.Repeat("é", 8), nil}, // 16 byte

		{"7 karakter terlalu pendek", "1234567", auth.ErrPasswordTooShort},
		{"kosong", "", auth.ErrPasswordTooShort},
		{"73 byte terlalu panjang", strings.Repeat("a", 73), auth.ErrPasswordTooLong},
		{"37 karakter multibyte = 74 byte", strings.Repeat("é", 37), auth.ErrPasswordTooLong},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := auth.ValidatePassword(tc.in)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
