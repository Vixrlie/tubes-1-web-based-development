package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserNotFound  = errors.New("user tidak ditemukan")
	ErrUsernameTaken = errors.New("username sudah dipakai")
	ErrEmailTaken    = errors.New("email sudah dipakai")
)

// Account = User + data yang tidak boleh bocor keluar paket auth.
type Account struct {
	User
	FullName     string
	Email        string
	PasswordHash string
}

// NewAccount adalah data untuk INSERT. PasswordHash sudah di-hash oleh pemanggil.
type NewAccount struct {
	Username        string
	FullName        string
	Email           string
	PasswordHash    string
	IsPlatformAdmin bool
}

type Store interface {
	GetByUsername(ctx context.Context, username string) (Account, error)
	GetByID(ctx context.Context, id int64) (Account, error)
	Create(ctx context.Context, n NewAccount) (Account, error)
}

type PGStore struct {
	pool *pgxpool.Pool
}

var _ Store = (*PGStore)(nil)

func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

const accountCols = `id, username, COALESCE(full_name, ''), COALESCE(email, ''),
	password_hash, is_platform_admin, is_active`

func scanAccount(row pgx.Row) (Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Username, &a.FullName, &a.Email,
		&a.PasswordHash, &a.IsPlatformAdmin, &a.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrUserNotFound
	}
	return a, err
}

func (s *PGStore) GetByUsername(ctx context.Context, username string) (Account, error) {
	return scanAccount(s.pool.QueryRow(ctx,
		`SELECT `+accountCols+` FROM users WHERE username = $1`, username))
}

func (s *PGStore) GetByID(ctx context.Context, id int64) (Account, error) {
	return scanAccount(s.pool.QueryRow(ctx,
		`SELECT `+accountCols+` FROM users WHERE id = $1`, id))
}

func (s *PGStore) Create(ctx context.Context, n NewAccount) (Account, error) {
	a, err := scanAccount(s.pool.QueryRow(ctx,
		`INSERT INTO users (username, full_name, email, password_hash, is_platform_admin)
		 VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5)
		 RETURNING `+accountCols,
		n.Username, n.FullName, n.Email, n.PasswordHash, n.IsPlatformAdmin))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			switch pgErr.ConstraintName {
			case "users_username_key":
				return Account{}, ErrUsernameTaken
			case "users_email_key":
				return Account{}, ErrEmailTaken
			}
		}
		return Account{}, err
	}
	return a, nil
}
