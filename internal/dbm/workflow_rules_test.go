package dbm

import (
	"errors"
	"testing"
)

type fakeExec struct {
	id, reverts int64
	status      string
}

func (f fakeExec) ExecID() int64      { return f.id }
func (f fakeExec) ExecStatus() string { return f.status }
func (f fakeExec) RevertsID() int64   { return f.reverts }

func ids(items []fakeExec) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.id
	}
	return out
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestHideRolledBackSpecExample(t *testing.T) {
	// Contoh spesifikasi: A[E1], B[E2], B rollback E2 [E3], B forward [E4], C[E5].
	history := []fakeExec{
		{id: 1, status: ExecSuccess},
		{id: 2, status: ExecSuccess},
		{id: 3, status: ExecSuccess, reverts: 2},
		{id: 4, status: ExecSuccess},
		{id: 5, status: ExecSuccess},
	}
	got := ids(HideRolledBack(history))
	if want := []int64{1, 4, 5}; !equalIDs(got, want) {
		t.Errorf("hide rollback = %v, want %v", got, want)
	}
	if len(history) != 5 {
		t.Error("filter tidak boleh mengubah data riwayat")
	}
}

func TestHideRolledBackKeepsFailures(t *testing.T) {
	history := []fakeExec{
		{id: 1, status: ExecSuccess},
		{id: 2, status: ExecFailed},              // attempt gagal tetap terlihat
		{id: 3, status: ExecFailed, reverts: 1},  // rollback gagal tidak menyembunyikan E1
		{id: 4, status: ExecSuccess, reverts: 1}, // rollback sukses: E1 dan E4 disembunyikan
	}
	got := ids(HideRolledBack(history))
	if want := []int64{2, 3}; !equalIDs(got, want) {
		t.Errorf("hide rollback = %v, want %v", got, want)
	}
}

// states membangun A→B→C seperti contoh spesifikasi.
func linearStates() []MigrationState {
	return []MigrationState{
		{MigrationID: 1, Reversible: true, HasAttempts: true, LastSuccessID: 10, LastSuccessAction: ActionUp},
		{MigrationID: 2, Reversible: true, HasAttempts: true, LastSuccessID: 11, LastSuccessAction: ActionUp},
		{MigrationID: 3, Reversible: true, HasAttempts: true, LastSuccessID: 12, LastSuccessAction: ActionUp},
	}
}

func TestPlanRollbackOnlyLatestActive(t *testing.T) {
	states := linearStates()

	if _, err := PlanAction(states, 2, ActionRollback); !errors.Is(err, ErrNotLatestActive) {
		t.Errorf("rollback B sebelum C: err = %v, want ErrNotLatestActive", err)
	}
	plan, err := PlanAction(states, 3, ActionRollback)
	if err != nil {
		t.Fatalf("rollback C: %v", err)
	}
	if plan.Action != ActionRollback || plan.TargetExecution != 12 {
		t.Errorf("plan = %+v, want rollback target 12", plan)
	}

	// Setelah C di-rollback (E13), B menjadi aktif terakhir.
	states[2].LastSuccessID, states[2].LastSuccessAction = 13, ActionRollback
	if _, err := PlanAction(states, 2, ActionRollback); err != nil {
		t.Errorf("rollback B setelah C: %v", err)
	}
	if _, err := PlanAction(states, 3, ActionRollback); !errors.Is(err, ErrNotActive) {
		t.Errorf("rollback C dua kali: err = %v, want ErrNotActive", err)
	}
}

func TestPlanRollbackIrreversible(t *testing.T) {
	states := linearStates()
	states[2].Reversible = false
	if _, err := PlanAction(states, 3, ActionRollback); !errors.Is(err, ErrNotReversible) {
		t.Errorf("err = %v, want ErrNotReversible", err)
	}
}

func TestPlanForwardLinearOrder(t *testing.T) {
	states := linearStates()
	// Rollback C (E13) lalu rollback B (E14).
	states[2].LastSuccessID, states[2].LastSuccessAction = 13, ActionRollback
	states[1].LastSuccessID, states[1].LastSuccessAction = 14, ActionRollback

	if _, err := PlanAction(states, 3, ActionForward); !errors.Is(err, ErrForwardOrder) {
		t.Errorf("forward C sebelum B: err = %v, want ErrForwardOrder", err)
	}
	plan, err := PlanAction(states, 2, ActionForward)
	if err != nil || plan.Action != ActionForward {
		t.Fatalf("forward B: plan=%+v err=%v", plan, err)
	}

	// Setelah B di-forward (E15), C boleh di-forward. Forward B tidak
	// otomatis menjalankan C.
	states[1].LastSuccessID, states[1].LastSuccessAction = 15, ActionForward
	if states[2].State() != StateRolledBack {
		t.Error("forward B tidak boleh mengubah posisi C")
	}
	if _, err := PlanAction(states, 3, ActionForward); err != nil {
		t.Errorf("forward C setelah B: %v", err)
	}
}

