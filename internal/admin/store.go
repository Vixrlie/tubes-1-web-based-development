package admin

import (
	"context"
	"database/sql"
	"errors"
	"strings"
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

type GrantRow struct {
	ConnectionName string
	Level          string
}

type GroupRow struct {
	ID            int64
	Name          string
	MemberCount   int
	MemberNames   []string
	Access        []GrantRow
	AccessSummary string
}

type GroupDetail struct {
	ID      int64
	Name    string
	Members []UserRow
	Grants  []GrantRow
}

type GroupStore interface {
	ListGroups(ctx context.Context) ([]GroupRow, error)
	ListGroupDetail(ctx context.Context, groupID int64) (*GroupDetail, error)
	CreateGroup(ctx context.Context, name string) (int64, error)
	UpdateGroupName(ctx context.Context, groupID int64, name string) error
	DeleteGroup(ctx context.Context, groupID int64) error
	RemoveMember(ctx context.Context, groupID int64, userID int64) error
}

var ErrGroupTaken = errors.New("nama group sudah digunakan")

func labelGrantLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "edit":
		return "View + Edit"
	default:
		return "View Only"
	}
}

func (s *PGUserStore) ListGroups(ctx context.Context) ([]GroupRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.name,
		       COUNT(m.user_id) FILTER (WHERE m.user_id IS NOT NULL) AS member_count,
		       COALESCE(string_agg(DISTINCT u.username ORDER BY u.username), '') FILTER (WHERE COUNT(m.user_id) > 0) AS member_names,
		       COALESCE(string_agg(DISTINCT dc.name || ':' || gr.level ORDER BY dc.name), '') FILTER (WHERE COUNT(dc.id) > 0) AS access_raw
		FROM groups g
		LEFT JOIN memberships m ON m.group_id = g.id
		LEFT JOIN users u ON u.id = m.user_id
		LEFT JOIN grants gr ON gr.group_id = g.id
		LEFT JOIN db_connections dc ON dc.id = gr.connection_id
		GROUP BY g.id, g.name
		ORDER BY g.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupRow
	for rows.Next() {
		var g GroupRow
		var memberNames, accessRaw sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &g.MemberCount, &memberNames, &accessRaw); err != nil {
			return nil, err
		}
		if memberNames.Valid {
			g.MemberNames = strings.Split(memberNames.String, ",")
		} else {
			g.MemberNames = []string{}
		}
		if accessRaw.Valid {
			for _, part := range strings.Split(accessRaw.String, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				kv := strings.SplitN(part, ":", 2)
				name := strings.TrimSpace(kv[0])
				level := ""
				if len(kv) == 2 {
					level = strings.TrimSpace(kv[1])
				}
				g.Access = append(g.Access, GrantRow{ConnectionName: name, Level: labelGrantLevel(level)})
			}
		}
		if len(g.Access) == 0 {
			g.AccessSummary = "Tidak ada akses"
		} else {
			parts := make([]string, 0, len(g.Access))
			for _, a := range g.Access {
				parts = append(parts, a.ConnectionName+": "+a.Level)
			}
			g.AccessSummary = strings.Join(parts, ", ")
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *PGUserStore) ListGroupDetail(ctx context.Context, groupID int64) (*GroupDetail, error) {
	var name string
	if err := s.pool.QueryRow(ctx, `SELECT name FROM groups WHERE id = $1`, groupID).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	members, err := s.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	grants, err := s.ListGrants(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return &GroupDetail{ID: groupID, Name: name, Members: members, Grants: grants}, nil
}

func (s *PGUserStore) ListMembers(ctx context.Context, groupID int64) ([]UserRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+userCols+` FROM users u
		JOIN memberships m ON m.user_id = u.id
		WHERE m.group_id = $1
		ORDER BY u.username
	`, groupID)
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

func (s *PGUserStore) ListGrants(ctx context.Context, groupID int64) ([]GrantRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT dc.name, gr.level
		FROM grants gr
		JOIN db_connections dc ON dc.id = gr.connection_id
		WHERE gr.group_id = $1
		ORDER BY dc.name
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GrantRow
	for rows.Next() {
		var g GrantRow
		if err := rows.Scan(&g.ConnectionName, &g.Level); err != nil {
			return nil, err
		}
		g.Level = labelGrantLevel(g.Level)
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *PGUserStore) CreateGroup(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO groups (name) VALUES ($1) RETURNING id`, name).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, ErrGroupTaken
		}
		return 0, err
	}
	return id, nil
}

func (s *PGUserStore) UpdateGroupName(ctx context.Context, groupID int64, name string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE groups SET name = $1, updated_at = now() WHERE id = $2`, name, groupID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrGroupTaken
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("group tidak ditemukan")
	}
	return nil
}

func (s *PGUserStore) DeleteGroup(ctx context.Context, groupID int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM groups WHERE id = $1`, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("group tidak ditemukan")
	}
	return nil
}

func (s *PGUserStore) RemoveMember(ctx context.Context, groupID int64, userID int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM memberships WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("anggota tidak ditemukan")
	}
	return nil
}

var _ GroupStore = (*PGUserStore)(nil)

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
