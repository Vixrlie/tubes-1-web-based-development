package admin

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

var (
	ErrGroupNotFound      = errors.New("group tidak ditemukan")
	ErrGroupNameTaken     = errors.New("nama group sudah dipakai")
	ErrMemberNotFound     = errors.New("anggota tidak ditemukan")
	ErrGrantNotFound      = errors.New("grant tidak ditemukan")
	ErrConnectionNotFound = errors.New("koneksi tidak ditemukan")
	ErrInvalidLevel       = errors.New("level akses tidak valid")
)

// Group adalah satu baris grup beserta jumlah anggotanya.
type Group struct {
	ID          int64
	Name        string
	CreatedAt   time.Time
	MemberCount int
}

// GrantRow adalah grant sebuah group pada satu koneksi.
type GrantRow struct {
	GroupID        int64
	ConnectionID   int64
	ConnectionName string
	Level          access.Action
}

// ConnectionRef adalah referensi koneksi target untuk ditampilkan di UI.
// Kredensial tidak pernah ikut.
type ConnectionRef struct {
	ID   int64
	Name string
}

// ConnectionLister membaca daftar koneksi target. Dimiliki Majiid; di sini
// hanya dipakai untuk pilihan pada form grant.
type ConnectionLister interface {
	ListConnections(ctx context.Context) ([]ConnectionRef, error)
}

// GroupStore mengelola group, anggota, dan grant.
type GroupStore interface {
	ListGroups(ctx context.Context) ([]Group, error)
	GetGroup(ctx context.Context, id int64) (Group, error)
	CreateGroup(ctx context.Context, name string) (Group, error)
	RenameGroup(ctx context.Context, id int64, name string) error
	DeleteGroup(ctx context.Context, id int64) error

	ListMembers(ctx context.Context, groupID int64) ([]UserRow, error)
	AddMember(ctx context.Context, groupID, userID int64) error
	RemoveMember(ctx context.Context, groupID, userID int64) error
	MemberIDs(ctx context.Context, groupID int64) ([]int64, error)

	ListGrants(ctx context.Context, groupID int64) ([]GrantRow, error)
	GetGrant(ctx context.Context, groupID, connID int64) (access.Action, bool, error)
	SetGrant(ctx context.Context, groupID, connID int64, level access.Action) error
	RevokeGrant(ctx context.Context, groupID, connID int64) error
}

// PGGroupStore adalah implementasi GroupStore di atas PostgreSQL.
type PGGroupStore struct {
	pool *pgxpool.Pool
}

var _ GroupStore = (*PGGroupStore)(nil)

// NewPGGroupStore membuat GroupStore berbasis PostgreSQL.
func NewPGGroupStore(pool *pgxpool.Pool) *PGGroupStore {
	return &PGGroupStore{pool: pool}
}

func (s *PGGroupStore) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT gr.id, gr.name, gr.created_at, COUNT(m.user_id)
		   FROM groups gr
		   LEFT JOIN memberships m ON m.group_id = gr.id
		  GROUP BY gr.id, gr.name, gr.created_at
		  ORDER BY gr.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt, &g.MemberCount); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *PGGroupStore) GetGroup(ctx context.Context, id int64) (Group, error) {
	var g Group
	err := s.pool.QueryRow(ctx,
		`SELECT gr.id, gr.name, gr.created_at, COUNT(m.user_id)
		   FROM groups gr
		   LEFT JOIN memberships m ON m.group_id = gr.id
		  WHERE gr.id = $1
		  GROUP BY gr.id, gr.name, gr.created_at`, id).
		Scan(&g.ID, &g.Name, &g.CreatedAt, &g.MemberCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrGroupNotFound
	}
	return g, err
}

func (s *PGGroupStore) CreateGroup(ctx context.Context, name string) (Group, error) {
	var g Group
	err := s.pool.QueryRow(ctx,
		`INSERT INTO groups (name) VALUES ($1)
		 RETURNING id, name, created_at`, name).
		Scan(&g.ID, &g.Name, &g.CreatedAt)
	if constraintOf(err) == "groups_name_key" {
		return Group{}, ErrGroupNameTaken
	}
	return g, err
}

