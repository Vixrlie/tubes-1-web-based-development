package async

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/dbm"
)

const (
	maxErrorLength         = 2000
	targetCheckpointSchema = `
CREATE TABLE IF NOT EXISTS _pathgres_batch_checkpoints (
    job_id        BIGINT      NOT NULL,
    batch_number  INT         NOT NULL,
    direction     TEXT        NOT NULL,
    cursor_start  TEXT        NOT NULL,
    cursor_end    TEXT        NOT NULL,
    rows_affected BIGINT      NOT NULL DEFAULT 0,
    committed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, batch_number, direction)
)`
)

var (
	credentialPattern = regexp.MustCompile(`(?i)((?:postgres(?:ql)?|mysql)://[^:\s/@]+:)[^@\s]+(@)`)
	identPattern      = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// SanitizeError menghapus kredensial dari pesan error database dan memotong panjangnya.
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := credentialPattern.ReplaceAllString(err.Error(), "${1}***${2}")
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > maxErrorLength {
		msg = msg[:maxErrorLength] + "…"
	}
	return msg
}

// IsTransientError membedakan kegagalan sementara (koneksi, deadlock, serialization,
// timeout) yang layak di-retry dari error SQL permanen (syntax, constraint).
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", // serialization_failure
			"40P01", // deadlock_detected
			"55P03", // lock_not_available
			"57P01", // admin_shutdown
			"57P02", // crash_shutdown
			"57P03", // cannot_connect_now
			"08000", // connection_exception
			"08003", // connection_does_not_exist
			"08006", // connection_failure
			"08001", // sqlclient_unable_to_establish_sqlconnection
			"08004": // sqlserver_rejected_establishment_of_sqlconnection
			return true
		}
	}
	return false
}

// ExecutionResult merangkum hasil akhir eksekusi satu job oleh Executor.
type ExecutionResult struct {
	Status    Status
	Transient bool
	Err       error
}

// Executor menjalankan SQL migration (normal maupun batch) ke database target.
type Executor interface {
	Execute(ctx context.Context, job *Job) ExecutionResult
}

// TargetExecutor menjalankan migration terhadap database target melalui dbm.Registry.
type TargetExecutor struct {
	store    Store
	registry dbm.Registry
	metrics  MetricsObserver
}

var _ Executor = (*TargetExecutor)(nil)

func NewTargetExecutor(store Store, registry dbm.Registry, metrics MetricsObserver) *TargetExecutor {
	if metrics == nil {
		metrics = NoopMetricsObserver{}
	}
	return &TargetExecutor{
		store:    store,
		registry: registry,
		metrics:  metrics,
	}
}

func (e *TargetExecutor) Execute(ctx context.Context, job *Job) ExecutionResult {
	switch job.Action {
	case ActionCancelBatch:
		return e.executeCancelBatch(ctx, job)
	case ActionResume:
		return e.executeBatchUp(ctx, job, true)
	default:
		if job.Mode == ModeBatch {
			return e.executeBatchUp(ctx, job, false)
		}
		return e.executeNormal(ctx, job)
	}
}

// executeNormal menjalankan seluruh SQL dalam satu transaksi pada database target.
func (e *TargetExecutor) executeNormal(ctx context.Context, job *Job) ExecutionResult {
	sqlText := job.SQLUp
	if job.Action == ActionDown {
		sqlText = job.SQLDown
	}
	if strings.TrimSpace(sqlText) == "" {
		return ExecutionResult{Status: StatusFailed, Err: errors.New("SQL migration kosong")}
	}

	target, err := e.registry.Get(ctx, job.ConnectionID)
	if err != nil {
		return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
	}

	conn, err := target.DB.Conn(ctx)
	if err != nil {
		return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
	}
	defer conn.Close()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
	}
	if err := tx.Commit(); err != nil {
		return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
	}

	return ExecutionResult{Status: StatusSuccess}
}

func validateBatchIdentifiers(table, column string) ([]string, string, error) {
	table = strings.TrimSpace(table)
	column = strings.TrimSpace(column)
	if table == "" || column == "" {
		return nil, "", fmt.Errorf("%w: batch_table dan batch_column wajib diisi", ErrInvalidBatchInput)
	}
	parts := strings.Split(table, ".")
	if len(parts) > 2 {
		return nil, "", fmt.Errorf("%w: format batch_table tidak valid", ErrInvalidBatchInput)
	}
	for _, p := range parts {
		if !identPattern.MatchString(p) {
			return nil, "", fmt.Errorf("%w: nama tabel tidak valid", ErrInvalidBatchInput)
		}
	}
	if !identPattern.MatchString(column) {
		return nil, "", fmt.Errorf("%w: nama kolom cursor tidak valid", ErrInvalidBatchInput)
	}
	return parts, column, nil
}

