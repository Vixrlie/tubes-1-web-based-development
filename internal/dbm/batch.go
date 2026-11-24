package dbm

// Migration per batch. Diadaptasi dari internal/async (feat/async) ke dalam
// pipeline workflow: cursor berbasis kolom unik, checkpoint di database target
// dalam transaksi yang sama dengan batch, resume tanpa mengulang batch yang
// sudah committed, dan pembatalan per batch dalam urutan terbalik.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Arah rangkaian batch: up menerapkan SQL up, down membatalkan per batch.
const (
	BatchUp   = "up"
	BatchDown = "down"
)

// BatchConfig = bagian definisi migration untuk mode batch.
type BatchConfig struct {
	Table  string `json:"table"`
	Column string `json:"column"`
	Size   int    `json:"size"`
}

// Enabled true bila migration memakai mode batch.
func (c *BatchConfig) Enabled() bool { return c != nil && c.Table != "" }

var (
	ErrBatchConfig       = errors.New("konfigurasi batch tidak valid: isi tabel, kolom cursor, dan ukuran batch > 0")
	ErrBatchPlaceholders = errors.New("SQL mode batch wajib memakai placeholder $1 (awal) dan $2 (akhir) rentang cursor")
	ErrBatchIdentifier   = errors.New("nama tabel atau kolom cursor batch tidak valid")
)

// MaxBatchSize membatasi ukuran satu transaksi batch.
const MaxBatchSize = 1_000_000

var batchIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Normalize merapikan konfigurasi; nil bila mode batch tidak dipakai.
func (c *BatchConfig) Normalize() (*BatchConfig, error) {
	if c == nil {
		return nil, nil
	}
	out := BatchConfig{Table: strings.TrimSpace(c.Table), Column: strings.TrimSpace(c.Column), Size: c.Size}
	if out.Table == "" && out.Column == "" && out.Size == 0 {
		return nil, nil
	}
	if out.Table == "" || out.Column == "" || out.Size <= 0 || out.Size > MaxBatchSize {
		return nil, ErrBatchConfig
	}
	if _, err := splitBatchTable(out.Table); err != nil {
		return nil, err
	}
	if !batchIdent.MatchString(out.Column) {
		return nil, ErrBatchIdentifier
	}
	return &out, nil
}

// splitBatchTable menerima "tabel" atau "schema.tabel".
func splitBatchTable(table string) ([]string, error) {
	parts := strings.Split(table, ".")
	if len(parts) > 2 {
		return nil, ErrBatchIdentifier
	}
	for _, p := range parts {
		if !batchIdent.MatchString(p) {
			return nil, ErrBatchIdentifier
		}
	}
	return parts, nil
}

// hasBatchPlaceholders memeriksa $1 dan $2 sebagai token utuh ($10 tidak dihitung).
func hasBatchPlaceholders(sqlText string) bool {
	return placeholderPattern(1).MatchString(sqlText) && placeholderPattern(2).MatchString(sqlText)
}

func placeholderPattern(n int) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`\$%d([^0-9]|$)`, n))
}

// targetCheckpointDDL dibuat di database target. Baris batch_number 0 arah
// 'scope' menyimpan cakupan cursor yang ditetapkan sejak batch pertama.
const targetCheckpointDDL = `
CREATE TABLE IF NOT EXISTS _pathgres_batch_checkpoints (
    batch_key     BIGINT      NOT NULL,
    direction     TEXT        NOT NULL,
    batch_number  INT         NOT NULL,
    cursor_start  TEXT        NOT NULL,
    cursor_end    TEXT        NOT NULL,
    rows_affected BIGINT      NOT NULL DEFAULT 0,
    committed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (batch_key, direction, batch_number)
)`

// BatchCheckpoint = satu batch yang sudah committed di database target.
type BatchCheckpoint struct {
	Number      int    `json:"batch_number"`
	CursorStart string `json:"cursor_start"`
	CursorEnd   string `json:"cursor_end"`
	Rows        int64  `json:"rows_affected"`
}

// BatchProgress dipanggil setelah setiap batch committed.
type BatchProgress func(cp BatchCheckpoint, totalBatches, totalRows int64) error

// BatchRun = satu rangkaian batch terhadap database target.
type BatchRun struct {
	DB        *sql.DB
	Dialect   Dialect
	Config    BatchConfig
	Key       int64
	Direction string
	SQL       string
	// LockSQL dijalankan di awal setiap transaksi batch (lock lintas proses
	// di database target, sama dengan dry-run).
	LockSQL  string
	Progress BatchProgress
}

// BatchResult merangkum rangkaian: total committed (termasuk dari attempt sebelumnya).
type BatchResult struct {
	Batches    int64
	Rows       int64
	TotalBatch int64
	LastCursor string
	RunRows    int64 // row yang diproses attempt ini saja
	RunBatches int64
}

// Committed true bila rangkaian sudah punya minimal satu batch committed.
func (r BatchResult) Committed() bool { return r.Batches > 0 }

