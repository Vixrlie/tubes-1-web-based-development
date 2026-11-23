package async

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnqueueInput adalah parameter untuk menambahkan job migration baru ke queue persisten.
type EnqueueInput struct {
	MigrationID    int64
	ExecutionID    *int64
	ConnectionID   int64
	Action         Action
	Mode           Mode
	SQLUp          string
	SQLDown        string
	Irreversible   bool
	BatchTable     string
	BatchColumn    string
	BatchSize      int
	CursorMin      string
	CursorMax      string
	LastCursor     string
	ParentJobID    *int64
	IdempotencyKey string
	MaxRetries     int
	TraceContext   map[string]string
	CreatedBy      *int64
}

// Store mendefinisikan operasi persistensi untuk queue job dan checkpoint batch.
type Store interface {
	Enqueue(ctx context.Context, in EnqueueInput) (Job, error)
	GetJob(ctx context.Context, jobID int64) (Job, error)
	ListJobsByConnection(ctx context.Context, connID int64, limit int) ([]Job, error)
	DequeueNext(ctx context.Context) (*Job, bool, error)
	MarkSucceeded(ctx context.Context, jobID int64) error
	MarkReverted(ctx context.Context, jobID int64) error
	MarkFailed(ctx context.Context, jobID int64, errMsg string) error
	MarkPartial(ctx context.Context, jobID int64, errMsg string) error
	MarkPartiallyReverted(ctx context.Context, jobID int64, errMsg string) error
	ScheduleRetry(ctx context.Context, jobID int64, nextRetryAt time.Time, errMsg string) error
	RequeueWithoutAttempt(ctx context.Context, jobID int64, nextRetryAt time.Time) error
	UpdateBatchBounds(ctx context.Context, jobID int64, cursorMin, cursorMax string) error
	RecordBatchProgress(ctx context.Context, jobID int64, lastCursor string, deltaBatches int, deltaRows int64) error
	SaveCheckpoint(ctx context.Context, cp Checkpoint) (Checkpoint, error)
	ListCheckpoints(ctx context.Context, jobID int64) ([]Checkpoint, error)
	ListCommittedCheckpointsDesc(ctx context.Context, jobID int64) ([]Checkpoint, error)
	MarkCheckpointReverted(ctx context.Context, checkpointID int64) error
	HasBlockingJob(ctx context.Context, connID int64, excludeJobID int64) (bool, error)
	RecoverStaleRunningJobs(ctx context.Context) (int64, error)
}

// PGStore adalah implementasi Store berbasis PostgreSQL (platform-db).
type PGStore struct {
	pool *pgxpool.Pool
}

var _ Store = (*PGStore)(nil)

func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

const jobColumns = `
	id, migration_id, execution_id, connection_id, action, mode,
	sql_up, COALESCE(sql_down, ''), irreversible,
	COALESCE(batch_table, ''), COALESCE(batch_column, ''), COALESCE(batch_size, 0),
	COALESCE(cursor_min, ''), COALESCE(cursor_max, ''), COALESCE(last_cursor, ''),
	committed_batches, committed_rows, status, parent_job_id,
	COALESCE(idempotency_key, ''), attempt, max_retries, next_retry_at,
	trace_context, COALESCE(error, ''), created_by, created_at, started_at, finished_at, updated_at`

func scanJob(row pgx.Row) (Job, error) {
	var (
		j          Job
		execID     sql.NullInt64
		parentID   sql.NullInt64
		createdBy  sql.NullInt64
		startedAt  sql.NullTime
		finishedAt sql.NullTime
		traceRaw   []byte
	)
	err := row.Scan(
		&j.ID, &j.MigrationID, &execID, &j.ConnectionID, &j.Action, &j.Mode,
		&j.SQLUp, &j.SQLDown, &j.Irreversible,
		&j.BatchTable, &j.BatchColumn, &j.BatchSize,
		&j.CursorMin, &j.CursorMax, &j.LastCursor,
		&j.CommittedBatches, &j.CommittedRows, &j.Status, &parentID,
		&j.IdempotencyKey, &j.Attempt, &j.MaxRetries, &j.NextRetryAt,
		&traceRaw, &j.Error, &createdBy, &j.CreatedAt, &startedAt, &finishedAt, &j.UpdatedAt,
	)
	if err != nil {
		return Job{}, err
	}
	if execID.Valid {
		j.ExecutionID = &execID.Int64
	}
	if parentID.Valid {
		j.ParentJobID = &parentID.Int64
	}
	if createdBy.Valid {
		j.CreatedBy = &createdBy.Int64
	}
	if startedAt.Valid {
		j.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		j.FinishedAt = &finishedAt.Time
	}
	j.TraceContext = map[string]string{}
	if len(traceRaw) > 0 {
		_ = json.Unmarshal(traceRaw, &j.TraceContext)
	}
	return j, nil
}

