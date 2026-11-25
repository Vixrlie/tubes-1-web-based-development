package dbm

// Executor menjalankan satu job: lock → revalidasi → eksekusi → catat hasil.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
)

var (
	// ErrDatabaseBusy: database target sedang dikunci migration lain. Queue
	// menjadwalkan ulang job tanpa menghitungnya sebagai attempt.
	ErrDatabaseBusy = errors.New("database target sedang menjalankan migration lain")
	// ErrOutcomeUncertain: hasil commit di target belum dapat dipastikan.
	ErrOutcomeUncertain = errors.New("hasil commit di database target belum pasti")
)

// RetryableError menandai kegagalan sementara (mis. koneksi putus) yang boleh
// dicoba lagi oleh queue dengan jeda.
type RetryableError struct{ Err error }

func (e RetryableError) Error() string { return e.Err.Error() }
func (e RetryableError) Unwrap() error { return e.Err }

// IsRetryable true bila err (atau bungkusannya) adalah RetryableError.
func IsRetryable(err error) bool {
	var r RetryableError
	return errors.As(err, &r)
}

// ExecJob = job yang diambil worker dari queue.
type ExecJob struct {
	JobID       int64
	RequestID   int64
	Attempt     int
	MaxAttempts int
}

// executionLockKey: advisory lock berbasis sesi, otomatis lepas bila worker mati.
const executionLockKey = 7102

type Executor struct {
	pool     *pgxpool.Pool
	registry Registry
	logger   *activity.Logger
	metrics  *activity.Metrics
}

func NewExecutor(pool *pgxpool.Pool, registry Registry, logger *activity.Logger, metrics *activity.Metrics) *Executor {
	if registry == nil {
		registry = StubRegistry{}
	}
	return &Executor{pool: pool, registry: registry, logger: logger, metrics: metrics}
}

// requestRow = kolom pengajuan yang dibutuhkan executor.
type requestRow struct {
	ID, MigrationID, ConnectionID int64
	Action, Status, Checksum      string
	TargetExecution               *int64
	RequestedBy                   *int64
}

func (x *Executor) loadRequest(ctx context.Context, q pgxQuerier, reqID int64, forUpdate bool) (requestRow, error) {
	query := `SELECT id, migration_id, connection_id, action, status, checksum, target_execution_id, requested_by
	            FROM migration_requests WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var r requestRow
	err := q.QueryRow(ctx, query, reqID).Scan(&r.ID, &r.MigrationID, &r.ConnectionID, &r.Action,
		&r.Status, &r.Checksum, &r.TargetExecution, &r.RequestedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return requestRow{}, ErrRequestNotFound
	}
	return r, err
}

func (x *Executor) event(ctx context.Context, req requestRow, action, status string, detail map[string]string, err error) {
	if detail == nil {
		detail = map[string]string{}
	}
	detail["request_id"] = idStr(req.ID)
	detail["migration_id"] = idStr(req.MigrationID)
	detail["database_id"] = idStr(req.ConnectionID)
	detail["migration_action"] = req.Action
	actor := "worker"
	if req.RequestedBy != nil {
		actor = idStr(*req.RequestedBy)
	}
	x.logger.Log(ctx, activity.Event{UserID: actor, Action: action, Status: status, Detail: detail, Err: err})
}

// Run: nil = selesai (termasuk gagal yang sudah tercatat). Job duplikat
// diabaikan karena hanya pengajuan queued yang dieksekusi.
func (x *Executor) Run(ctx context.Context, job ExecJob) (err error) {
	ctx, span := activity.StartSpan(ctx, "migration.worker.run")
	defer func() { activity.EndSpan(span, err != nil) }()

	req, err := x.loadRequest(ctx, x.pool, job.RequestID, false)
	if err != nil {
		return err
	}
	ctx = activity.WithMigrationID(ctx, req.MigrationID)
	if req.RequestedBy != nil {
		ctx = activity.WithActor(ctx, *req.RequestedBy)
	}
	if req.Status != RequestQueued && req.Status != RequestRunning {
		x.event(ctx, req, "migration.job.duplicate", activity.StatusWarning, map[string]string{
			"job_id": idStr(job.JobID), "request_status": req.Status,
		}, nil)
		return nil
	}

	// Lock lintas worker: satu database hanya menjalankan satu migration.
	lockCtx, lockSpan := activity.StartSpan(ctx, "migration.lock")
	lockStart := time.Now()
	conn, err := x.pool.Acquire(lockCtx)
	if err != nil {
		activity.EndSpan(lockSpan, true)
		return RetryableError{err}
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(lockCtx, `SELECT pg_try_advisory_lock($1::int, $2::int)`,
		executionLockKey, req.ConnectionID).Scan(&locked); err != nil {
		activity.EndSpan(lockSpan, true)
		return RetryableError{err}
	}
	x.metrics.ObserveLock(time.Since(lockStart))
	activity.EndSpan(lockSpan, !locked)
	if !locked {
		x.event(ctx, req, "migration.lock.busy", activity.StatusWarning, map[string]string{"job_id": idStr(job.JobID)}, nil)
		return ErrDatabaseBusy
	}
	defer func() {
		// Context terpisah agar unlock tetap jalan walau ctx dibatalkan.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1::int, $2::int)`,
			executionLockKey, req.ConnectionID); err != nil {
			// Lock tetap lepas saat koneksi ditutup; buang koneksinya.
			_ = conn.Conn().Close(unlockCtx)
		}
	}()

	if req.Status == RequestRunning {
		resolved, err := x.recoverRunning(ctx, req)
		if err != nil || resolved {
			return err
		}
	}

	exec, err := x.start(ctx, job)
	if err != nil || exec == nil {
		return err
	}
	return x.execute(ctx, job, req, exec)
}

