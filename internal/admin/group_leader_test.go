package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

func TestGroupLeader(t *testing.T) {
	ctx := context.Background()
	pool := testutil.NewPool(t)
	store := NewPGGroupStore(pool)

	grp, err := store.CreateGroup(ctx, "tim")
	if err != nil {
		t.Fatal(err)
	}
	var member, outsider, inactive int64
	for name, dst := range map[string]*int64{"member": &member, "outsider": &outsider, "inactive": &inactive} {
		if err := pool.QueryRow(ctx,
			`INSERT INTO users (username, password_hash, is_active) VALUES ($1, 'x', $2) RETURNING id`,
			name, name != "inactive").Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{member, inactive} {
		if err := store.AddMember(ctx, grp.ID, id); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.SetLeader(ctx, grp.ID, &outsider); !errors.Is(err, ErrLeaderNotMember) {
		t.Errorf("leader bukan anggota: err=%v", err)
	}
	if err := store.SetLeader(ctx, grp.ID, &inactive); !errors.Is(err, ErrLeaderNotMember) {
		t.Errorf("leader nonaktif: err=%v", err)
	}
	missing := grp.ID + 999
	if err := store.SetLeader(ctx, missing, &member); !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("group tidak ada: err=%v", err)
	}

	if err := store.SetLeader(ctx, grp.ID, &member); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GroupLeader(ctx, grp.ID); got == nil || *got != member {
		t.Fatalf("leader = %v, want %d", got, member)
	}

	if err := store.RemoveMember(ctx, grp.ID, member); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GroupLeader(ctx, grp.ID); got != nil {
		t.Errorf("leader masih %d setelah dikeluarkan dari group", *got)
	}

	if err := store.SetLeader(ctx, grp.ID, nil); err != nil {
		t.Errorf("kosongkan leader: %v", err)
	}
}
