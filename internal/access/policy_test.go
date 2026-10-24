package access

import (
	"context"
	"errors"
	"testing"
)

type fakeSource struct {
	levels []Action
	err    error
}

func (f fakeSource) Levels(ctx context.Context, userID int64, connID int64) ([]Action, error) {
	return f.levels, f.err
}

func TestRank(t *testing.T) {
	cases := map[Action]int{
		ActionEdit:      2,
		ActionView:      1,
		Action("bogus"): 0,
		Action(""):      0,
	}
	for a, want := range cases {
		if got := Rank(a); got != want {
			t.Errorf("Rank(%q) = %d, want %d", a, got, want)
		}
	}
}

func TestEffective(t *testing.T) {
	cases := []struct {
		name   string
		levels []Action
		want   Action
		ok     bool
	}{
		{"none", nil, "", false},
		{"unknown only", []Action{"bogus"}, "", false},
		{"view", []Action{ActionView}, ActionView, true},
		{"edit", []Action{ActionEdit}, ActionEdit, true},
		{"highest wins", []Action{ActionView, ActionEdit}, ActionEdit, true},
		{"order independent", []Action{ActionEdit, ActionView}, ActionEdit, true},
		{"ignores unknown", []Action{"bogus", ActionView}, ActionView, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Effective(tc.levels)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("Effective(%v) = (%q, %v), want (%q, %v)", tc.levels, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestAtLeast(t *testing.T) {
	cases := []struct {
		have, want Action
		ok         bool
	}{
		{ActionEdit, ActionEdit, true},
		{ActionEdit, ActionView, true},
		{ActionView, ActionView, true},
		{ActionView, ActionEdit, false},
		{Action(""), ActionView, false},
		{ActionView, Action("bogus"), false},
	}
	for _, tc := range cases {
		if got := AtLeast(tc.have, tc.want); got != tc.ok {
			t.Errorf("AtLeast(%q, %q) = %v, want %v", tc.have, tc.want, got, tc.ok)
		}
	}
}

func TestPostgresPolicyCan(t *testing.T) {
	boom := errors.New("db down")
	cases := []struct {
		name    string
		src     fakeSource
		action  Action
		want    bool
		wantErr bool
	}{
		{"no grant denies view", fakeSource{}, ActionView, false, false},
		{"no grant denies edit", fakeSource{}, ActionEdit, false, false},
		{"view denies edit", fakeSource{levels: []Action{ActionView}}, ActionEdit, false, false},
		{"view allows view", fakeSource{levels: []Action{ActionView}}, ActionView, true, false},
		{"edit allows view", fakeSource{levels: []Action{ActionEdit}}, ActionView, true, false},
		{"edit allows edit", fakeSource{levels: []Action{ActionEdit}}, ActionEdit, true, false},
		{"highest group wins", fakeSource{levels: []Action{ActionView, ActionEdit}}, ActionEdit, true, false},
		{"unknown action denied", fakeSource{levels: []Action{ActionEdit}}, Action("owner"), false, false},
		{"source error fails closed", fakeSource{err: boom}, ActionView, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPolicy(tc.src)
			got, err := p.Can(context.Background(), 1, 2, tc.action)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("Can = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDenyAll(t *testing.T) {
	var p Policy = DenyAll{}
	for _, a := range []Action{ActionView, ActionEdit} {
		ok, err := p.Can(context.Background(), 1, 1, a)
		if err != nil || ok {
			t.Fatalf("DenyAll.Can(%q) = (%v, %v), want (false, nil)", a, ok, err)
		}
	}
}