func TestPlanApplyStates(t *testing.T) {
	states := []MigrationState{
		{MigrationID: 1, Reversible: true},                                                                   // draft
		{MigrationID: 2, Reversible: true, HasAttempts: true},                                                // gagal
		{MigrationID: 3, Reversible: true, HasAttempts: true, LastSuccessID: 5, LastSuccessAction: ActionUp}, // aktif
	}
	cases := []struct {
		id         int64
		requested  string
		wantAction string
		wantErr    error
	}{
		{1, ActionUp, ActionUp, nil},
		{1, ActionForward, ActionUp, nil}, // belum pernah dieksekusi = up
		{2, ActionUp, ActionForward, nil}, // attempt gagal boleh di-forward lagi
		{3, ActionForward, "", ErrAlreadyActive},
		{1, ActionRollback, "", ErrNotActive},
		{1, "drop", "", ErrUnknownAction},
		{99, ActionUp, "", ErrMigrationNotFound},
	}
	for _, tc := range cases {
		plan, err := PlanAction(states, tc.id, tc.requested)
		if tc.wantErr != nil {
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("migration %d %s: err = %v, want %v", tc.id, tc.requested, err, tc.wantErr)
			}
			continue
		}
		if err != nil || plan.Action != tc.wantAction {
			t.Errorf("migration %d %s: plan=%+v err=%v, want %s", tc.id, tc.requested, plan, err, tc.wantAction)
		}
	}
}

func TestMigrationStateLabels(t *testing.T) {
	cases := []struct {
		s    MigrationState
		want string
	}{
		{MigrationState{}, StateDraft},
		{MigrationState{HasAttempts: true}, StateFailed},
		{MigrationState{HasAttempts: true, LastSuccessID: 1, LastSuccessAction: ActionUp}, StateActive},
		{MigrationState{HasAttempts: true, LastSuccessID: 2, LastSuccessAction: ActionForward}, StateActive},
		{MigrationState{HasAttempts: true, LastSuccessID: 3, LastSuccessAction: ActionRollback}, StateRolledBack},
	}
	for _, tc := range cases {
		if got := tc.s.State(); got != tc.want {
			t.Errorf("State(%+v) = %s, want %s", tc.s, got, tc.want)
		}
	}
}

func TestReviewOutcome(t *testing.T) {
	g := func(decision string) ReviewerGroup { return ReviewerGroup{Decision: decision} }
	cases := []struct {
		name   string
		groups []ReviewerGroup
		want   string
	}{
		{"tanpa group", nil, ReviewPending},
		{"satu tim disetujui", []ReviewerGroup{g(ReviewApproved)}, ReviewApproved},
		{"masih menunggu view-only", []ReviewerGroup{g(ReviewApproved), g(ReviewPending)}, ReviewPending},
		{"semua setuju", []ReviewerGroup{g(ReviewApproved), g(ReviewApproved)}, ReviewApproved},
		{"satu menolak", []ReviewerGroup{g(ReviewApproved), g(ReviewRejected), g(ReviewPending)}, ReviewRejected},
	}
	for _, tc := range cases {
		if got := ReviewOutcome(tc.groups); got != tc.want {
			t.Errorf("%s: ReviewOutcome = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestPendingGroupsLedBy(t *testing.T) {
	groups := []ReviewerGroup{
		{GroupID: 1, LeaderID: 7, Decision: ReviewPending},
		{GroupID: 2, LeaderID: 7, Decision: ReviewApproved},
		{GroupID: 3, LeaderID: 8, Decision: ReviewPending},
		{GroupID: 4, LeaderID: 7, Decision: ReviewPending},
	}
	got := PendingGroupsLedBy(groups, 7)
	if len(got) != 2 || got[0].GroupID != 1 || got[1].GroupID != 4 {
		t.Errorf("PendingGroupsLedBy = %+v, want group 1 dan 4", got)
	}
	if len(PendingGroupsLedBy(groups, 99)) != 0 {
		t.Error("bukan leader tidak boleh mendapat group")
	}
}
