package auth

import (
	"context"
	"strings"
	"testing"
)

// fakeStore menyimpan akun di memori untuk uji logika tanpa database.
type fakeStore struct {
	accounts map[string]Account
	nextID   int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{accounts: map[string]Account{}}
}

func (f *fakeStore) GetByUsername(ctx context.Context, username string) (Account, error) {
	a, ok := f.accounts[username]
	if !ok {
		return Account{}, ErrUserNotFound
	}
	return a, nil
}

func (f *fakeStore) GetByID(ctx context.Context, id int64) (Account, error) {
	for _, a := range f.accounts {
		if a.ID == id {
			return a, nil
		}
	}
	return Account{}, ErrUserNotFound
}

func (f *fakeStore) Create(ctx context.Context, n NewAccount) (Account, error) {
	if _, exists := f.accounts[n.Username]; exists {
		return Account{}, ErrUsernameTaken
	}
	f.nextID++
	a := Account{
		User: User{
			ID:              f.nextID,
			Username:        n.Username,
			IsPlatformAdmin: n.IsPlatformAdmin,
			IsActive:        true,
		},
		FullName:     n.FullName,
		Email:        n.Email,
		PasswordHash: n.PasswordHash,
	}
	f.accounts[n.Username] = a
	return a, nil
}

func TestNormalizeUsername(t *testing.T) {
	cases := map[string]string{
		"  Admin ":   "admin",
		"RINA":       "rina",
		"budi.santo": "budi.santo",
		"a_b_c":      "a_b_c",
	}
	for in, want := range cases {
		if got := NormalizeUsername(in); got != want {
			t.Errorf("NormalizeUsername(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateUsername(t *testing.T) {
	valid := []string{"abc", "rina.admin", "budi_santo", strings.Repeat("x", 32)}
	for _, u := range valid {
		if err := ValidateUsername(u); err != nil {
			t.Errorf("ValidateUsername(%q) = %v, want nil", u, err)
		}
	}
	invalid := []string{"", "ab", strings.Repeat("x", 33), "UPPER", "sp ace", "karakter-asing", "emoji😀"}
	for _, u := range invalid {
		if err := ValidateUsername(u); err == nil {
			t.Errorf("ValidateUsername(%q) = nil, want error", u)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("rahasia12"); err != nil {
		t.Errorf("password valid ditolak: %v", err)
	}
	if err := ValidatePassword("pendek"); err != ErrPasswordTooShort {
		t.Errorf("password 6 karakter: err = %v, want ErrPasswordTooShort", err)
	}
	if err := ValidatePassword(strings.Repeat("x", 73)); err != ErrPasswordTooLong {
		t.Errorf("password 73 byte: err = %v, want ErrPasswordTooLong", err)
	}
	if err := ValidatePassword(strings.Repeat("x", 72)); err != nil {
		t.Errorf("password 72 byte ditolak: %v", err)
	}
	// bcrypt hanya memakai 72 byte pertama; unicode 24 rune = 72 byte.
	unicode72 := strings.Repeat("é", 36) // 2 byte per rune
	if err := ValidatePassword(unicode72); err == nil {
		t.Log("unicode 36 rune (72 byte) diterima — batas byte sesuai desain")
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("rahasia12")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$2a$12$") {
		t.Errorf("prefix hash = %q, want $2a$12$ (bcrypt cost 12)", hash[:7])
	}
	if !VerifyPassword(hash, "rahasia12") {
		t.Error("password asli gagal diverifikasi")
	}
	if VerifyPassword(hash, "salah") {
		t.Error("password salah diterima")
	}
	// Hash berbeda tiap panggilan (salt otomatis).
	hash2, _ := HashPassword("rahasia12")
	if hash == hash2 {
		t.Error("hash identik padahal salt harus acak")
	}
}

func TestCreateUserNormalizesAndHashes(t *testing.T) {
	store := newFakeStore()
	acc, err := CreateUser(context.Background(), store, NewUser{
		Username: "  RINA ",
		Password: "rahasia12",
		FullName: "Rina",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if acc.Username != "rina" {
		t.Errorf("username = %q, want rina (ternormalisasi)", acc.Username)
	}
	if acc.PasswordHash == "rahasia12" {
		t.Fatal("password tersimpan plaintext!")
	}
	if !VerifyPassword(acc.PasswordHash, "rahasia12") {
		t.Error("hash tidak cocok dengan password asli")
	}
}

func TestCreateUserRejectsInvalid(t *testing.T) {
	store := newFakeStore()
	if _, err := CreateUser(context.Background(), store, NewUser{Username: "x", Password: "rahasia12"}); err == nil {
		t.Error("username pendek diterima")
	}
	if _, err := CreateUser(context.Background(), store, NewUser{Username: "valid", Password: "pendek"}); err == nil {
		t.Error("password pendek diterima")
	}
	if len(store.accounts) != 0 {
		t.Error("akun invalid tersimpan di store")
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	store := newFakeStore()
	in := NewUser{Username: "rina", Password: "rahasia12"}
	if _, err := CreateUser(context.Background(), store, in); err != nil {
		t.Fatalf("CreateUser pertama: %v", err)
	}
	if _, err := CreateUser(context.Background(), store, in); err != ErrUsernameTaken {
		t.Errorf("duplikat: err = %v, want ErrUsernameTaken", err)
	}
}

func TestSeedAdmin(t *testing.T) {
	t.Run("seed idempoten", func(t *testing.T) {
		store := newFakeStore()
		if err := SeedAdmin(context.Background(), store, "admin", "rahasia12"); err != nil {
			t.Fatalf("SeedAdmin: %v", err)
		}
		acc, _ := store.GetByUsername(context.Background(), "admin")
		if !acc.IsPlatformAdmin {
			t.Error("admin seed bukan platform admin")
		}
		// Ganti password lalu seed ulang: tidak boleh menimpa.
		acc.PasswordHash = "sudah-diganti"
		store.accounts["admin"] = acc
		if err := SeedAdmin(context.Background(), store, "admin", "password-baru-lain"); err != nil {
			t.Fatalf("SeedAdmin ulang: %v", err)
		}
		after, _ := store.GetByUsername(context.Background(), "admin")
		if after.PasswordHash != "sudah-diganti" {
			t.Error("seed ulang menimpa akun yang sudah ada")
		}
	})

	t.Run("kosong keduanya = lewati", func(t *testing.T) {
		store := newFakeStore()
		if err := SeedAdmin(context.Background(), store, "", ""); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if len(store.accounts) != 0 {
			t.Error("akun dibuat meski kredensial kosong")
		}
	})

	t.Run("hanya satu terisi = error", func(t *testing.T) {
		store := newFakeStore()
		if err := SeedAdmin(context.Background(), store, "admin", ""); err == nil {
			t.Error("username tanpa password diterima")
		}
		if err := SeedAdmin(context.Background(), store, "", "rahasia12"); err == nil {
			t.Error("password tanpa username diterima")
		}
	})
}
