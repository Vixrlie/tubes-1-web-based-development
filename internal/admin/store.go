package admin

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

// UserRow adalah data user untuk halaman admin. Sengaja tanpa password hash.
type UserRow struct {
	ID              int64
	Username        string
	FullName        string
	Email           string
	IsPlatformAdmin bool
	IsActive        bool
	CreatedAt       time.Time
}

type UserUpdate struct {
	FullName        string
	Email           string
	IsPlatformAdmin bool
}

// UserStore menanam auth.Store supaya auth.CreateUser (satu-satunya pintu
// membuat akun: normalisasi, validasi, hash) bisa dipakai langsung.
type UserStore interface {
	auth.Store
	List(ctx context.Context) ([]UserRow, error)
	Get(ctx context.Context, id int64) (UserRow, error)
	Update(ctx context.Context, id int64, u UserUpdate) error
	SetActive(ctx context.Context, id int64, active bool) error
	SetPassword(ctx context.Context, id int64, passwordHash string) error
}

type PGUserStore struct {
	*auth.PGStore
	pool *pgxpool.Pool
}

var _ UserStore = (*PGUserStore)(nil)

func NewPGUserStore(pool *pgxpool.Pool) *PGUserStore {
	return &PGUserStore{PGStore: auth.NewPGStore(pool), pool: pool}
}

const userCols = `id, username, COALESCE(full_name, ''), COALESCE(email, ''),
	is_platform_admin, is_active, created_at`

func scanUser(row pgx.Row) (UserRow, error) {
	var u UserRow
	err := row.Scan(&u.ID, &u.Username, &u.FullName, &u.Email,
		&u.IsPlatformAdmin, &u.IsActive, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRow{}, auth.ErrUserNotFound
	}
	return u, err
}

func (s *PGUserStore) List(ctx context.Context) ([]UserRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserRow
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *PGUserStore) Get(ctx context.Context, id int64) (UserRow, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

func (s *PGUserStore) Update(ctx context.Context, id int64, u UserUpdate) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE users
		    SET full_name = NULLIF($2, ''), email = NULLIF($3, ''),
		        is_platform_admin = $4, updated_at = now()
		  WHERE id = $1`,
		id, u.FullName, u.Email, u.IsPlatformAdmin)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
			return auth.ErrEmailTaken
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return auth.ErrUserNotFound
	}
	return nil
}

func (s *PGUserStore) SetActive(ctx context.Context, id int64, active bool) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE users SET is_active = $2, updated_at = now() WHERE id = $1`, id, active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return auth.ErrUserNotFound
	}
	return nil
}

func (s *PGUserStore) SetPassword(ctx context.Context, id int64, passwordHash string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, id, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return auth.ErrUserNotFound
	}
	return nil
}