func (b BatchRun) quoted() (table, column string, err error) {
	parts, err := splitBatchTable(b.Config.Table)
	if err != nil {
		return "", "", err
	}
	if !batchIdent.MatchString(b.Config.Column) {
		return "", "", ErrBatchIdentifier
	}
	return b.Dialect.QuoteIdent(parts...), b.Dialect.QuoteIdent(b.Config.Column), nil
}

// EnsureCheckpointTable membuat tabel checkpoint di database target.
func EnsureCheckpointTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, targetCheckpointDDL)
	return err
}

// LoadCheckpoints membaca checkpoint committed untuk satu arah, urut naik.
func LoadCheckpoints(ctx context.Context, db *sql.DB, key int64, direction string) ([]BatchCheckpoint, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT batch_number, cursor_start, cursor_end, rows_affected
		   FROM _pathgres_batch_checkpoints
		  WHERE batch_key = $1 AND direction = $2 AND batch_number > 0
		  ORDER BY batch_number`, key, direction)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BatchCheckpoint
	for rows.Next() {
		var cp BatchCheckpoint
		if err := rows.Scan(&cp.Number, &cp.CursorStart, &cp.CursorEnd, &cp.Rows); err != nil {
			return nil, err
		}
		out = append(out, cp)
	}
	return out, rows.Err()
}

// scope = cakupan [min, max] cursor yang dikunci sejak awal rangkaian, agar
// row yang ditambahkan setelah migration dimulai tidak ikut diproses.
func (b BatchRun) scope(ctx context.Context, table, column string) (lo, hi string, empty bool, err error) {
	err = b.DB.QueryRowContext(ctx,
		`SELECT cursor_start, cursor_end FROM _pathgres_batch_checkpoints
		  WHERE batch_key = $1 AND direction = 'scope' AND batch_number = 0`, b.Key).Scan(&lo, &hi)
	if err == nil {
		return lo, hi, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", "", false, err
	}
	var minV, maxV sql.NullString
	if err := b.DB.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT MIN(%s)::text, MAX(%s)::text FROM %s`, column, column, table)).Scan(&minV, &maxV); err != nil {
		return "", "", false, err
	}
	if !minV.Valid {
		return "", "", true, nil
	}
	if _, err := b.DB.ExecContext(ctx,
		`INSERT INTO _pathgres_batch_checkpoints (batch_key, direction, batch_number, cursor_start, cursor_end)
		 VALUES ($1, 'scope', 0, $2, $3) ON CONFLICT DO NOTHING`, b.Key, minV.String, maxV.String); err != nil {
		return "", "", false, err
	}
	// Baca ulang: bila attempt lain sempat menulis lebih dulu, cakupannya yang dipakai.
	return b.scope(ctx, table, column)
}

// window mencari rentang batch berikutnya tanpa memuat row ke memory: hanya
// agregat min/max/count dari LIMIT batch_size nilai cursor berikutnya.
func (b BatchRun) window(ctx context.Context, table, column, after, hi string, first bool) (start, end string, n int64, err error) {
	op := ">"
	if first {
		op = ">="
	}
	var s, e sql.NullString
	q := fmt.Sprintf(`SELECT MIN(c)::text, MAX(c)::text, COUNT(*) FROM (
	                    SELECT %[2]s AS c FROM %[1]s
	                     WHERE %[2]s %[3]s $1 AND %[2]s <= $2
	                     ORDER BY %[2]s LIMIT $3) w`, table, column, op)
	if err := b.DB.QueryRowContext(ctx, q, after, hi, b.Config.Size).Scan(&s, &e, &n); err != nil {
		return "", "", 0, err
	}
	return s.String, e.String, n, nil
}