func validateBatchSQL(sqlText string) error {
	if strings.TrimSpace(sqlText) == "" {
		return fmt.Errorf("%w: SQL batch kosong", ErrInvalidBatchInput)
	}
	if !strings.Contains(sqlText, "$1") || !strings.Contains(sqlText, "$2") {
		return fmt.Errorf("%w: SQL batch wajib menggunakan placeholder cursor $1 (start) dan $2 (end)", ErrInvalidBatchInput)
	}
	return nil
}

func ensureTargetCheckpointTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, targetCheckpointSchema)
	return err
}

func targetCheckpointExists(ctx context.Context, db *sql.DB, rootJobID int64, batchNum int, direction string) (bool, string, string, int64, error) {
	var start, end string
	var rows int64
	err := db.QueryRowContext(ctx, `
		SELECT cursor_start, cursor_end, rows_affected
		  FROM _pathgres_batch_checkpoints
		 WHERE job_id = $1 AND batch_number = $2 AND direction = $3`,
		rootJobID, batchNum, direction,
	).Scan(&start, &end, &rows)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", "", 0, nil
	}
	if err != nil {
		return false, "", "", 0, err
	}
	return true, start, end, rows, nil
}

// executeBatchUp menjalankan migration per batch berbasis cursor.
// Setiap batch memakai transaksi tersendiri dan menyimpan checkpoint di target DB
// dalam transaksi yang sama sebelum commit.
func (e *TargetExecutor) executeBatchUp(ctx context.Context, job *Job, isResume bool) ExecutionResult {
	tableParts, col, err := validateBatchIdentifiers(job.BatchTable, job.BatchColumn)
	if err != nil {
		return ExecutionResult{Status: StatusFailed, Err: err}
	}
	if job.BatchSize <= 0 {
		return ExecutionResult{Status: StatusFailed, Err: fmt.Errorf("%w: batch_size harus > 0", ErrInvalidBatchInput)}
	}
	if err := validateBatchSQL(job.SQLUp); err != nil {
		return ExecutionResult{Status: StatusFailed, Err: err}
	}

	target, err := e.registry.Get(ctx, job.ConnectionID)
	if err != nil {
		return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
	}
	if err := ensureTargetCheckpointTable(ctx, target.DB); err != nil {
		return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
	}

	quotedTable := target.Dialect.QuoteIdent(tableParts...)
	quotedCol := target.Dialect.QuoteIdent(col)

	// rootJobID mengidentifikasi rangkaian checkpoint untuk job ini (atau parent job saat resume).
	rootJobID := job.ID
	if isResume && job.ParentJobID != nil {
		rootJobID = *job.ParentJobID
	}

	// Tentukan cakupan target awal (cursor_min, cursor_max) bila belum tersimpan.
	cursorMin := job.CursorMin
	cursorMax := job.CursorMax
	if cursorMax == "" {
		var minNull, maxNull sql.NullString
		boundsQuery := fmt.Sprintf(`SELECT MIN(%s)::text, MAX(%s)::text FROM %s`, quotedCol, quotedCol, quotedTable)
		if err := target.DB.QueryRowContext(ctx, boundsQuery).Scan(&minNull, &maxNull); err != nil {
			return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
		}
		if !minNull.Valid || !maxNull.Valid {
			// Tabel kosong, tidak ada row yang perlu diproses.
			return ExecutionResult{Status: StatusSuccess}
		}
		cursorMin = minNull.String
		cursorMax = maxNull.String
		if err := e.store.UpdateBatchBounds(ctx, job.ID, cursorMin, cursorMax); err != nil {
			return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
		}
		job.CursorMin = cursorMin
		job.CursorMax = cursorMax
	}

	// Tentukan posisi batch & cursor terakhir dari checkpoint yang sudah committed.
	existingCPs, err := e.store.ListCheckpoints(ctx, rootJobID)
	if err != nil {
		return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
	}
	lastCursor := job.LastCursor
	nextBatchNum := 1
	hasCommitted := false
	for _, cp := range existingCPs {
		if cp.Status == CheckpointCommitted {
			hasCommitted = true
			if cp.BatchNumber >= nextBatchNum {
				nextBatchNum = cp.BatchNumber + 1
				lastCursor = cp.CursorEnd
			}
		}
	}

	for {
		if err := ctx.Err(); err != nil {
			status := StatusFailed
			if hasCommitted {
				status = StatusPartial
			}
			return ExecutionResult{Status: status, Transient: true, Err: err}
		}

		// Ambil window cursor untuk batch berikutnya tanpa memuat seluruh tabel ke memory.
		cursorStart, cursorEnd, windowCount, err := fetchCursorWindow(ctx, target.DB, quotedTable, quotedCol, cursorMin, cursorMax, lastCursor, hasCommitted, job.BatchSize)
		if err != nil {
			status := StatusFailed
			if hasCommitted {
				status = StatusPartial
			}
			return ExecutionResult{Status: status, Transient: IsTransientError(err), Err: err}
		}
		if windowCount == 0 {
			break
		}

		// Cek bukti checkpoint di target DB terlebih dahulu (idempotency & uncertain commit).
		exists, cpStart, cpEnd, cpRows, err := targetCheckpointExists(ctx, target.DB, rootJobID, nextBatchNum, "up")
		if err != nil {
			status := StatusFailed
			if hasCommitted {
				status = StatusPartial
			}
			return ExecutionResult{Status: status, Transient: IsTransientError(err), Err: err}
		}

		var rowsAffected int64
		if exists {
			cursorStart = cpStart
			cursorEnd = cpEnd
			rowsAffected = cpRows
		} else {
			rowsAffected, err = runBatchTx(ctx, target.DB, rootJobID, nextBatchNum, "up", job.SQLUp, cursorStart, cursorEnd)
			if err != nil {
				status := StatusFailed
				if hasCommitted {
					status = StatusPartial
				}
				return ExecutionResult{Status: status, Transient: IsTransientError(err), Err: err}
			}
		}

		// Simpan checkpoint & progres di platform-db setelah commit target berhasil.
		if _, err := e.store.SaveCheckpoint(ctx, Checkpoint{
			JobID:        rootJobID,
			BatchNumber:  nextBatchNum,
			CursorStart:  cursorStart,
			CursorEnd:    cursorEnd,
			RowsAffected: rowsAffected,
			Status:       CheckpointCommitted,
		}); err != nil {
			return ExecutionResult{Status: StatusPartial, Transient: IsTransientError(err), Err: err}
		}
		if err := e.store.RecordBatchProgress(ctx, job.ID, cursorEnd, 1, rowsAffected); err != nil {
			return ExecutionResult{Status: StatusPartial, Transient: IsTransientError(err), Err: err}
		}
		if rootJobID != job.ID {
			_ = e.store.RecordBatchProgress(ctx, rootJobID, cursorEnd, 1, rowsAffected)
		}

		e.metrics.OnBatchCommitted(rowsAffected)
		hasCommitted = true
		lastCursor = cursorEnd
		nextBatchNum++
	}

	if rootJobID != job.ID {
		_ = e.store.MarkSucceeded(ctx, rootJobID)
	}
	return ExecutionResult{Status: StatusSuccess}
}

