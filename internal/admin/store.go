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

var (
	// ErrLastAdmin tolak penghapusan active admin terakhir
	ErrLastAdmin = errors.New("admin aktif terakhir tidak dapat dihapus")
	// ErrUserHasHistory menolak penghapusan user yang tercatat sebagai
	// pelaku migration, cukup dinonaktifkan.
	ErrUserHasHistory = errors.New("user memiliki riwayat migration dan hanya dapat dinonaktifkan")
)

// UserStore menanam auth.Store supaya auth.CreateUser bisa dipakai langsung.
type UserStore interface {
	auth.Store
	List(ctx context.Context) ([]UserRow, error)
	Get(ctx context.Context, id int64) (UserRow, error)
	Update(ctx context.Context, id int64, u UserUpdate) error
	SetActive(ctx context.Context, id int64, active bool) error
	SetPassword(ctx context.Context, id int64, passwordHash string) error
	// GroupNamesByUser mengembalikan nama-nama group per user (untuk daftar user).
	GroupNamesByUser(ctx context.Context) (map[int64][]string, error)
	// Delete menghapus user beserta keanggotaan dan session (cascade).
	// Ditolak dengan ErrLastAdmin atau ErrUserHasHistory sesuai guard di atas.
	Delete(ctx context.Context, id int64) error
	// EffectiveAccess mengembalikan grant setiap group yang dianggotei user
	// pada setiap database (level kosong = group itu tidak punya grant di DB tsb).
	EffectiveAccess(ctx context.Context, userID int64) ([]EffectiveGrant, error)
}

// EffectiveGrant adalah satu baris grant group→database untuk satu user.
type EffectiveGrant struct {
	ConnectionID   int64
	ConnectionName string
	GroupID        int64
	GroupName      string
	Level          string // "view", "edit", atau "" (tanpa grant)
}

type PGUserStore struct {
	*auth.PGStore
	pool *pgxpool.Pool
}

func NewPGUserStore(pool *pgxpool.Pool) *PGUserStore {
	return &PGUserStore{PGStore: auth.NewPGStore(pool), pool: pool}
}

var _ UserStore = (*PGUserStore)(nil)

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

// GroupNamesByUser mengembalikan nama group per user, urut nama group,
// untuk kolom "Group" di daftar user.
func (s *PGUserStore) GroupNamesByUser(ctx context.Context) (map[int64][]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.user_id, g.name
		   FROM memberships m
		   JOIN groups g ON g.id = m.group_id
		  ORDER BY g.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64][]string{}
	for rows.Next() {
		var userID int64
		var name string
		if err := rows.Scan(&userID, &name); err != nil {
			return nil, err
		}
		out[userID] = append(out[userID], name)
	}
	return out, rows.Err()
}

// Delete menghapus user; memberships dan sessions ikut terhapus (cascade).
// Guard: admin aktif terakhir dan user dengan riwayat migration tidak boleh dihapus.
func (s *PGUserStore) Delete(ctx context.Context, id int64) error {
	var isAdmin, isActive bool
	err := s.pool.QueryRow(ctx,
		`SELECT is_platform_admin, is_active FROM users WHERE id = $1`, id).
		Scan(&isAdmin, &isActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrUserNotFound
	}
	if err != nil {
		return err
	}

	if isAdmin && isActive {
		var remaining int64
		if err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM users WHERE is_platform_admin AND is_active AND id <> $1`, id).
			Scan(&remaining); err != nil {
			return err
		}
		if remaining == 0 {
			return ErrLastAdmin
		}
	}

	var history int64
	if err := s.pool.QueryRow(ctx,
		`SELECT (SELECT COUNT(*) FROM migration_history WHERE created_by = $1)
		      + (SELECT COUNT(*) FROM migration_history WHERE executed_by = $1)`, id).
		Scan(&history); err != nil {
		return err
	}
	if history > 0 {
		return ErrUserHasHistory
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return auth.ErrUserNotFound
	}
	return nil
}

// EffectiveAccess mengembalikan semua kombinasi (database × group yang
// dianggotei user) beserta level grant-nya — group tanpa grant di database
// itu ikut tampil dengan level kosong, persis seperti matriks akses admin.
func (s *PGUserStore) EffectiveAccess(ctx context.Context, userID int64) ([]EffectiveGrant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT dc.id, dc.name, gr.id, gr.name, COALESCE(g.level, '')
		   FROM memberships m
		   JOIN groups gr    ON gr.id = m.group_id
		  CROSS JOIN db_connections dc
		   LEFT JOIN grants g ON g.group_id = gr.id AND g.connection_id = dc.id
		  WHERE m.user_id = $1
		  ORDER BY dc.name, gr.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EffectiveGrant
	for rows.Next() {
		var e EffectiveGrant
		if err := rows.Scan(&e.ConnectionID, &e.ConnectionName, &e.GroupID, &e.GroupName, &e.Level); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