func nullableText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullableInt(n int) *int {
	if n <= 0 {
		return nil
	}
	return &n
}

func (s *PGStore) Enqueue(ctx context.Context, in EnqueueInput) (Job, error) {
	mode := in.Mode
	if mode == "" {
		mode = ModeNormal
	}
	action := in.Action
	if action == "" {
		action = ActionUp
	}
	maxRetries := in.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	traceJSON, err := json.Marshal(in.TraceContext)
	if err != nil || len(in.TraceContext) == 0 {
		traceJSON = []byte("{}")
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO migration_jobs (
			migration_id, execution_id, connection_id, action, mode,
			sql_up, sql_down, irreversible,
			batch_table, batch_column, batch_size,
			cursor_min, cursor_max, last_cursor,
			parent_job_id, idempotency_key, max_retries, trace_context, created_by
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11,
			$12, $13, $14,
			$15, $16, $17, $18, $19
		)
		RETURNING `+jobColumns,
		in.MigrationID, in.ExecutionID, in.ConnectionID, string(action), string(mode),
		in.SQLUp, nullableText(in.SQLDown), in.Irreversible,
		nullableText(in.BatchTable), nullableText(in.BatchColumn), nullableInt(in.BatchSize),
		nullableText(in.CursorMin), nullableText(in.CursorMax), nullableText(in.LastCursor),
		in.ParentJobID, nullableText(in.IdempotencyKey), maxRetries, traceJSON, in.CreatedBy,
	)

	job, err := scanJob(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && in.IdempotencyKey != "" {
			existing, getErr := s.getByIdempotencyKey(ctx, in.IdempotencyKey)
			if getErr == nil {
				return existing, ErrDuplicateJob
			}
		}
		return Job{}, err
	}
	return job, nil
}

func (s *PGStore) getByIdempotencyKey(ctx context.Context, key string) (Job, error) {
	j, err := scanJob(s.pool.QueryRow(ctx,
		`SELECT `+jobColumns+` FROM migration_jobs WHERE idempotency_key = $1`, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	return j, err
}

func (s *PGStore) GetJob(ctx context.Context, jobID int64) (Job, error) {
	j, err := scanJob(s.pool.QueryRow(ctx,
		`SELECT `+jobColumns+` FROM migration_jobs WHERE id = $1`, jobID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	return j, err
}

func (s *PGStore) ListJobsByConnection(ctx context.Context, connID int64, limit int) ([]Job, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+jobColumns+`
		   FROM migration_jobs
		  WHERE connection_id = $1
		  ORDER BY created_at DESC, id DESC
		  LIMIT $2`, connID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Job, 0, 16)
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// DequeueNext mengambil satu job pending yang siap dijalankan menggunakan
// FOR UPDATE SKIP LOCKED dan melewati database yang sedang memiliki lock aktif.
func (s *PGStore) DequeueNext(ctx context.Context) (*Job, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var jobID int64
	err = tx.QueryRow(ctx, `
		SELECT j.id
		  FROM migration_jobs j
		 WHERE j.status = 'pending'
		   AND j.next_retry_at <= now()
		   AND NOT EXISTS (
		       SELECT 1
		         FROM migration_locks l
		        WHERE l.connection_id = j.connection_id
		          AND l.lease_expires_at > now()
		   )
		 ORDER BY j.next_retry_at ASC, j.id ASC
		 FOR UPDATE OF j SKIP LOCKED
		 LIMIT 1`).Scan(&jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	j, err := scanJob(tx.QueryRow(ctx, `
		UPDATE migration_jobs
		   SET status = 'running',
		       attempt = attempt + 1,
		       started_at = COALESCE(started_at, now()),
		       updated_at = now()
		 WHERE id = $1
		 RETURNING `+jobColumns, jobID))
	if err != nil {
		return nil, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return &j, true, nil
}

func (s *PGStore) MarkSucceeded(ctx context.Context, jobID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE migration_jobs
		   SET status = 'success',
		       error = NULL,
		       finished_at = now(),
		       updated_at = now()
		 WHERE id = $1`, jobID)
	return err
}

func (s *PGStore) MarkReverted(ctx context.Context, jobID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE migration_jobs
		   SET status = 'reverted',
		       error = NULL,
		       finished_at = now(),
		       updated_at = now()
		 WHERE id = $1`, jobID)
	return err
}

func (s *PGStore) MarkFailed(ctx context.Context, jobID int64, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE migration_jobs
		   SET status = 'failed',
		       error = $2,
		       finished_at = now(),
		       updated_at = now()
		 WHERE id = $1`, jobID, errMsg)
	return err
}