// executeCancelBatch membatalkan batch yang sudah committed dalam urutan terbalik
// menggunakan SQL down yang immutable. Jika gagal di tengah jalan, status menjadi
// partially_reverted agar dapat dilanjutkan.
func (e *TargetExecutor) executeCancelBatch(ctx context.Context, job *Job) ExecutionResult {
	if job.Irreversible || strings.TrimSpace(job.SQLDown) == "" {
		return ExecutionResult{Status: StatusFailed, Err: ErrUnsafeCancel}
	}
	if err := validateBatchSQL(job.SQLDown); err != nil {
		return ExecutionResult{Status: StatusFailed, Err: fmt.Errorf("%w: %v", ErrUnsafeCancel, err)}
	}

	rootJobID := job.ID
	if job.ParentJobID != nil {
		rootJobID = *job.ParentJobID
	}

	committed, err := e.store.ListCommittedCheckpointsDesc(ctx, rootJobID)
	if err != nil {
		return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
	}
	if len(committed) == 0 {
		if rootJobID != job.ID {
			_ = e.store.MarkReverted(ctx, rootJobID)
		}
		return ExecutionResult{Status: StatusReverted}
	}

	target, err := e.registry.Get(ctx, job.ConnectionID)
	if err != nil {
		return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
	}
	if err := ensureTargetCheckpointTable(ctx, target.DB); err != nil {
		return ExecutionResult{Status: StatusFailed, Transient: IsTransientError(err), Err: err}
	}

	revertedAny := false
	for _, cp := range committed {
		if err := ctx.Err(); err != nil {
			status := StatusFailed
			if revertedAny {
				status = StatusPartiallyReverted
			}
			return ExecutionResult{Status: status, Transient: true, Err: err}
		}

		exists, _, _, cpRows, err := targetCheckpointExists(ctx, target.DB, rootJobID, cp.BatchNumber, "down")
		if err != nil {
			status := StatusFailed
			if revertedAny {
				status = StatusPartiallyReverted
			}
			return ExecutionResult{Status: status, Transient: IsTransientError(err), Err: err}
		}

		var rowsAffected int64
		if exists {
			rowsAffected = cpRows
		} else {
			rowsAffected, err = runBatchTx(ctx, target.DB, rootJobID, cp.BatchNumber, "down", job.SQLDown, cp.CursorStart, cp.CursorEnd)
			if err != nil {
				status := StatusFailed
				if revertedAny {
					status = StatusPartiallyReverted
				}
				return ExecutionResult{Status: status, Transient: IsTransientError(err), Err: err}
			}
		}

		if err := e.store.MarkCheckpointReverted(ctx, cp.ID); err != nil {
			return ExecutionResult{Status: StatusPartiallyReverted, Transient: IsTransientError(err), Err: err}
		}
		_ = e.store.RecordBatchProgress(ctx, job.ID, cp.CursorStart, 1, rowsAffected)
		e.metrics.OnBatchCommitted(rowsAffected)
		revertedAny = true
	}

	if rootJobID != job.ID {
		_ = e.store.MarkReverted(ctx, rootJobID)
	}
	return ExecutionResult{Status: StatusReverted}
}

