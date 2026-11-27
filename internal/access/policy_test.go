package access

import (
	"context"
	"errors"
	"testing"
)

type stubLevelSource struct {
	levels []Action
	err    error
}

func (s stubLevelSource) Levels(context.Context, int64, int64) ([]Action, error) {
	return s.levels, s.err
}

func TestRankAndAtLeast(t *testing.T) {
	if Rank(ActionView) != 1 || Rank(ActionEdit) != 2 || Rank("") != 0 || Rank("admin") != 0 {
		t.Fatalf("unexpected ranks: view=%d edit=%d empty=%d unknown=%d",
			Rank(ActionView), Rank(ActionEdit), Rank(""), Rank("admin"))
	}

	cases := []struct {
		have, want Action
		ok         bool
	}{
		{ActionEdit, ActionEdit, true},
		{ActionEdit, ActionView, true},
		{ActionView, ActionView, true},
		{ActionView, ActionEdit, false},
		{"", ActionView, false},
		{ActionEdit, "", false},
		{ActionEdit, "unknown", false},
	}
	for _, tc := range cases {
		if got := AtLeast(tc.have, tc.want); got != tc.ok {
			t.Errorf("AtLeast(%q, %q) = %v, want %v", tc.have, tc.want, got, tc.ok)
		}
	}
}

func TestEffective(t *testing.T) {
	if got, ok := Effective(nil); ok || got != "" {
		t.Fatalf("empty grants: got (%q, %v), want ('', false)", got, ok)
	}
	if got, ok := Effective([]Action{"", "invalid"}); ok || got != "" {
		t.Fatalf("invalid grants: got (%q, %v), want ('', false)", got, ok)
	}
	if got, ok := Effective([]Action{ActionView}); !ok || got != ActionView {
		t.Fatalf("single view: got (%q, %v), want ('view', true)", got, ok)
	}
	if got, ok := Effective([]Action{ActionView, ActionEdit, ActionView}); !ok || got != ActionEdit {
		t.Fatalf("highest grant across groups: got (%q, %v), want ('edit', true)", got, ok)
	}
}

func TestDenyAll(t *testing.T) {
	ok, err := (DenyAll{}).Can(context.Background(), 1, 1, ActionView)
	if err != nil || ok {
		t.Fatalf("DenyAll.Can = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestPostgresPolicyCan(t *testing.T) {
	ctx := context.Background()

	t.Run("berhasil: view dan edit diizinkan sesuai level tertinggi", func(t *testing.T) {
		p := NewPolicy(stubLevelSource{levels: []Action{ActionView, ActionEdit}})
		for _, act := range []Action{ActionView, ActionEdit} {
			ok, err := p.Can(ctx, 1, 10, act)
			if err != nil || !ok {
				t.Fatalf("Can(%s) = (%v, %v), want (true, nil)", act, ok, err)
			}
		}
	})

	t.Run("akses ditolak: view-only ditolak untuk edit dan tanpa grant ditolak", func(t *testing.T) {
		viewOnly := NewPolicy(stubLevelSource{levels: []Action{ActionView}})
		if ok, err := viewOnly.Can(ctx, 1, 10, ActionEdit); err != nil || ok {
			t.Fatalf("view-only edit = (%v, %v), want (false, nil)", ok, err)
		}

		noGrant := NewPolicy(stubLevelSource{})
		if ok, err := noGrant.Can(ctx, 1, 10, ActionView); err != nil || ok {
			t.Fatalf("no-grant view = (%v, %v), want (false, nil)", ok, err)
		}

		if ok, err := viewOnly.Can(ctx, 1, 10, "unknown"); err != nil || ok {
			t.Fatalf("unknown action = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("gagal: fail-closed saat sumber grant error", func(t *testing.T) {
		boom := errors.New("db unavailable")
		p := NewPolicy(stubLevelSource{err: boom})
		ok, err := p.Can(ctx, 1, 10, ActionView)
		if ok || !errors.Is(err, boom) {
			t.Fatalf("error source = (%v, %v), want (false, boom)", ok, err)
		}
	})
}
