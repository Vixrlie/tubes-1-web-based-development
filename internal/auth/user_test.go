package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

// fakeStore adalah Store di memori untuk unit test.
type fakeStore struct {
	byName      map[string]auth.Account
	nextID      int64
	createCalls int
	getErr      error // kalau diisi, GetByUsername/GetByID mengembalikannya
	createErr   error // kalau diisi, Create mengembalikannya
}

var _ auth.Store = (*fakeStore)(nil)

func newFakeStore() *fakeStore {
	return &fakeStore{byName: map[string]auth.Account{}, nextID: 1}
}

func (f *fakeStore) GetByUsername(_ context.Context, username string) (auth.Account, error) {
	if f.getErr != nil {
		return auth.Account{}, f.getErr
	}
	a, ok := f.byName[username]
	if !ok {
		return auth.Account{}, auth.ErrUserNotFound
	}
	return a, nil
}

func (f *fakeStore) GetByID(_ context.Context, id int64) (auth.Account, error) {
	if f.getErr != nil {
		return auth.Account{}, f.getErr
	}
	for _, a := range f.byName {
		if a.ID == id {
			return a, nil
		}
	}
	return auth.Account{}, auth.ErrUserNotFound
}

func (f *fakeStore) Create(_ context.Context, n auth.NewAccount) (auth.Account, error) {
	f.createCalls++
	if f.createErr != nil {
		return auth.Account{}, f.createErr
	}
	if _, ok := f.byName[n.Username]; ok {
		return auth.Account{}, auth.ErrUsernameTaken
	}
	a := auth.Account{
		User: auth.User{
			ID:              f.nextID,
			Username:        n.Username,
			IsPlatformAdmin: n.IsPlatformAdmin,
			IsActive:        true,
		},
		FullName:     n.FullName,
		Email:        n.Email,
		PasswordHash: n.PasswordHash,
	}
	f.nextID++
	f.byName[n.Username] = a
	return a, nil
}

func TestCreateUser_Success(t *testing.T) {
	ctx := context.Background()

	t.Run("username dinormalisasi dan password di-hash", func(t *testing.T) {
		f := newFakeStore()
		acc, err := auth.CreateUser(ctx, f, auth.NewUser{
			Username: "  Budi.S ",
			FullName: "Budi Santoso",
			Email:    "budi@example.com",
			Password: "rahasia123",
		})
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if acc.Username != "budi.s" {
			t.Errorf("Username = %q, want %q", acc.Username, "budi.s")
		}
		if acc.PasswordHash == "" || acc.PasswordHash == "rahasia123" {
			t.Errorf("PasswordHash = %q, harus hash bukan plaintext", acc.PasswordHash)
		}
		if !auth.VerifyPassword(acc.PasswordHash, "rahasia123") {
			t.Error("hash yang tersimpan tidak cocok dengan password asli")
		}
		if acc.FullName != "Budi Santoso" || acc.Email != "budi@example.com" {
			t.Errorf("FullName/Email tidak diteruskan: %+v", acc)
		}
		if !acc.IsActive {
			t.Error("user baru harus aktif")
		}
	})

	t.Run("user biasa tidak otomatis admin", func(t *testing.T) {
		f := newFakeStore()
		acc, err := auth.CreateUser(ctx, f, auth.NewUser{Username: "budi", Password: "rahasia123"})
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if acc.IsPlatformAdmin {
			t.Fatal("user tanpa IsPlatformAdmin menjadi admin")
		}
	})

	t.Run("IsPlatformAdmin diteruskan bila diminta", func(t *testing.T) {
		f := newFakeStore()
		acc, err := auth.CreateUser(ctx, f, auth.NewUser{
			Username: "root", Password: "rahasia123", IsPlatformAdmin: true,
		})
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if !acc.IsPlatformAdmin {
			t.Fatal("IsPlatformAdmin tidak diteruskan")
		}
	})
}

func TestCreateUser_Failure(t *testing.T) {
	ctx := context.Background()

	t.Run("input tidak valid ditolak sebelum menyentuh store", func(t *testing.T) {
		tests := []struct {
			name    string
			in      auth.NewUser
			wantErr error
		}{
			{"username terlalu pendek", auth.NewUser{Username: "ab", Password: "rahasia123"}, auth.ErrUsernameInvalid},
			{"username berisi spasi", auth.NewUser{Username: "bud i", Password: "rahasia123"}, auth.ErrUsernameInvalid},
			{"password terlalu pendek", auth.NewUser{Username: "budi", Password: "1234567"}, auth.ErrPasswordTooShort},
			{"password lebih dari 72 byte", auth.NewUser{Username: "budi", Password: string(make([]byte, 73))}, auth.ErrPasswordTooLong},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				f := newFakeStore()
				_, err := auth.CreateUser(ctx, f, tc.in)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				if f.createCalls != 0 || len(f.byName) != 0 {
					t.Fatalf("store tersentuh padahal input tidak valid (createCalls=%d)", f.createCalls)
				}
			})
		}
	})

	t.Run("username sudah dipakai", func(t *testing.T) {
		f := newFakeStore()
		in := auth.NewUser{Username: "budi", Password: "rahasia123"}
		if _, err := auth.CreateUser(ctx, f, in); err != nil {
			t.Fatalf("CreateUser pertama: %v", err)
		}
		// "BUDI" dinormalisasi menjadi "budi", jadi harus bentrok.
		_, err := auth.CreateUser(ctx, f, auth.NewUser{Username: "BUDI", Password: "rahasia123"})
		if !errors.Is(err, auth.ErrUsernameTaken) {
			t.Fatalf("error = %v, want ErrUsernameTaken", err)
		}
	})

	t.Run("error dari store diteruskan", func(t *testing.T) {
		boom := errors.New("db mati")
		f := newFakeStore()
		f.createErr = boom
		_, err := auth.CreateUser(ctx, f, auth.NewUser{Username: "budi", Password: "rahasia123"})
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})
}
