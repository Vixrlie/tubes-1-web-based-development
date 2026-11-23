package async

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

const (
	defaultConcurrency      = 2
	defaultPollInterval     = 500 * time.Millisecond
	defaultRecoveryInterval = 5 * time.Second
	defaultBaseRetryDelay   = 2 * time.Second
	defaultMaxRetryDelay    = 30 * time.Second
	lockContentionDelay     = 1 * time.Second
	blockedDBDelay          = 3 * time.Second
)

// WorkerConfig mengatur perilaku worker pool asynchronous.
type WorkerConfig struct {
	WorkerID         string
	Concurrency      int
	PollInterval     time.Duration
	RecoveryInterval time.Duration
	LockTTL          time.Duration
	BaseRetryDelay   time.Duration
	MaxRetryDelay    time.Duration
	Store            Store
	Locks            LockManager
	Executor         Executor
	History          HistoryRecorder
	Tracer           TracePropagator
	Metrics          MetricsObserver
	Logger           *slog.Logger
}

// Worker mengambil job dari queue persisten dan mengeksekusinya secara paralel
// untuk database berbeda serta serial untuk database yang sama.
type Worker struct {
	cfg WorkerConfig
}

func NewWorker(cfg WorkerConfig) *Worker {
	if cfg.WorkerID == "" {
		cfg.WorkerID = defaultWorkerID()
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = defaultConcurrency
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.RecoveryInterval <= 0 {
		cfg.RecoveryInterval = defaultRecoveryInterval
	}
	if cfg.LockTTL <= 0 {
		cfg.LockTTL = DefaultLockTTL
	}
	if cfg.BaseRetryDelay <= 0 {
		cfg.BaseRetryDelay = defaultBaseRetryDelay
	}
	if cfg.MaxRetryDelay <= 0 {
		cfg.MaxRetryDelay = defaultMaxRetryDelay
	}
	if cfg.History == nil {
		cfg.History = NoopHistoryRecorder{}
	}
	if cfg.Tracer == nil {
		cfg.Tracer = NoopTracePropagator{}
	}
	if cfg.Metrics == nil {
		cfg.Metrics = NoopMetricsObserver{}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Worker{cfg: cfg}
}

func defaultWorkerID() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "worker"
	}
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("%s-%s", host, hex.EncodeToString(buf))
}

// Start menjalankan loop recovery dan pool goroutine worker hingga ctx dibatalkan
// (graceful shutdown menunggu job yang sedang berjalan selesai).
func (w *Worker) Start(ctx context.Context) {
	var wg sync.WaitGroup

	// Loop pemulihan lock kedaluwarsa & job macet akibat worker crash.
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.recoveryLoop(ctx)
	}()

	// Pool goroutine worker: database berbeda dapat diproses paralel.
	for i := 0; i < w.cfg.Concurrency; i++ {
		workerName := fmt.Sprintf("%s-%d", w.cfg.WorkerID, i+1)
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			w.pollLoop(ctx, id)
		}(workerName)
	}

	wg.Wait()
}

func (w *Worker) recoveryLoop(ctx context.Context) {
	// Jalankan recovery sekali saat startup.
	w.runRecoveryOnce(ctx)

	ticker := time.NewTicker(w.cfg.RecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runRecoveryOnce(ctx)
		}
	}
}

func (w *Worker) runRecoveryOnce(ctx context.Context) {
	recCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if w.cfg.Locks != nil {
		expired, err := w.cfg.Locks.RecoverExpired(recCtx)
		if err != nil {
			w.cfg.Logger.ErrorContext(recCtx, "gagal memulihkan lock kedaluwarsa", "error", err)
		} else if len(expired) > 0 {
			w.cfg.Logger.WarnContext(recCtx, "lock kedaluwarsa dipulihkan", "count", len(expired))
		}
	}

	if w.cfg.Store != nil {
		recovered, err := w.cfg.Store.RecoverStaleRunningJobs(recCtx)
		if err != nil {
			w.cfg.Logger.ErrorContext(recCtx, "gagal memulihkan job running macet", "error", err)
		} else if recovered > 0 {
			w.cfg.Logger.WarnContext(recCtx, "job running macet dipulihkan", "count", recovered)
		}
	}
}

