package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

func TestPGGroupStore(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()
	s := NewPGGroupStore(pool)

	g, err := s.CreateGroup(ctx, "developers")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if g.ID == 0 || g.Name != "developers" {
		t.Fatalf("CreateGroup returned %+v", g)
	}

	if _, err := s.CreateGroup(ctx, "developers"); !errors.Is(err, ErrGroupNameTaken) {
		t.Fatalf("duplicate CreateGroup err = %v, want ErrGroupNameTaken", err)
	}

	other, err := s.CreateGroup(ctx, "qa")
	if err != nil {
		t.Fatalf("CreateGroup qa: %v", err)
	}

	groups, err := s.ListGroups(ctx)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("ListGroups len = %d, want 2", len(groups))
	}

	if _, err := s.GetGroup(ctx, g.ID); err != nil {
		t.Fatalf("GetGroup: %v", err)
	}
	if _, err := s.GetGroup(ctx, 999999); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("GetGroup missing err = %v, want ErrGroupNotFound", err)
	}

	if err := s.RenameGroup(ctx, g.ID, "platform"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	if err := s.RenameGroup(ctx, g.ID, "qa"); !errors.Is(err, ErrGroupNameTaken) {
		t.Fatalf("RenameGroup conflict err = %v, want ErrGroupNameTaken", err)
	}
	if err := s.RenameGroup(ctx, 999999, "x"); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("RenameGroup missing err = %v, want ErrGroupNotFound", err)
	}

	alice := testUser(t, pool, "alice")
	bob := testUser(t, pool, "bob")

	if err := s.AddMember(ctx, g.ID, alice); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if err := s.AddMember(ctx, g.ID, bob); err != nil {
		t.Fatalf("AddMember bob: %v", err)
	}
	if err := s.AddMember(ctx, g.ID, bob); err != nil {
		t.Fatalf("AddMember duplicate should be idempotent: %v", err)
	}
	if err := s.AddMember(ctx, g.ID, 999999); !errors.Is(err, auth.ErrUserNotFound) {
		t.Fatalf("AddMember missing user err = %v, want ErrUserNotFound", err)
	}

	if err := s.AddMember(ctx, 999999, alice); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("AddMember missing group err = %v, want ErrGroupNotFound", err)
	}

	members, err := s.ListMembers(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("ListMembers len = %d, want 2", len(members))
	}

	ids, err := s.MemberIDs(ctx, g.ID)
	if err != nil {
		t.Fatalf("MemberIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("MemberIDs len = %d, want 2", len(ids))
	}

	if err := s.RemoveMember(ctx, g.ID, alice); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if err := s.RemoveMember(ctx, g.ID, alice); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("RemoveMember repeat err = %v, want ErrMemberNotFound", err)
	}

	conn := testConnection(t, pool, "sample")

	if err := s.SetGrant(ctx, g.ID, conn, access.ActionView); err != nil {
		t.Fatalf("SetGrant view: %v", err)
	}
	if level, ok, err := s.GetGrant(ctx, g.ID, conn); err != nil || !ok || level != access.ActionView {
		t.Fatalf("GetGrant view = (%q, %v, %v)", level, ok, err)
	}
	if err := s.SetGrant(ctx, g.ID, conn, access.ActionEdit); err != nil {
		t.Fatalf("SetGrant upgrade: %v", err)
	}
	if level, _, _ := s.GetGrant(ctx, g.ID, conn); level != access.ActionEdit {
		t.Fatalf("GetGrant after upgrade = %q, want edit", level)
	}
	if err := s.SetGrant(ctx, g.ID, conn, access.Action("owner")); !errors.Is(err, ErrInvalidLevel) {
		t.Fatalf("SetGrant invalid err = %v, want ErrInvalidLevel", err)
	}
	if err := s.SetGrant(ctx, g.ID, 999999, access.ActionView); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("SetGrant missing connection err = %v, want ErrConnectionNotFound", err)
	}
	if err := s.SetGrant(ctx, 999999, conn, access.ActionView); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("SetGrant missing group err = %v, want ErrGroupNotFound", err)
	}

	grants, err := s.ListGrants(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListGrants: %v", err)
	}
	if len(grants) != 1 || grants[0].ConnectionName != "sample" || grants[0].Level != access.ActionEdit {
		t.Fatalf("ListGrants = %+v", grants)
	}

	if err := s.RevokeGrant(ctx, g.ID, conn); err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}
	if err := s.RevokeGrant(ctx, g.ID, conn); !errors.Is(err, ErrGrantNotFound) {
		t.Fatalf("RevokeGrant repeat err = %v, want ErrGrantNotFound", err)
	}
	if _, ok, _ := s.GetGrant(ctx, g.ID, conn); ok {
		t.Fatal("GetGrant still reports a grant after revoke")
	}

	// Delete cascades memberships and grants.
	if err := s.AddMember(ctx, g.ID, bob); err != nil {
		t.Fatalf("re-add bob: %v", err)
	}
	if err := s.SetGrant(ctx, g.ID, conn, access.ActionView); err != nil {
		t.Fatalf("re-add grant: %v", err)
	}
	if err := s.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if err := s.DeleteGroup(ctx, g.ID); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("DeleteGroup repeat err = %v, want ErrGroupNotFound", err)
	}
	if _, err := s.GetGroup(ctx, g.ID); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("GetGroup after delete err = %v, want ErrGroupNotFound", err)
	}

	var memberships, grantsCount int
	if err := pool.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM memberships WHERE group_id = $1),
		        (SELECT count(*) FROM grants WHERE group_id = $1)`, g.ID).
		Scan(&memberships, &grantsCount); err != nil {
		t.Fatalf("count after delete: %v", err)
	}
	if memberships != 0 || grantsCount != 0 {
		t.Fatalf("cascade failed: memberships=%d grants=%d", memberships, grantsCount)
	}

	// Unrelated group survives.
	if _, err := s.GetGroup(ctx, other.ID); err != nil {
		t.Fatalf("other group should survive: %v", err)
	}
}

func TestPGConnectionLister(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()

	want := testConnection(t, pool, "sample")

	lister := NewPGConnectionLister(pool)
	conns, err := lister.ListConnections(ctx)
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(conns) != 1 || conns[0].ID != want || conns[0].Name != "sample" {
		t.Fatalf("ListConnections = %+v", conns)
	}
}

func testUser(t *testing.T, pool *pgxpool.Pool, username string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		username).Scan(&id); err != nil {
		t.Fatalf("insert user %q: %v", username, err)
	}
	return id
}

func testConnection(t *testing.T, pool *pgxpool.Pool, name string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO db_connections (name, connection_url) VALUES ($1, 'postgres://x') RETURNING id`,
		name).Scan(&id); err != nil {
		t.Fatalf("insert connection %q: %v", name, err)
	}
	return id
}
