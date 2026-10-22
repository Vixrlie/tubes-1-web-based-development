package auth

import (
	"context"
	"errors"
	"log"
)

// SeedAdmin membuat Platform Admin pertama dari ADMIN_USERNAME dan ADMIN_PASSWORD.
// Aman dipanggil di setiap startup: kalau username sudah ada, tidak melakukan apa pun.
func SeedAdmin(ctx context.Context, s Store, username, password string) error {
	if username == "" && password == "" {
		log.Println("seed admin dilewati: ADMIN_USERNAME dan ADMIN_PASSWORD kosong")
		return nil
	}
	if username == "" || password == "" {
		return errors.New("ADMIN_USERNAME dan ADMIN_PASSWORD harus diisi bersamaan")
	}

	username = NormalizeUsername(username)

	_, err := s.GetByUsername(ctx, username)
	if err == nil {
		return nil // sudah ada
	}
	if !errors.Is(err, ErrUserNotFound) {
		return err
	}

	_, err = CreateUser(ctx, s, NewUser{
		Username:        username,
		Password:        password,
		IsPlatformAdmin: true,
	})
	if errors.Is(err, ErrUsernameTaken) {
		return nil // keduluan proses lain, tidak masalah
	}
	if err != nil {
		return err
	}

	log.Printf("admin awal dibuat: %s", username) // jangan log password
	return nil
}