func (s *PGGroupStore) RenameGroup(ctx context.Context, id int64, name string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE groups SET name = $2, updated_at = now() WHERE id = $1`, id, name)
	if constraintOf(err) == "groups_name_key" {
		return ErrGroupNameTaken
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrGroupNotFound
	}
	return nil
}

func (s *PGGroupStore) DeleteGroup(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM groups WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrGroupNotFound
	}
	return nil
}

func (s *PGGroupStore) ListMembers(ctx context.Context, groupID int64) ([]UserRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT u.id, u.username, COALESCE(u.full_name, ''), COALESCE(u.email, ''),
		        u.is_platform_admin, u.is_active, u.created_at
		   FROM users u
		   JOIN memberships m ON m.user_id = u.id
		  WHERE m.group_id = $1
		  ORDER BY u.username`, groupID)
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

func (s *PGGroupStore) AddMember(ctx context.Context, groupID, userID int64) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO memberships (user_id, group_id) VALUES ($1, $2)
		 ON CONFLICT (user_id, group_id) DO NOTHING`, userID, groupID)
	switch constraintOf(err) {
	case "memberships_user_id_fkey":
		return auth.ErrUserNotFound
	case "memberships_group_id_fkey":
		return ErrGroupNotFound
	}
	return err
}

func (s *PGGroupStore) RemoveMember(ctx context.Context, groupID, userID int64) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM memberships WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (s *PGGroupStore) MemberIDs(ctx context.Context, groupID int64) ([]int64, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT user_id FROM memberships WHERE group_id = $1`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *PGGroupStore) ListGrants(ctx context.Context, groupID int64) ([]GrantRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT gr.group_id, c.id, c.name, gr.level
		   FROM grants gr
		   JOIN db_connections c ON c.id = gr.connection_id
		  WHERE gr.group_id = $1
		  ORDER BY c.name`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GrantRow
	for rows.Next() {
		var g GrantRow
		var level string
		if err := rows.Scan(&g.GroupID, &g.ConnectionID, &g.ConnectionName, &level); err != nil {
			return nil, err
		}
		g.Level = access.Action(level)
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *PGGroupStore) GetGrant(ctx context.Context, groupID, connID int64) (access.Action, bool, error) {
	var level string
	err := s.pool.QueryRow(ctx,
		`SELECT level FROM grants WHERE group_id = $1 AND connection_id = $2`,
		groupID, connID).Scan(&level)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return access.Action(level), true, nil
}

func (s *PGGroupStore) SetGrant(ctx context.Context, groupID, connID int64, level access.Action) error {
	if level != access.ActionView && level != access.ActionEdit {
		return ErrInvalidLevel
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO grants (group_id, connection_id, level) VALUES ($1, $2, $3)
		 ON CONFLICT (group_id, connection_id) DO UPDATE SET level = EXCLUDED.level`,
		groupID, connID, string(level))
	switch constraintOf(err) {
	case "grants_group_id_fkey":
		return ErrGroupNotFound
	case "grants_connection_id_fkey":
		return ErrConnectionNotFound
	}
	return err
}

func (s *PGGroupStore) RevokeGrant(ctx context.Context, groupID, connID int64) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM grants WHERE group_id = $1 AND connection_id = $2`, groupID, connID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrGrantNotFound
	}
	return nil
}

// PGConnectionLister membaca db_connections tanpa kredensial.
type PGConnectionLister struct {
	pool *pgxpool.Pool
}

var _ ConnectionLister = (*PGConnectionLister)(nil)

// NewPGConnectionLister membuat ConnectionLister berbasis PostgreSQL.
func NewPGConnectionLister(pool *pgxpool.Pool) *PGConnectionLister {
	return &PGConnectionLister{pool: pool}
}

func (l *PGConnectionLister) ListConnections(ctx context.Context) ([]ConnectionRef, error) {
	rows, err := l.pool.Query(ctx, `SELECT id, name FROM db_connections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ConnectionRef
	for rows.Next() {
		var c ConnectionRef
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
