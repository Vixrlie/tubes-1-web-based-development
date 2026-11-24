package dbm

// Aturan workflow murni (tanpa database): urutan linear, hide rollback, dan
// agregasi approval.

import (
	"errors"
	"fmt"
)

// Aksi eksekusi migration.
const (
	ActionUp       = "up"
	ActionRollback = "rollback"
	ActionForward  = "forward"
	// ActionResume melanjutkan eksekusi batch yang berhenti sebagian dari checkpoint.
	ActionResume = "resume"
)

// Status pengajuan (request) migration.
// Draft → Pending Review → Approved → Queued → Running → Success / Failed.
const (
	RequestPendingReview = "pending_review"
	RequestRejected      = "rejected"
	RequestApproved      = "approved"
	RequestQueued        = "queued"
	RequestRunning       = "running"
	RequestSuccess       = "success"
	RequestFailed        = "failed"
	// RequestPartial: eksekusi batch berhenti setelah sebagian batch committed.
	RequestPartial   = "partial"
	RequestCancelled = "cancelled"
)

// Status satu attempt eksekusi.
const (
	ExecRunning = "running"
	ExecSuccess = "success"
	ExecFailed  = "failed"
	// ExecPartial: sebagian batch committed lalu berhenti (final, tidak ditimpa).
	ExecPartial = "partial"
)

// Posisi migration terhadap database target, diturunkan dari eksekusi sukses.
const (
	StateDraft      = "draft"       // belum pernah dieksekusi
	StateActive     = "active"      // eksekusi sukses terakhir = up/forward
	StateRolledBack = "rolled_back" // eksekusi sukses terakhir = rollback
	StateFailed     = "failed"      // pernah dicoba tetapi belum pernah sukses
	// StatePartial: hasil terakhir = eksekusi batch yang berhenti sebagian.
	// Selesaikan dengan resume (lanjut) atau rollback (batalkan per batch).
	StatePartial = "partial"
)

var (
	ErrNotReversible   = errors.New("migration ditandai irreversible sehingga tidak dapat di-rollback")
	ErrNotActive       = errors.New("migration tidak sedang aktif sehingga tidak dapat di-rollback")
	ErrNotLatestActive = errors.New("rollback hanya dapat dilakukan pada migration aktif terakhir")
	ErrAlreadyActive   = errors.New("migration sudah aktif; rollback terlebih dahulu sebelum forward")
	ErrForwardOrder    = errors.New("forward harus mengikuti urutan linear: forward migration yang terakhir di-rollback terlebih dahulu")
	ErrUnknownAction   = errors.New("aksi tidak dikenal; gunakan up, forward, rollback, atau resume")
	ErrPartialPending  = errors.New("migration berhenti sebagian; lanjutkan dengan resume atau batalkan dengan rollback terlebih dahulu")
	ErrNotPartial      = errors.New("resume hanya untuk eksekusi batch yang berhenti sebagian")
	ErrDatabasePartial = errors.New("database ditahan karena ada migration yang berhenti sebagian; selesaikan migration tersebut terlebih dahulu")
)

// MigrationState = ringkasan posisi satu migration, dihitung dari riwayat
// eksekusi sukses pada koneksi yang sama.
type MigrationState struct {
	MigrationID int64
	Reversible  bool
	// HasAttempts true bila migration pernah dieksekusi (berhasil maupun gagal).
	HasAttempts bool
	// LastSuccessID/Action = eksekusi sukses terakhir milik migration ini.
	LastSuccessID     int64
	LastSuccessAction string
	// Diisi bila hasil terakhir (sukses atau partial) adalah eksekusi partial.
	PartialID      int64
	PartialAction  string
	PartialReverts int64
}

// setOutcome mencatat hasil terakhir (sukses/partial); hanya partial yang
// mengubah posisi, karena posisi sukses sudah dari LastSuccess*.
func (s *MigrationState) setOutcome(id int64, action, status string, reverts int64) {
	if status == ExecPartial && id > s.LastSuccessID {
		s.PartialID, s.PartialAction, s.PartialReverts = id, action, reverts
	}
}

// State menerjemahkan eksekusi sukses terakhir menjadi posisi migration.
func (s MigrationState) State() string {
	switch {
	case s.PartialID != 0:
		return StatePartial
	case s.LastSuccessID == 0 && s.HasAttempts:
		return StateFailed
	case s.LastSuccessID == 0:
		return StateDraft
	case s.LastSuccessAction == ActionRollback:
		return StateRolledBack
	default:
		return StateActive
	}
}

// ActiveExecutionID = eksekusi up/forward yang membuat migration aktif (0 bila tidak aktif).
func (s MigrationState) ActiveExecutionID() int64 {
	if s.State() == StateActive {
		return s.LastSuccessID
	}
	return 0
}

// Plan = hasil pemeriksaan sebuah aksi: aksi sebenarnya dan, untuk rollback,
// eksekusi yang akan dibatalkan.
type Plan struct {
	Action          string
	TargetExecution int64
}

