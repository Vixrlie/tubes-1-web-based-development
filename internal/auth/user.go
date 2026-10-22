package auth

import "context"

type NewUser struct {
	Username        string
	FullName        string
	Email           string
	Password        string // plaintext, hanya hidup di fungsi ini
	IsPlatformAdmin bool
}

// CreateUser adalah satu-satunya pintu membuat akun: normalisasi, validasi, hash, simpan.
// Dipakai seed admin sekarang, dan halaman admin kelola user di langkah 5.
func CreateUser(ctx context.Context, s Store, in NewUser) (Account, error) {
	username := NormalizeUsername(in.Username)
	if err := ValidateUsername(username); err != nil {
		return Account{}, err
	}
	if err := ValidatePassword(in.Password); err != nil {
		return Account{}, err
	}

	hash, err := HashPassword(in.Password)
	if err != nil {
		return Account{}, err
	}

	return s.Create(ctx, NewAccount{
		Username:        username,
		FullName:        in.FullName,
		Email:           in.Email,
		PasswordHash:    hash,
		IsPlatformAdmin: in.IsPlatformAdmin,
	})
}