func (s *PGStore) MarkPartial(ctx context.Context, jobID int64, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE migration_jobs
		   SET status = 'partial',
		       error = $2,
		       finished_at = now(),
		       updated_at = now()
		 WHERE id = $1`, jobID, errMsg)
	return err
}

func (s *PGStore) MarkPartiallyReverted(ctx context.Context, jobID int64, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE migration_jobs
		   SET status = 'partially_reverted',
		       error = $2,
		       finished_at = now(),
		       updated_at = now()
		 WHERE id = $1`, jobID, errMsg)
	return err
}

func (s *PGStore) ScheduleRetry(ctx context.Context, jobID int64, nextRetryAt time.Time, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE migration_jobs
		   SET status = 'pending',
		       next_retry_at = $2,
		       error = $3,
		       updated_at = now()
		 WHERE id = $1`, jobID, nextRetryAt, errMsg)
	return err
}

func (s *PGStore) RequeueWithoutAttempt(ctx context.Context, jobID int64, nextRetryAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE migration_jobs
		   SET status = 'pending',
		       attempt = GREATEST(attempt - 1, 0),
		       next_retry_at = $2,
		       updated_at = now()
		 WHERE id = $1`, jobID, nextRetryAt)
	return err
}

func (s *PGStore) UpdateBatchBounds(ctx context.Context, jobID int64, cursorMin, cursorMax string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE migration_jobs
		   SET cursor_min = $2,
		       cursor_max = $3,
		       updated_at = now()
		 WHERE id = $1`, jobID, nullableText(cursorMin), nullableText(cursorMax))
	return err
}

func (s *PGStore) RecordBatchProgress(ctx context.Context, jobID int64, lastCursor string, deltaBatches int, deltaRows int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE migration_jobs
		   SET last_cursor = $2,
		       committed_batches = committed_batches + $3,
		       committed_rows = committed_rows + $4,
		       updated_at = now()
		 WHERE id = $1`, jobID, lastCursor, deltaBatches, deltaRows)
	return err
}

func (s *PGStore) SaveCheckpoint(ctx context.Context, cp Checkpoint) (Checkpoint, error) {
	status := cp.Status
	if status == "" {
		status = CheckpointCommitted
	}
	var recovery any
	if len(cp.RecoveryData) > 0 {
		recovery = cp.RecoveryData
	}

	var out Checkpoint
	var revertedAt sql.NullTime
	var recBytes []byte
	err := s.pool.QueryRow(ctx, `
		INSERT INTO migration_batch_checkpoints (
			job_id, batch_number, cursor_start, cursor_end, rows_affected, status, recovery_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (job_id, batch_number) DO UPDATE
		   SET rows_affected = migration_batch_checkpoints.rows_affected
		RETURNING id, job_id, batch_number, cursor_start, cursor_end, rows_affected,
		          status, recovery_data, committed_at, reverted_at`,
		cp.JobID, cp.BatchNumber, cp.CursorStart, cp.CursorEnd, cp.RowsAffected, string(status), recovery,
	).Scan(
		&out.ID, &out.JobID, &out.BatchNumber, &out.CursorStart, &out.CursorEnd,
		&out.RowsAffected, &out.Status, &recBytes, &out.CommittedAt, &revertedAt,
	)
	if err != nil {
		return Checkpoint{}, err
	}
	out.RecoveryData = recBytes
	if revertedAt.Valid {
		out.RevertedAt = &revertedAt.Time
	}
	return out, nil
}

func (s *PGStore) ListCheckpoints(ctx context.Context, jobID int64) ([]Checkpoint, error) {
	return s.queryCheckpoints(ctx, `
		SELECT id, job_id, batch_number, cursor_start, cursor_end, rows_affected,
		       status, recovery_data, committed_at, reverted_at
		  FROM migration_batch_checkpoints
		 WHERE job_id = $1
		 ORDER BY batch_number ASC`, jobID)
}

func (s *PGStore) ListCommittedCheckpointsDesc(ctx context.Context, jobID int64) ([]Checkpoint, error) {
	return s.queryCheckpoints(ctx, `
		SELECT id, job_id, batch_number, cursor_start, cursor_end, rows_affected,
		       status, recovery_data, committed_at, reverted_at
		  FROM migration_batch_checkpoints
		 WHERE job_id = $1 AND status = 'committed'
		 ORDER BY batch_number DESC`, jobID)
}

