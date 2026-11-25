package dbm

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
)

// executeBatch menjalankan migration per batch. Setiap batch committed dengan
// checkpoint-nya di target; progres disalin ke platform untuk tampilan.
//
// Hasil:
//   - semua batch selesai          → eksekusi success
//   - galat sementara, attempt ada → eksekusi failed, pengajuan kembali queued
//     (attempt berikutnya melanjutkan dari checkpoint)
//   - berhenti setelah ≥1 batch    → eksekusi partial, pengajuan partial;
//     database ditahan sampai resume atau rollback per batch
//   - gagal sebelum batch pertama  → eksekusi failed
func (x *Executor) executeBatch(ctx context.Context, job ExecJob, req requestRow, exec *startedExecution) error {
	ctx, span := activity.StartSpan(ctx, "migration.execute.batch")
	finish := x.metrics.StartMigration()
	start := time.Now()
	direction := BatchUp
	if exec.Action == ActionRollback {
		direction = BatchDown
	}
	base := func() map[string]string {
		return map[string]string{
			"job_id": idStr(job.JobID), "execution_id": idStr(exec.ID), "attempt": strconv.Itoa(job.Attempt),
			"batch_key": idStr(exec.BatchKey), "direction": direction, "batch_size": strconv.Itoa(exec.Batch.Size),
		}
	}
	startDetail := base()
	startDetail["sql"] = truncateForLog(exec.SQL)
	startDetail["version"] = strconv.FormatInt(exec.Version, 10)
	x.event(ctx, req, "migration.execution.start", "started", startDetail, nil)

	target, err := x.registry.Get(ctx, exec.ConnectionID)
	if err != nil {
		finish(false)
		activity.EndSpan(span, true)
		return x.closeBatch(ctx, job, req, exec, BatchResult{}, RetryableError{err}, time.Since(start), base())
	}
	if target.Dialect.Name() != "postgres" {
		finish(false)
		activity.EndSpan(span, true)
		return x.closeBatch(ctx, job, req, exec, BatchResult{},
			errors.New("mode batch hanya didukung untuk database target PostgreSQL"), time.Since(start), base())
	}

	run := BatchRun{
		DB: target.DB, Dialect: target.Dialect, Config: *exec.Batch, Key: exec.BatchKey,
		Direction: direction, SQL: exec.SQL, LockSQL: target.Dialect.LockSQL(exec.ConnectionID),
		Progress: func(cp BatchCheckpoint, totalBatches, totalRows int64) error {
			x.metrics.ObserveBatch(cp.Rows)
			if _, err := x.pool.Exec(ctx,
				`UPDATE migration_executions
				    SET committed_batches = $2, committed_rows = $3, total_batches = $4,
				        cursor_last = $5, rows_affected = $3
				  WHERE id = $1 AND status = 'running'`,
				exec.ID, cp.Number, totalRows, totalBatches, cp.CursorEnd); err != nil {
				return RetryableError{err}
			}
			if _, err := x.pool.Exec(ctx,
				`INSERT INTO migration_batch_checkpoints
				        (execution_id, batch_key, direction, batch_number, cursor_start, cursor_end, rows_affected)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)
				 ON CONFLICT (batch_key, direction, batch_number) DO NOTHING`,
				exec.ID, exec.BatchKey, direction, cp.Number, cp.CursorStart, cp.CursorEnd, cp.Rows); err != nil {
				return RetryableError{err}
			}
			d := base()
			d["batch"] = strconv.Itoa(cp.Number)
			d["total_batches"] = strconv.FormatInt(totalBatches, 10)
			d["rows"] = strconv.FormatInt(cp.Rows, 10)
			d["committed_rows"] = strconv.FormatInt(totalRows, 10)
			d["cursor_start"], d["cursor_end"] = cp.CursorStart, cp.CursorEnd
			x.event(ctx, req, "migration.batch.commit", activity.StatusSuccess, d, nil)
			return nil
		},
	}
	batchCtx, batchSpan := activity.StartSpan(ctx, "migration.batch.run")
	res, runErr := run.Run(batchCtx)
	activity.EndSpan(batchSpan, runErr != nil)
	finish(runErr == nil)
	activity.EndSpan(span, runErr != nil)
	return x.closeBatch(ctx, job, req, exec, res, runErr, time.Since(start), base())
}

// closeBatch menutup attempt batch sesuai hasilnya.
func (x *Executor) closeBatch(ctx context.Context, job ExecJob, req requestRow, exec *startedExecution,
	res BatchResult, runErr error, duration time.Duration, detail map[string]string) error {
	detail["duration_ms"] = strconv.FormatInt(duration.Milliseconds(), 10)
	detail["committed_batches"] = strconv.FormatInt(res.Batches, 10)
	detail["committed_rows"] = strconv.FormatInt(res.Rows, 10)
	rows := res.Rows
	// Context dibatalkan (worker berhenti): tetap catat hasil dengan context baru.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	if runErr == nil {
		if err := x.finalizeBatch(writeCtx, exec.ID, req.ID, ExecSuccess, "", "", "", duration, rows, res, RequestSuccess); err != nil {
			x.event(ctx, req, "migration.history.write_failed", activity.StatusFailed, detail, err)
			return RetryableError{err}
		}
		x.event(ctx, req, "migration.execution.commit", activity.StatusSuccess, detail, nil)
		return nil
	}

	code := sqlState(runErr)
	detail["sqlstate"] = code
	message := sanitizePGError(runErr)
	stopped := ctx.Err() != nil
	retry := stopped || ((isTransient(runErr) || IsRetryable(runErr)) && job.Attempt < job.MaxAttempts)

	execStatus, requestStatus, stage := ExecFailed, RequestFailed, "execution"
	switch {
	case retry:
		requestStatus = RequestQueued
		if stopped {
			stage = "platform"
			message = "worker berhenti di tengah rangkaian batch; dilanjutkan dari checkpoint terakhir"
		}
	case res.Committed():
		execStatus, requestStatus = ExecPartial, RequestPartial
	}
	if err := x.finalizeBatch(writeCtx, exec.ID, req.ID, execStatus, stage, message, code, duration, rows, res, requestStatus); err != nil {
		return RetryableError{err}
	}
	action := "migration.execution.rollback"
	if execStatus == ExecPartial {
		action = "migration.execution.partial"
	}
	x.event(ctx, req, action, activity.StatusFailed, detail, runErr)
	if retry {
		return RetryableError{runErr}
	}
	return nil
}

// finalizeBatch = finalize plus angka progres akhir.
func (x *Executor) finalizeBatch(ctx context.Context, execID, reqID int64, status, stage, message, code string,
	duration time.Duration, rows int64, res BatchResult, requestStatus string) error {
	if _, err := x.pool.Exec(ctx,
		`UPDATE migration_executions
		    SET committed_batches = $2, committed_rows = $3,
		        total_batches = GREATEST(COALESCE(total_batches, 0), $4),
		        cursor_last = COALESCE(NULLIF($5, ''), cursor_last)
		  WHERE id = $1 AND status = 'running'`,
		execID, res.Batches, res.Rows, res.TotalBatch, res.LastCursor); err != nil {
		return err
	}
	return x.finalize(ctx, execID, reqID, status, stage, message, code, duration, &rows, requestStatus)
}
