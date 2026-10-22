package auth

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrUsernameInvalid  = errors.New("username harus 3-32 karakter: huruf kecil, angka, titik, atau garis bawah")
	ErrPasswordTooShort = errors.New("password minimal 8 karakter")
	ErrPasswordTooLong  = errors.New("password maksimal 72 byte")
)

var usernameRe = regexp.MustCompile(`^[a-z0-9_.]{3,32}$`)

// NormalizeUsername dipanggil sebelum validasi, simpan, dan pencarian.
func NormalizeUsername(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func ValidateUsername(s string) error {
	if !usernameRe.MatchString(s) {
		return ErrUsernameInvalid
	}
	return nil
}

func ValidatePassword(p string) error {
	if utf8.RuneCountInString(p) < 8 {
		return ErrPasswordTooShort
	}
	// bcrypt hanya memakai 72 byte pertama, yang lebih panjang ditolak
	// supaya tidak diam-diam terpotong.
	if len(p) > 72 {
		return ErrPasswordTooLong
	}
	return nil
}