func fetchCursorWindow(
	ctx context.Context,
	db *sql.DB,
	quotedTable, quotedCol string,
	cursorMin, cursorMax, lastCursor string,
	hasCommitted bool,
	batchSize int,
) (start string, end string, count int, err error) {
	var query string
	var lowerBound string
	if !hasCommitted || lastCursor == "" {
		query = fmt.Sprintf(
			`SELECT %s::text FROM %s WHERE %s >= $1 AND %s <= $2 ORDER BY %s ASC LIMIT $3`,
			quotedCol, quotedTable, quotedCol, quotedCol, quotedCol,
		)
		lowerBound = cursorMin
	} else {
		query = fmt.Sprintf(
			`SELECT %s::text FROM %s WHERE %s > $1 AND %s <= $2 ORDER BY %s ASC LIMIT $3`,
			quotedCol, quotedTable, quotedCol, quotedCol, quotedCol,
		)
		lowerBound = lastCursor
	}

	rows, err := db.QueryContext(ctx, query, lowerBound, cursorMax, batchSize)
	if err != nil {
		return "", "", 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var val string
		if err := rows.Scan(&val); err != nil {
			return "", "", 0, err
		}
		if count == 0 {
			start = val
		}
		end = val
		count++
	}
	if err := rows.Err(); err != nil {
		return "", "", 0, err
	}
	return start, end, count, nil
}

// runBatchTx menjalankan satu batch SQL dan mencatat checkpoint di database target
// dalam satu transaksi atomik.
func runBatchTx(
	ctx context.Context,
	db *sql.DB,
	rootJobID int64,
	batchNum int,
	direction string,
	sqlText string,
	cursorStart, cursorEnd string,
) (int64, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, sqlText, cursorStart, cursorEnd)
	if err != nil {
		return 0, err
	}
	rowsAffected, _ := res.RowsAffected()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO _pathgres_batch_checkpoints (
			job_id, batch_number, direction, cursor_start, cursor_end, rows_affected
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (job_id, batch_number, direction) DO NOTHING`,
		rootJobID, batchNum, direction, cursorStart, cursorEnd, rowsAffected,
	); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return rowsAffected, nil
}

// BackoffDelay menghitung jeda retry eksponensial terbatas untuk kegagalan sementara.
func BackoffDelay(attempt int, base, maxDelay time.Duration) time.Duration {
	if base <= 0 {
		base = 2 * time.Second
	}
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}
	if attempt <= 1 {
		if base > maxDelay {
			return maxDelay
		}
		return base
	}
	delay := base
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= maxDelay {
			return maxDelay
		}
	}
	return delay
}