func (w *Worker) pollLoop(ctx context.Context, workerID string) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		processed, err := w.ProcessOnce(ctx, workerID)
		if err != nil && !errors.Is(err, context.Canceled) {
			w.cfg.Logger.ErrorContext(ctx, "worker gagal mengambil job", "worker_id", workerID, "error", err)
		}
		if processed {
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessOnce mengambil dan memproses maksimal satu job dari queue.
// Mengembalikan true bila ada job yang diproses.
func (w *Worker) ProcessOnce(ctx context.Context, workerID string) (bool, error) {
	if workerID == "" {
		workerID = w.cfg.WorkerID
	}

	job, ok, err := w.cfg.Store.DequeueNext(ctx)
	if err != nil || !ok {
		return false, err
	}

	jobCtx := w.cfg.Tracer.Extract(ctx, job.TraceContext)
	queueWait := time.Since(job.CreatedAt)
	if queueWait < 0 {
		queueWait = 0
	}

	allowBlocked := job.Action == ActionResume || job.Action == ActionCancelBatch
	lockStart := time.Now()
	_, lockErr := w.cfg.Locks.Acquire(jobCtx, job.ConnectionID, job.ID, workerID, w.cfg.LockTTL, allowBlocked, job.ParentJobID)
	w.cfg.Metrics.OnLockWait(time.Since(lockStart))

	if lockErr != nil {
		if errors.Is(lockErr, ErrDatabaseLocked) {
			// Database sedang dikunci worker lain; kembalikan ke queue tanpa memotong kuota retry.
			_ = w.cfg.Store.RequeueWithoutAttempt(ctx, job.ID, time.Now().UTC().Add(lockContentionDelay))
			return true, nil
		}
		if errors.Is(lockErr, ErrDatabaseBlocked) {
			// Tahan migration baru selama ada job partial / partially_reverted pada DB yang sama.
			_ = w.cfg.Store.RequeueWithoutAttempt(ctx, job.ID, time.Now().UTC().Add(blockedDBDelay))
			w.cfg.Logger.InfoContext(jobCtx, "job ditahan karena database memiliki migration partial",
				"job_id", job.ID, "connection_id", job.ConnectionID)
			return true, nil
		}
		errMsg := SanitizeError(lockErr)
		_ = w.cfg.Store.MarkFailed(ctx, job.ID, errMsg)
		w.cfg.Metrics.OnJobFinished(job.Mode, job.Action, StatusFailed, 0)
		return true, lockErr
	}

	stopHeartbeat := w.cfg.Locks.StartHeartbeat(jobCtx, job.ConnectionID, job.ID, workerID, w.cfg.LockTTL)
	defer func() {
		stopHeartbeat()
		relCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = w.cfg.Locks.Release(relCtx, job.ConnectionID, job.ID, workerID)
	}()

	startedAt := time.Now().UTC()
	w.cfg.Metrics.OnJobStarted(job.Mode, job.Action, queueWait)
	w.cfg.Logger.InfoContext(jobCtx, "memulai eksekusi job migration",
		"job_id", job.ID, "migration_id", job.MigrationID, "connection_id", job.ConnectionID,
		"action", job.Action, "mode", job.Mode, "attempt", job.Attempt, "worker_id", workerID)

	res := w.cfg.Executor.Execute(jobCtx, job)
	finishedAt := time.Now().UTC()
	duration := finishedAt.Sub(startedAt)

	w.finalizeJob(jobCtx, job, res, startedAt, finishedAt, duration)
	return true, nil
}

func (w *Worker) finalizeJob(
	ctx context.Context,
	job *Job,
	res ExecutionResult,
	startedAt, finishedAt time.Time,
	duration time.Duration,
) {
	saveCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	executedSQL := job.SQLUp
	if job.Action == ActionDown || job.Action == ActionCancelBatch {
		executedSQL = job.SQLDown
	}

	if res.Err == nil && (res.Status == StatusSuccess || res.Status == StatusReverted) {
		if res.Status == StatusReverted {
			_ = w.cfg.Store.MarkReverted(saveCtx, job.ID)
		} else {
			_ = w.cfg.Store.MarkSucceeded(saveCtx, job.ID)
		}
		_ = w.cfg.History.RecordAttempt(saveCtx, AttemptRecord{
			JobID:        job.ID,
			MigrationID:  job.MigrationID,
			ExecutionID:  job.ExecutionID,
			ConnectionID: job.ConnectionID,
			Action:       job.Action,
			Mode:         job.Mode,
			SQL:          executedSQL,
			Status:       res.Status,
			ExecutedBy:   job.CreatedBy,
			StartedAt:    startedAt,
			FinishedAt:   finishedAt,
		})
		w.cfg.Metrics.OnJobFinished(job.Mode, job.Action, res.Status, duration)
		w.cfg.Logger.InfoContext(ctx, "job migration selesai",
			"job_id", job.ID, "status", res.Status, "duration_ms", duration.Milliseconds())
		return
	}

	errMsg := SanitizeError(res.Err)

	// Retry otomatis hanya untuk kegagalan sementara selama attempt <= max_retries.
	if res.Transient && job.Attempt <= job.MaxRetries {
		delay := BackoffDelay(job.Attempt, w.cfg.BaseRetryDelay, w.cfg.MaxRetryDelay)
		nextRetry := time.Now().UTC().Add(delay)
		_ = w.cfg.Store.ScheduleRetry(saveCtx, job.ID, nextRetry, errMsg)
		w.cfg.Metrics.OnJobRetry(job.Mode, job.Action)
		w.cfg.Logger.WarnContext(ctx, "job migration gagal sementara, dijadwalkan retry",
			"job_id", job.ID, "attempt", job.Attempt, "max_retries", job.MaxRetries,
			"retry_in_ms", delay.Milliseconds(), "error", errMsg)
		return
	}

	finalStatus := res.Status
	switch finalStatus {
	case StatusPartial:
		_ = w.cfg.Store.MarkPartial(saveCtx, job.ID, errMsg)
	case StatusPartiallyReverted:
		_ = w.cfg.Store.MarkPartiallyReverted(saveCtx, job.ID, errMsg)
	default:
		finalStatus = StatusFailed
		_ = w.cfg.Store.MarkFailed(saveCtx, job.ID, errMsg)
	}

	_ = w.cfg.History.RecordAttempt(saveCtx, AttemptRecord{
		JobID:        job.ID,
		MigrationID:  job.MigrationID,
		ExecutionID:  job.ExecutionID,
		ConnectionID: job.ConnectionID,
		Action:       job.Action,
		Mode:         job.Mode,
		SQL:          executedSQL,
		Status:       finalStatus,
		Error:        errMsg,
		ExecutedBy:   job.CreatedBy,
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
	})
	w.cfg.Metrics.OnJobFinished(job.Mode, job.Action, finalStatus, duration)
	w.cfg.Logger.ErrorContext(ctx, "job migration berhenti dengan error",
		"job_id", job.ID, "status", finalStatus, "duration_ms", duration.Milliseconds(), "error", errMsg)
}