func (s *PGStore) queryCheckpoints(ctx context.Context, query string, jobID int64) ([]Checkpoint, error) {
	rows, err := s.pool.Query(ctx, query, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Checkpoint, 0, 16)
	for rows.Next() {
		var cp Checkpoint
		var revertedAt sql.NullTime
		var recBytes []byte
		if err := rows.Scan(
			&cp.ID, &cp.JobID, &cp.BatchNumber, &cp.CursorStart, &cp.CursorEnd,
			&cp.RowsAffected, &cp.Status, &recBytes, &cp.CommittedAt, &revertedAt,
		); err != nil {
			return nil, err
		}
		cp.RecoveryData = recBytes
		if revertedAt.Valid {
			cp.RevertedAt = &revertedAt.Time
		}
		out = append(out, cp)
	}
	return out, rows.Err()
}

func (s *PGStore) MarkCheckpointReverted(ctx context.Context, checkpointID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE migration_batch_checkpoints
		   SET status = 'reverted',
		       reverted_at = now()
		 WHERE id = $1`, checkpointID)
	return err
}

// HasBlockingJob memeriksa apakah database target memiliki migration partial,
// partially_reverted, atau running lain yang harus diselesaikan sebelum migration baru.
func (s *PGStore) HasBlockingJob(ctx context.Context, connID int64, excludeJobID int64) (bool, error) {
	var blocked bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM migration_jobs
			 WHERE connection_id = $1
			   AND id <> $2
			   AND status IN ('partial', 'partially_reverted')
		)`, connID, excludeJobID).Scan(&blocked)
	return blocked, err
}

// RecoverStaleRunningJobs mengembalikan job yang macet di status 'running'
// akibat worker crash (lease lock sudah kedaluwarsa atau hilang).
// Job batch yang sudah punya checkpoint ditandai 'partial' agar dapat diperiksa/dilanjutkan;
// job lain dikembalikan ke 'pending' untuk di-retry dengan pengecekan bukti eksekusi.
func (s *PGStore) RecoverStaleRunningJobs(ctx context.Context) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tagPartial, err := tx.Exec(ctx, `
		UPDATE migration_jobs j
		   SET status = CASE WHEN j.action = 'cancel_batch' THEN 'partially_reverted' ELSE 'partial' END,
		       error = COALESCE(j.error, 'worker terhenti saat eksekusi batch; lanjutkan atau batalkan dari checkpoint'),
		       finished_at = now(),
		       updated_at = now()
		 WHERE j.status = 'running'
		   AND j.mode = 'batch'
		   AND j.committed_batches > 0
		   AND NOT EXISTS (
		       SELECT 1 FROM migration_locks l
		        WHERE l.connection_id = j.connection_id
		          AND l.job_id = j.id
		          AND l.lease_expires_at > now()
		   )`)
	if err != nil {
		return 0, err
	}

	tagPending, err := tx.Exec(ctx, `
		UPDATE migration_jobs j
		   SET status = 'pending',
		       next_retry_at = now(),
		       error = COALESCE(j.error, 'worker terhenti sebelum job selesai; dijadwalkan ulang'),
		       updated_at = now()
		 WHERE j.status = 'running'
		   AND NOT EXISTS (
		       SELECT 1 FROM migration_locks l
		        WHERE l.connection_id = j.connection_id
		          AND l.job_id = j.id
		          AND l.lease_expires_at > now()
		   )`)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return tagPartial.RowsAffected() + tagPending.RowsAffected(), nil
}

// PGHistoryRecorder memperbarui tabel migration_history bawaan platform sebagai
// implementasi default HistoryRecorder.
type PGHistoryRecorder struct {
	pool *pgxpool.Pool
}

var _ HistoryRecorder = (*PGHistoryRecorder)(nil)

func NewPGHistoryRecorder(pool *pgxpool.Pool) *PGHistoryRecorder {
	return &PGHistoryRecorder{pool: pool}
}

func (r *PGHistoryRecorder) RecordAttempt(ctx context.Context, rec AttemptRecord) error {
	if r == nil || r.pool == nil {
		return nil
	}
	historyStatus := "failed"
	switch rec.Status {
	case StatusSuccess, StatusReverted:
		historyStatus = "applied"
	case StatusFailed, StatusPartial, StatusPartiallyReverted:
		historyStatus = "failed"
	default:
		return nil
	}
	var errVal *string
	if rec.Error != "" {
		errVal = &rec.Error
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE migration_history
		   SET status = $2,
		       error = $3,
		       executed_by = COALESCE($4, executed_by),
		       executed_at = $5
		 WHERE id = $1`,
		rec.MigrationID, historyStatus, errVal, rec.ExecutedBy, rec.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("record attempt migration_history: %w", err)
	}
	return nil
}
