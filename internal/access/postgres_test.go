package access

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

func TestPGLevelSourceAndPolicy(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()

	adminID := mustUser(t, pool, "admin1", true)
	activeID := mustUser(t, pool, "alice", true)
	inactiveID := mustUser(t, pool, "bob", false)
	connID := mustConnection(t, pool, "sample")
	otherConnID := mustConnection(t, pool, "other")

	viewGroup := mustGroup(t, pool, "viewers")
	editGroup := mustGroup(t, pool, "editors")

	mustMembership(t, pool, activeID, viewGroup)
	mustMembership(t, pool, activeID, editGroup)
	mustMembership(t, pool, inactiveID, editGroup)
	mustGrant(t, pool, viewGroup, connID, "view")
	mustGrant(t, pool, editGroup, connID, "edit")
	mustGrant(t, pool, editGroup, otherConnID, "edit")

	src := NewPGLevelSource(pool)
	policy := NewPolicy(src)

	if ok, err := policy.Can(ctx, activeID, connID, ActionView); err != nil || !ok {
		t.Fatalf("active user view: got (%v, %v), want (true, nil)", ok, err)
	}
	if ok, err := policy.Can(ctx, activeID, connID, ActionEdit); err != nil || !ok {
		t.Fatalf("active user edit via multiple groups: got (%v, %v), want (true, nil)", ok, err)
	}
	if ok, err := policy.Can(ctx, inactiveID, connID, ActionView); err != nil || ok {
		t.Fatalf("inactive user: got (%v, %v), want (false, nil)", ok, err)
	}
	if ok, err := policy.Can(ctx, adminID, connID, ActionView); err != nil || ok {
		t.Fatalf("platform admin without grant: got (%v, %v), want (false, nil)", ok, err)
	}
	if ok, err := policy.Can(ctx, activeID, 999999, ActionView); err != nil || ok {
		t.Fatalf("unknown connection: got (%v, %v), want (false, nil)", ok, err)
	}
}

func mustUser(t *testing.T, pool *pgxpool.Pool, username string, active bool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash, is_active) VALUES ($1, 'x', $2) RETURNING id`,
		username, active).Scan(&id)
	if err != nil {
		t.Fatalf("insert user %q: %v", username, err)
	}
	return id
}

func mustGroup(t *testing.T, pool *pgxpool.Pool, name string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO groups (name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
		t.Fatalf("insert group %q: %v", name, err)
	}
	return id
}

func mustConnection(t *testing.T, pool *pgxpool.Pool, name string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO db_connections (name, connection_url) VALUES ($1, 'postgres://x') RETURNING id`,
		name).Scan(&id); err != nil {
		t.Fatalf("insert connection %q: %v", name, err)
	}
	return id
}

func mustMembership(t *testing.T, pool *pgxpool.Pool, userID, groupID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO memberships (user_id, group_id) VALUES ($1, $2)`, userID, groupID); err != nil {
		t.Fatalf("insert membership (%d,%d): %v", userID, groupID, err)
	}
}

func mustGrant(t *testing.T, pool *pgxpool.Pool, groupID, connID int64, level string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO grants (group_id, connection_id, level) VALUES ($1, $2, $3)`,
		groupID, connID, level); err != nil {
		t.Fatalf("insert grant (%d,%d,%s): %v", groupID, connID, level, err)
	}
}