// startedExecution = eksekusi yang sudah lolos revalidasi dan tercatat running.
type startedExecution struct {
	ID           int64
	SQL          string
	Action       string
	ConnectionID int64
	Version      int64
	// Diisi untuk migration per batch.
	Batch    *BatchConfig
	BatchKey int64
}

// start: kegagalan revalidasi dicatat sebagai attempt gagal tanpa menyentuh target.
func (x *Executor) start(ctx context.Context, job ExecJob) (*startedExecution, error) {
	ctx, span := activity.StartSpan(ctx, "migration.revalidate")
	defer span.End()

	tx, err := x.pool.Begin(ctx)
	if err != nil {
		return nil, RetryableError{err}
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	req, err := x.loadRequest(ctx, tx, job.RequestID, true)
	if err != nil {
		return nil, err
	}
	if err := lockConnection(ctx, tx, req.ConnectionID); err != nil {
		return nil, RetryableError{err}
	}
	if req.Status != RequestQueued {
		x.event(ctx, req, "migration.job.duplicate", activity.StatusWarning, map[string]string{
			"job_id": idStr(job.JobID), "request_status": req.Status,
		}, nil)
		return nil, nil
	}
	// Tahan eksekusi berikutnya selama database masih punya eksekusi yang
	// hasilnya belum pasti (pengajuan lain tertinggal running).
	var held bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM migration_requests
		                 WHERE connection_id = $1 AND status = 'running' AND id <> $2)`,
		req.ConnectionID, req.ID).Scan(&held); err != nil {
		return nil, RetryableError{err}
	}
	if held {
		x.event(ctx, req, "migration.held", activity.StatusWarning, map[string]string{"job_id": idStr(job.JobID)}, nil)
		return nil, ErrDatabaseBusy
	}
	m, err := scanMigration(tx.QueryRow(ctx, `SELECT `+migrationColumns+migrationJoins+` WHERE m.id = $1`, req.MigrationID))
	if err != nil {
		return nil, err
	}
	// Tahan migration lain selama database punya migration batch yang berhenti
	// sebagian; hanya resume/rollback migration itu yang boleh berjalan.
	states, err := loadStates(ctx, tx, req.ConnectionID)
	if err != nil {
		return nil, RetryableError{err}
	}
	for _, st := range states {
		if st.State() == StatePartial && st.MigrationID != m.ID {
			x.event(ctx, req, "migration.held", activity.StatusWarning, map[string]string{
				"job_id": idStr(job.JobID), "partial_migration_id": idStr(st.MigrationID),
			}, nil)
			return nil, ErrDatabaseBusy
		}
	}

	sqlText := m.UpSQL
	if req.Action == ActionRollback {
		sqlText = m.DownSQL
	}

	reason := x.revalidate(ctx, tx, req, m)
	// Kunci rangkaian batch: up/forward memulai rangkaian baru (ID pengajuan),
	// resume dan rollback melanjutkan rangkaian milik eksekusi target.
	var batchKey, batchSize any
	var key int64
	if m.Batch.Enabled() && reason == nil {
		key = req.ID
		if (req.Action == ActionResume || req.Action == ActionRollback) && req.TargetExecution != nil {
			var targetKey *int64
			if err := tx.QueryRow(ctx, `SELECT batch_key FROM migration_executions WHERE id = $1`,
				*req.TargetExecution).Scan(&targetKey); err != nil {
				return nil, err
			}
			if targetKey != nil {
				key = *targetKey
			}
		}
		batchKey, batchSize = key, m.Batch.Size
	}
	var revertsID any
	if req.Action == ActionRollback && req.TargetExecution != nil {
		revertsID = *req.TargetExecution
	}
	if reason != nil {
		var execID int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO migration_executions (request_id, migration_id, connection_id, action,
			        reverts_execution_id, migration_version, migration_revision, sql_executed,
			        status, error_stage, error, attempt, job_id, executed_by, finished_at, duration_ms)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'failed', 'revalidation', $9, $10, $11, $12, now(), 0)
			 RETURNING id`,
			req.ID, m.ID, m.ConnectionID, req.Action, revertsID, m.Version, m.Revision, sqlText,
			reason.Error(), job.Attempt, job.JobID, req.RequestedBy).Scan(&execID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE migration_requests SET status = 'failed', error = $2, finished_at = now(), updated_at = now()
			  WHERE id = $1`, req.ID, "revalidasi gagal: "+reason.Error()); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, RetryableError{err}
		}
		activity.EndSpan(span, true)
		x.event(ctx, req, "migration.revalidation", activity.StatusFailed, map[string]string{
			"job_id": idStr(job.JobID), "execution_id": idStr(execID), "attempt": strconv.Itoa(job.Attempt),
			"reason": reason.Error(),
		}, reason)
		return nil, nil
	}

	var execID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO migration_executions (request_id, migration_id, connection_id, action,
		        reverts_execution_id, migration_version, migration_revision, sql_executed,
		        status, attempt, job_id, executed_by, batch_key, batch_size)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'running', $9, $10, $11, $12, $13)
		 RETURNING id`,
		req.ID, m.ID, m.ConnectionID, req.Action, revertsID, m.Version, m.Revision, sqlText,
		job.Attempt, job.JobID, req.RequestedBy, batchKey, batchSize).Scan(&execID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE migrations SET locked_at = now() WHERE id = $1 AND locked_at IS NULL`, m.ID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE migration_requests SET status = 'running', started_at = COALESCE(started_at, now()), updated_at = now()
		  WHERE id = $1`, req.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, RetryableError{err}
	}
	x.event(ctx, req, "migration.revalidation", activity.StatusSuccess, map[string]string{
		"job_id": idStr(job.JobID), "execution_id": idStr(execID), "attempt": strconv.Itoa(job.Attempt),
	}, nil)
	started := &startedExecution{ID: execID, SQL: sqlText, Action: req.Action, ConnectionID: m.ConnectionID, Version: m.Version}
	if m.Batch.Enabled() {
		started.Batch, started.BatchKey = m.Batch, key
	}
	return started, nil
}