// PlanAction: rollback hanya untuk eksekusi aktif dengan ID terbesar; forward
// mengikuti LIFO (A→B→C, rollback C lalu B: forward B sebelum C).
func PlanAction(states []MigrationState, migID int64, requested string) (Plan, error) {
	var self *MigrationState
	for i := range states {
		if states[i].MigrationID == migID {
			self = &states[i]
			break
		}
	}
	if self == nil {
		return Plan{}, ErrMigrationNotFound
	}

	// Database ditahan selama ada migration partial; hanya migration itu yang
	// boleh diselesaikan.
	if self.State() == StatePartial {
		return planPartial(*self, requested)
	}
	for _, other := range states {
		if other.State() == StatePartial {
			return Plan{}, fmt.Errorf("%w (migration #%d)", ErrDatabasePartial, other.MigrationID)
		}
	}

	switch requested {
	case ActionResume:
		return Plan{}, ErrNotPartial
	case ActionRollback:
		if self.State() != StateActive {
			return Plan{}, ErrNotActive
		}
		if !self.Reversible {
			return Plan{}, ErrNotReversible
		}
		for _, other := range states {
			if other.ActiveExecutionID() > self.ActiveExecutionID() {
				return Plan{}, fmt.Errorf("%w (migration aktif terakhir: #%d)", ErrNotLatestActive, other.MigrationID)
			}
		}
		return Plan{Action: ActionRollback, TargetExecution: self.ActiveExecutionID()}, nil

	case ActionUp, ActionForward:
		switch self.State() {
		case StateActive:
			return Plan{}, ErrAlreadyActive
		case StateDraft:
			return Plan{Action: ActionUp}, nil
		case StateRolledBack:
			for _, other := range states {
				if other.State() == StateRolledBack && other.LastSuccessID > self.LastSuccessID {
					return Plan{}, fmt.Errorf("%w (migration #%d)", ErrForwardOrder, other.MigrationID)
				}
			}
		}
		return Plan{Action: ActionForward}, nil
	}
	return Plan{}, ErrUnknownAction
}

// planPartial: up/forward/resume yang berhenti sebagian boleh di-resume atau
// dibatalkan; rollback yang berhenti sebagian (partially reverted) hanya boleh
// dilanjutkan pembatalannya.
func planPartial(self MigrationState, requested string) (Plan, error) {
	reverting := self.PartialAction == ActionRollback
	switch requested {
	case ActionResume:
		if reverting {
			return Plan{}, ErrPartialPending
		}
		return Plan{Action: ActionResume, TargetExecution: self.PartialID}, nil
	case ActionRollback:
		if !self.Reversible {
			return Plan{}, ErrNotReversible
		}
		if reverting {
			return Plan{Action: ActionRollback, TargetExecution: self.PartialReverts}, nil
		}
		return Plan{Action: ActionRollback, TargetExecution: self.PartialID}, nil
	case ActionUp, ActionForward:
		return Plan{}, ErrPartialPending
	}
	return Plan{}, ErrUnknownAction
}

// Execution = satu attempt eksekusi (baris migration_executions).
// HideRolledBack hanya membutuhkan ID, status, dan referensi rollback.
type ExecutionRef interface {
	ExecID() int64
	ExecStatus() string
	RevertsID() int64
}

// HideRolledBack menyembunyikan pasangan berdasarkan execution ID; attempt
// gagal tetap terlihat.
func HideRolledBack[T ExecutionRef](items []T) []T {
	hidden := make(map[int64]bool)
	for _, it := range items {
		if it.ExecStatus() == ExecSuccess && it.RevertsID() != 0 {
			hidden[it.ExecID()] = true
			hidden[it.RevertsID()] = true
		}
	}
	out := make([]T, 0, len(items))
	for _, it := range items {
		if !hidden[it.ExecID()] {
			out = append(out, it)
		}
	}
	return out
}

// Keputusan review per group.
const (
	ReviewPending  = "pending"
	ReviewApproved = "approved"
	ReviewRejected = "rejected"
)

// ReviewerGroup = group yang memiliki akses (view atau edit) ke database dan
// karena itu wajib memberi approval melalui leadernya.
type ReviewerGroup struct {
	GroupID    int64  `json:"group_id"`
	GroupName  string `json:"group_name"`
	Level      string `json:"level"`
	LeaderID   int64  `json:"leader_id,omitempty"`
	LeaderName string `json:"leader_name,omitempty"`
	Decision   string `json:"decision"`
	Note       string `json:"note,omitempty"`
	DecidedBy  string `json:"decided_by,omitempty"`
}

// ReviewOutcome: satu penolakan cukup untuk menolak.
func ReviewOutcome(groups []ReviewerGroup) string {
	if len(groups) == 0 {
		return ReviewPending
	}
	approved := 0
	for _, g := range groups {
		switch g.Decision {
		case ReviewRejected:
			return ReviewRejected
		case ReviewApproved:
			approved++
		}
	}
	if approved == len(groups) {
		return ReviewApproved
	}
	return ReviewPending
}

// PendingGroupsLedBy mengembalikan group yang masih menunggu keputusan dan
// dipimpin userID. Satu leader dapat memimpin beberapa group sekaligus.
func PendingGroupsLedBy(groups []ReviewerGroup, userID int64) []ReviewerGroup {
	var out []ReviewerGroup
	for _, g := range groups {
		if g.LeaderID == userID && g.Decision == ReviewPending {
			out = append(out, g)
		}
	}
	return out
}

// RequestOpen true bila pengajuan masih berjalan (belum final).
func RequestOpen(status string) bool {
	switch status {
	case RequestPendingReview, RequestApproved, RequestQueued, RequestRunning:
		return true
	}
	return false
}