// countBatches memperkirakan jumlah batch dalam cakupan (untuk progres).
func (b BatchRun) countBatches(ctx context.Context, table, column, lo, hi string) (int64, error) {
	var rows int64
	if err := b.DB.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %[1]s WHERE %[2]s >= $1 AND %[2]s <= $2`, table, column), lo, hi).Scan(&rows); err != nil {
		return 0, err
	}
	size := int64(b.Config.Size)
	return (rows + size - 1) / size, nil
}

// Run menjalankan rangkaian sampai selesai atau gagal. Batch yang sudah punya
// checkpoint dilewati, sehingga Run aman dipanggil ulang setelah kegagalan.
func (b BatchRun) Run(ctx context.Context) (BatchResult, error) {
	if b.Direction == BatchDown {
		return b.runDown(ctx)
	}
	return b.runUp(ctx)
}

func (b BatchRun) runUp(ctx context.Context) (BatchResult, error) {
	var res BatchResult
	table, column, err := b.quoted()
	if err != nil {
		return res, err
	}
	if err := EnsureCheckpointTable(ctx, b.DB); err != nil {
		return res, err
	}
	done, err := LoadCheckpoints(ctx, b.DB, b.Key, BatchUp)
	if err != nil {
		return res, err
	}
	for _, cp := range done {
		res.Batches++
		res.Rows += cp.Rows
		res.LastCursor = cp.CursorEnd
	}
	lo, hi, empty, err := b.scope(ctx, table, column)
	if err != nil || empty {
		return res, err
	}
	if total, err := b.countBatches(ctx, table, column, lo, hi); err == nil {
		res.TotalBatch = max(total, res.Batches)
	}

	after, first := lo, true
	if res.LastCursor != "" {
		after, first = res.LastCursor, false
	}
	for {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		start, end, n, err := b.window(ctx, table, column, after, hi, first)
		if err != nil {
			return res, err
		}
		if n == 0 {
			res.TotalBatch = res.Batches
			return res, nil
		}
		cp := BatchCheckpoint{Number: int(res.Batches) + 1, CursorStart: start, CursorEnd: end}
		cp.Rows, err = b.commitBatch(ctx, BatchUp, cp)
		if err != nil {
			return res, err
		}
		res.Batches++
		res.RunBatches++
		res.Rows += cp.Rows
		res.RunRows += cp.Rows
		res.LastCursor = end
		res.TotalBatch = max(res.TotalBatch, res.Batches)
		if b.Progress != nil {
			if err := b.Progress(cp, res.TotalBatch, res.Rows); err != nil {
				return res, err
			}
		}
		after, first = end, false
	}
}

// runDown membatalkan batch up yang committed dalam urutan terbalik memakai
// SQL down per rentang cursor batch tersebut.
func (b BatchRun) runDown(ctx context.Context) (BatchResult, error) {
	var res BatchResult
	if err := EnsureCheckpointTable(ctx, b.DB); err != nil {
		return res, err
	}
	ups, err := LoadCheckpoints(ctx, b.DB, b.Key, BatchUp)
	if err != nil {
		return res, err
	}
	downs, err := LoadCheckpoints(ctx, b.DB, b.Key, BatchDown)
	if err != nil {
		return res, err
	}
	reverted := make(map[int]bool, len(downs))
	for _, cp := range downs {
		reverted[cp.Number] = true
		res.Batches++
		res.Rows += cp.Rows
	}
	res.TotalBatch = int64(len(ups))
	for i := len(ups) - 1; i >= 0; i-- {
		up := ups[i]
		if reverted[up.Number] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}
		cp := BatchCheckpoint{Number: up.Number, CursorStart: up.CursorStart, CursorEnd: up.CursorEnd}
		cp.Rows, err = b.commitBatch(ctx, BatchDown, cp)
		if err != nil {
			return res, err
		}
		res.Batches++
		res.RunBatches++
		res.Rows += cp.Rows
		res.RunRows += cp.Rows
		res.LastCursor = up.CursorStart
		if b.Progress != nil {
			if err := b.Progress(cp, res.TotalBatch, res.Rows); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

// commitBatch: SQL batch dan checkpoint-nya di-commit dalam satu transaksi.
// Bila commit gagal tanpa kepastian, keberadaan checkpoint menjadi buktinya.
func (b BatchRun) commitBatch(ctx context.Context, direction string, cp BatchCheckpoint) (int64, error) {
	conn, err := b.DB.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck
	if b.LockSQL != "" {
		if _, err := tx.ExecContext(ctx, b.LockSQL); err != nil {
			return 0, err
		}
	}
	res, err := tx.ExecContext(ctx, b.SQL, cp.CursorStart, cp.CursorEnd)
	if err != nil {
		return 0, err
	}
	rows, _ := res.RowsAffected()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO _pathgres_batch_checkpoints
		        (batch_key, direction, batch_number, cursor_start, cursor_end, rows_affected)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		b.Key, direction, cp.Number, cp.CursorStart, cp.CursorEnd, rows); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		var exists bool
		if qerr := b.DB.QueryRowContext(context.WithoutCancel(ctx),
			`SELECT EXISTS (SELECT 1 FROM _pathgres_batch_checkpoints
			                 WHERE batch_key = $1 AND direction = $2 AND batch_number = $3)`,
			b.Key, direction, cp.Number).Scan(&exists); qerr == nil && exists {
			return rows, nil
		}
		return 0, err
	}
	return rows, nil
}

// FirstWindow mengembalikan rentang batch pertama untuk dry-run validasi.
func (b BatchRun) FirstWindow(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}) (start, end string, n int64, err error) {
	table, column, err := b.quoted()
	if err != nil {
		return "", "", 0, err
	}
	var s, e sql.NullString
	err = q.QueryRowContext(ctx, fmt.Sprintf(`SELECT MIN(c)::text, MAX(c)::text, COUNT(*) FROM (
	        SELECT %[2]s AS c FROM %[1]s ORDER BY %[2]s LIMIT $1) w`, table, column), b.Config.Size).Scan(&s, &e, &n)
	return s.String, e.String, n, err
}
