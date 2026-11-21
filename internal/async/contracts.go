package async

import (
	"context"
	"time"
)

// ApprovalChecker memverifikasi bahwa perubahan database (up, down/rollback,
// forward, resume, cancel_batch) telah disetujui sebelum masuk queue.
// Implementasi konkret akan disambungkan dengan modul workflow & approval.
type ApprovalChecker interface {
	EnsureApproved(ctx context.Context, connID, migrationID int64, action Action) error
}

// AllowAllApproval adalah stub sementara sebelum modul approval terintegrasi.
type AllowAllApproval struct{}

func (AllowAllApproval) EnsureApproved(context.Context, int64, int64, Action) error {
	return nil
}

// AttemptRecord menyimpan detail satu attempt eksekusi migration untuk dicatat
// ke migration_history (baik berhasil maupun gagal).
type AttemptRecord struct {
	JobID        int64
	MigrationID  int64
	ExecutionID  *int64
	ConnectionID int64
	Action       Action
	Mode         Mode
	SQL          string
	Status       Status
	Error        string
	ExecutedBy   *int64
	StartedAt    time.Time
	FinishedAt   time.Time
}

// HistoryRecorder mencatat setiap attempt eksekusi ke riwayat migration.
// Implementasi konkret dapat disesuaikan dengan skema migration_history.
type HistoryRecorder interface {
	RecordAttempt(ctx context.Context, rec AttemptRecord) error
}

// NoopHistoryRecorder adalah stub default bila recorder belum disuntikkan.
type NoopHistoryRecorder struct{}

func (NoopHistoryRecorder) RecordAttempt(context.Context, AttemptRecord) error {
	return nil
}

// TracePropagator menyuntikkan dan mengekstrak trace context OpenTelemetry
// melalui payload queue (API -> queue -> worker -> lock -> database).
// Implementasi konkret akan disambungkan dengan modul tracing.
type TracePropagator interface {
	Inject(ctx context.Context) map[string]string
	Extract(ctx context.Context, carrier map[string]string) context.Context
}

// NoopTracePropagator adalah stub default sebelum OTel tracer terintegrasi.
type NoopTracePropagator struct{}

func (NoopTracePropagator) Inject(context.Context) map[string]string {
	return map[string]string{}
}

func (NoopTracePropagator) Extract(ctx context.Context, _ map[string]string) context.Context {
	return ctx
}

// MetricsObserver menyediakan hook metrik ber-kardinalitas rendah untuk bonus
// Observability (Prometheus/Grafana). Hindari label user_id, job_id, atau SQL.
type MetricsObserver interface {
	OnJobEnqueued(mode Mode, action Action)
	OnJobStarted(mode Mode, action Action, queueWait time.Duration)
	OnJobFinished(mode Mode, action Action, status Status, duration time.Duration)
	OnJobRetry(mode Mode, action Action)
	OnLockWait(duration time.Duration)
	OnBatchCommitted(rows int64)
}

// NoopMetricsObserver adalah stub default bila metrik belum disuntikkan.
type NoopMetricsObserver struct{}

func (NoopMetricsObserver) OnJobEnqueued(Mode, Action)                         {}
func (NoopMetricsObserver) OnJobStarted(Mode, Action, time.Duration)           {}
func (NoopMetricsObserver) OnJobFinished(Mode, Action, Status, time.Duration)  {}
func (NoopMetricsObserver) OnJobRetry(Mode, Action)                            {}
func (NoopMetricsObserver) OnLockWait(time.Duration)                           {}
func (NoopMetricsObserver) OnBatchCommitted(int64)                             {}