// revalidate memeriksa checksum, approval, dan urutan yang bisa berubah sejak approval.
func (x *Executor) revalidate(ctx context.Context, tx pgx.Tx, req requestRow, m Migration) error {
	if m.Checksum != req.Checksum {
		return ErrChecksumMismatch
	}
	reviewers, err := loadReviewers(ctx, tx, req.ConnectionID, req.ID)
	if err != nil {
		return err
	}
	if ReviewOutcome(reviewers) != ReviewApproved {
		return errors.New("approval tidak lagi lengkap untuk seluruh group yang memiliki akses")
	}
	states, err := loadStates(ctx, tx, req.ConnectionID)
	if err != nil {
		return err
	}
	plan, err := PlanAction(states, m.ID, req.Action)
	if err != nil {
		return err
	}
	if req.Action == ActionRollback || req.Action == ActionResume {
		if req.TargetExecution == nil || plan.TargetExecution != *req.TargetExecution {
			return errors.New("eksekusi target sudah bukan hasil terakhir migration ini")
		}
	}
	return nil
}

// execute menjalankan SQL di target lalu mencatat hasil akhirnya.
func (x *Executor) execute(ctx context.Context, job ExecJob, req requestRow, exec *startedExecution) error {
	if exec.Batch != nil {
		return x.executeBatch(ctx, job, req, exec)
	}
	ctx, span := activity.StartSpan(ctx, "migration.execute")
	finish := x.metrics.StartMigration()
	start := time.Now()
	x.event(ctx, req, "migration.execution.start", "started", map[string]string{
		"job_id": idStr(job.JobID), "execution_id": idStr(exec.ID), "attempt": strconv.Itoa(job.Attempt),
		"version": strconv.FormatInt(exec.Version, 10), "sql": truncateForLog(exec.SQL),
	}, nil)

	rows, runErr := x.runOnTarget(ctx, exec)
	duration := time.Since(start)
	detail := map[string]string{
		"job_id": idStr(job.JobID), "execution_id": idStr(exec.ID), "attempt": strconv.Itoa(job.Attempt),
		"duration_ms": strconv.FormatInt(duration.Milliseconds(), 10), "sql": truncateForLog(exec.SQL),
	}
	if rows != nil {
		detail["rows"] = strconv.FormatInt(*rows, 10)
	}

	if errors.Is(runErr, ErrOutcomeUncertain) {
		// Biarkan running; percobaan berikutnya memeriksa bukti txid lebih dulu.
		finish(false)
		activity.EndSpan(span, true)
		x.event(ctx, req, "migration.execution.uncertain", activity.StatusWarning, detail, runErr)
		return RetryableError{runErr}
	}

	if runErr == nil {
		err := x.finalize(ctx, exec.ID, req.ID, ExecSuccess, "", "", "", duration, rows, RequestSuccess)
		finish(err == nil)
		activity.EndSpan(span, err != nil)
		if err != nil {
			// Target sudah commit; recoverRunning akan menyelesaikan pencatatan.
			x.event(ctx, req, "migration.history.write_failed", activity.StatusFailed, detail, err)
			return RetryableError{err}
		}
		x.event(ctx, req, "migration.execution.commit", activity.StatusSuccess, detail, nil)
		return nil
	}

	finish(false)
	activity.EndSpan(span, true)
	code := sqlState(runErr)
	detail["sqlstate"] = code
	message := sanitizePGError(runErr)
	transient := isTransient(runErr)
	requestStatus := RequestFailed
	if transient && job.Attempt < job.MaxAttempts {
		requestStatus = RequestQueued
	}
	if err := x.finalize(ctx, exec.ID, req.ID, ExecFailed, "execution", message, code, duration, rows, requestStatus); err != nil {
		return RetryableError{err}
	}
	x.event(ctx, req, "migration.execution.rollback", activity.StatusFailed, detail, runErr)
	if requestStatus == RequestQueued {
		return RetryableError{runErr}
	}
	return nil
}

