package async

import (
	"errors"
	"time"
)

// Action mendefinisikan jenis eksekusi migration yang diproses oleh worker.
type Action string

const (
	ActionUp          Action = "up"
	ActionDown        Action = "down"
	ActionForward     Action = "forward"
	ActionResume      Action = "resume"
	ActionCancelBatch Action = "cancel_batch"
)

// Mode mendefinisikan apakah migration dijalankan sekaligus atau per batch.
type Mode string

const (
	ModeNormal Mode = "normal"
	ModeBatch  Mode = "batch"
)

// Status merepresentasikan siklus hidup job asynchronous.
type Status string

const (
	StatusPending           Status = "pending"
	StatusRunning           Status = "running"
	StatusSuccess           Status = "success"
	StatusFailed            Status = "failed"
	StatusPartial           Status = "partial"
	StatusPartiallyReverted Status = "partially_reverted"
	StatusReverted          Status = "reverted"
)

// CheckpointStatus menandakan status satu batch yang pernah di-commit.
type CheckpointStatus string

const (
	CheckpointCommitted CheckpointStatus = "committed"
	CheckpointReverted  CheckpointStatus = "reverted"
)

var (
	ErrJobNotFound       = errors.New("job tidak ditemukan")
	ErrNotApproved       = errors.New("migration belum mendapat approval")
	ErrDatabaseLocked    = errors.New("database sedang menjalankan migration lain")
	ErrDatabaseBlocked   = errors.New("database memiliki migration partial/belum pasti yang harus diselesaikan dulu")
	ErrInvalidJobState   = errors.New("status job tidak valid untuk aksi ini")
	ErrUnsafeCancel      = errors.New("pembatalan batch tidak aman; buat migration perbaikan baru")
	ErrDuplicateJob      = errors.New("job dengan idempotency key ini sudah ada")
	ErrInvalidBatchInput = errors.New("konfigurasi batch tidak valid")
)

// Job adalah representasi satu pekerjaan migration pada queue persisten.
type Job struct {
	ID               int64
	MigrationID      int64
	ExecutionID      *int64
	ConnectionID     int64
	Action           Action
	Mode             Mode
	SQLUp            string
	SQLDown          string
	Irreversible     bool
	BatchTable       string
	BatchColumn      string
	BatchSize        int
	CursorMin        string
	CursorMax        string
	LastCursor       string
	CommittedBatches int
	CommittedRows    int64
	Status           Status
	ParentJobID      *int64
	IdempotencyKey   string
	Attempt          int
	MaxRetries       int
	NextRetryAt      time.Time
	TraceContext     map[string]string
	Error            string
	CreatedBy        *int64
	CreatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
	UpdatedAt        time.Time
}

// Lock merepresentasikan lease lock aktif untuk satu database target.
type Lock struct {
	ConnectionID   int64
	JobID          int64
	WorkerID       string
	LockedAt       time.Time
	LeaseExpiresAt time.Time
}

// Checkpoint mencatat satu batch yang sudah berhasil di-commit pada target DB.
type Checkpoint struct {
	ID           int64
	JobID        int64
	BatchNumber  int
	CursorStart  string
	CursorEnd    string
	RowsAffected int64
	Status       CheckpointStatus
	RecoveryData []byte
	CommittedAt  time.Time
	RevertedAt   *time.Time
}