func truncateForLog(s string) string {
	const max = 4000
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// isTransient: galat SQL dari server (PgError) bersifat permanen; galat
// koneksi/jaringan dianggap sementara.
func isTransient(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// 08xxx connection exception, 40001 serialization, 40P01 deadlock, 57P0x shutdown.
		switch {
		case len(pgErr.Code) >= 2 && pgErr.Code[:2] == "08",
			pgErr.Code == "40001", pgErr.Code == "40P01",
			pgErr.Code == "57P01", pgErr.Code == "57P02", pgErr.Code == "57P03":
			return true
		}
		return false
	}
	return !errors.Is(err, context.Canceled)
}

// finalize menutup attempt (running → success/failed) dan memperbarui
// status pengajuan dalam satu transaksi platform.
func (x *Executor) finalize(ctx context.Context, execID, reqID int64, status, stage, message, code string,
	duration time.Duration, rows *int64, requestStatus string) error {
	tx, err := x.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx,
		`UPDATE migration_executions
		    SET status = $2, error_stage = $3, error = $4, sqlstate = $5,
		        finished_at = now(), duration_ms = $6, rows_affected = $7
		  WHERE id = $1 AND status = 'running'`,
		execID, status, nullable(stage), nullable(message), nullable(code), duration.Milliseconds(), rows); err != nil {
		return err
	}
	switch requestStatus {
	case RequestQueued:
		_, err = tx.Exec(ctx,
			`UPDATE migration_requests SET status = 'queued', error = $2, updated_at = now() WHERE id = $1`,
			reqID, nullable(message))
	default:
		_, err = tx.Exec(ctx,
			`UPDATE migration_requests SET status = $2, error = $3, finished_at = now(), updated_at = now()
			  WHERE id = $1`, reqID, requestStatus, nullable(message))
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// runOnTarget mencatat txid sebelum SQL jalan, sebagai bukti commit bila worker mati.
func (x *Executor) runOnTarget(ctx context.Context, exec *startedExecution) (*int64, error) {
	target, err := x.registry.Get(ctx, exec.ConnectionID)
	if err != nil {
		return nil, RetryableError{err}
	}
	conn, err := target.DB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	if lock := target.Dialect.LockSQL(exec.ConnectionID); lock != "" {
		lockCtx, span := activity.StartSpan(ctx, "migration.target_lock")
		_, err := tx.ExecContext(lockCtx, lock)
		activity.EndSpan(span, err != nil)
		if err != nil {
			return nil, err
		}
	}
	if target.Dialect.Name() == "postgres" {
		var txid int64
		if err := tx.QueryRowContext(ctx, `SELECT txid_current()`).Scan(&txid); err != nil {
			return nil, err
		}
		if _, err := x.pool.Exec(ctx,
			`UPDATE migration_executions SET target_txid = $2 WHERE id = $1`, exec.ID, txid); err != nil {
			return nil, RetryableError{err}
		}
	}

	sqlCtx, span := activity.StartSpan(ctx, "migration.sql")
	res, err := tx.ExecContext(sqlCtx, exec.SQL)
	activity.EndSpan(span, err != nil)
	if err != nil {
		return nil, err
	}
	var rows *int64
	if n, err := res.RowsAffected(); err == nil {
		rows = &n
	}

	if err := tx.Commit(); err != nil {
		// Commit gagal: bisa jadi sudah commit di server. Periksa buktinya.
		if target.Dialect.Name() == "postgres" {
			status, checkErr := x.targetTxStatus(ctx, exec.ConnectionID, exec.ID)
			switch {
			case checkErr == nil && status == "committed":
				return rows, nil
			case checkErr == nil && status == "aborted":
				return nil, err
			}
		}
		return nil, fmt.Errorf("%w: %v", ErrOutcomeUncertain, err)
	}
	if unlock := target.Dialect.UnlockSQL(exec.ConnectionID); unlock != "" {
		_, _ = conn.ExecContext(ctx, unlock)
	}
	return rows, nil
}

// targetTxStatus membaca txid_status() di target untuk eksekusi execID.
// Hasil: committed | aborted | in progress | "" (tidak diketahui).
func (x *Executor) targetTxStatus(ctx context.Context, connID, execID int64) (string, error) {
	var txid *int64
	if err := x.pool.QueryRow(ctx, `SELECT target_txid FROM migration_executions WHERE id = $1`, execID).Scan(&txid); err != nil {
		return "", err
	}
	if txid == nil {
		return "not_started", nil
	}
	target, err := x.registry.Get(ctx, connID)
	if err != nil {
		return "", err
	}
	var status sql.NullString
	if err := target.DB.QueryRowContext(ctx, `SELECT txid_status($1)`, *txid).Scan(&status); err != nil {
		return "", err
	}
	return status.String, nil
}

// recoverRunning memeriksa txid attempt yang menggantung sebelum retry, agar
// tidak terjadi double commit. resolved=true berarti pengajuan sudah final.
func (x *Executor) recoverRunning(ctx context.Context, req requestRow) (resolved bool, err error) {
	ctx, span := activity.StartSpan(ctx, "migration.recover")
	defer func() { activity.EndSpan(span, err != nil) }()

	var execID int64
	var started time.Time
	var batchKey *int64
	err = x.pool.QueryRow(ctx,
		`SELECT id, started_at, batch_key FROM migration_executions
		  WHERE request_id = $1 AND status = 'running' ORDER BY id DESC LIMIT 1`, req.ID).Scan(&execID, &started, &batchKey)
	if errors.Is(err, pgx.ErrNoRows) {
		// Tidak ada attempt menggantung: kembalikan ke antrian.
		_, err = x.pool.Exec(ctx, `UPDATE migration_requests SET status = 'queued', updated_at = now() WHERE id = $1`, req.ID)
		return false, err
	}
	if err != nil {
		return false, RetryableError{err}
	}

	if batchKey != nil {
		// Batch: checkpoint di target adalah bukti tiap batch, jadi attempt
		// berikutnya cukup melanjutkan rangkaian tanpa mengulang batch committed.
		msg := "worker berhenti di tengah rangkaian batch; dilanjutkan dari checkpoint terakhir"
		if err := x.finalize(ctx, execID, req.ID, ExecFailed, "platform", msg, "", time.Since(started), nil, RequestQueued); err != nil {
			return false, RetryableError{err}
		}
		x.event(ctx, req, "migration.recover.batch_resume", activity.StatusWarning,
			map[string]string{"execution_id": idStr(execID), "batch_key": idStr(*batchKey)}, nil)
		return false, nil
	}

	target, terr := x.registry.Get(ctx, req.ConnectionID)
	status := ""
	if terr == nil && target.Dialect.Name() == "postgres" {
		status, err = x.targetTxStatus(ctx, req.ConnectionID, execID)
		if err != nil {
			return false, RetryableError{fmt.Errorf("%w: %v", ErrOutcomeUncertain, err)}
		}
	} else if terr != nil {
		return false, RetryableError{terr}
	}

	detail := map[string]string{"execution_id": idStr(execID), "evidence": status}
	elapsed := time.Since(started)
	switch status {
	case "committed":
		if err := x.finalize(ctx, execID, req.ID, ExecSuccess, "", "", "", elapsed, nil, RequestSuccess); err != nil {
			return false, RetryableError{err}
		}
		x.event(ctx, req, "migration.recover.committed", activity.StatusSuccess, detail, nil)
		return true, nil
	case "aborted", "not_started":
		msg := "worker berhenti sebelum transaksi selesai; transaksi target tidak di-commit"
		if err := x.finalize(ctx, execID, req.ID, ExecFailed, "platform", msg, "", elapsed, nil, RequestQueued); err != nil {
			return false, RetryableError{err}
		}
		x.event(ctx, req, "migration.recover.requeue", activity.StatusWarning, detail, nil)
		return false, nil
	case "in progress":
		return false, ErrDatabaseBusy
	default:
		// MySQL atau txid sudah tidak tersedia: tahan, butuh pemeriksaan manual.
		msg := "hasil eksekusi tidak dapat dipastikan; periksa database target lalu ajukan forward/rollback baru"
		if err := x.finalize(ctx, execID, req.ID, ExecFailed, "platform", msg, "", elapsed, nil, RequestFailed); err != nil {
			return false, RetryableError{err}
		}
		x.event(ctx, req, "migration.recover.uncertain", activity.StatusFailed, detail, ErrOutcomeUncertain)
		return true, nil
	}
}

// Abandon tidak menyentuh pengajuan running: hasil di target belum pasti.
func (x *Executor) Abandon(ctx context.Context, requestID int64, cause error) error {
	msg := "job dihentikan setelah percobaan habis"
	if cause != nil {
		msg += ": " + sanitizePGError(cause)
	}
	_, err := x.pool.Exec(ctx,
		`UPDATE migration_requests SET status = 'failed', error = $2, finished_at = now(), updated_at = now()
		  WHERE id = $1 AND status IN ('queued', 'approved')`, requestID, msg)
	if err == nil {
		x.logger.Log(ctx, activity.Event{UserID: "worker", Action: "migration.job.abandoned", Status: activity.StatusFailed,
			Detail: map[string]string{"request_id": idStr(requestID)}, Err: cause})
	}
	return err
}

// QueueHandler menerjemahkan hasil Run ke semantik queue: busy dijadwalkan
// ulang tanpa menghabiskan attempt, galat sementara di-retry, nil selesai.
func (x *Executor) QueueHandler(busy, retry func(error) error) func(ctx context.Context, job ExecJob) error {
	return func(ctx context.Context, job ExecJob) error {
		err := x.Run(ctx, job)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, ErrDatabaseBusy):
			return busy(err)
		default:
			// Galat platform yang tak terklasifikasi juga dicoba lagi; setelah
			// attempt habis queue memanggil Abandon.
			return retry(err)
		}
	}
}
