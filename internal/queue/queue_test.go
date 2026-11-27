package queue

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

// seedRequest membuat pengajuan minimal yang menjadi pemilik job.
func seedRequest(t *testing.T, pool *pgxpool.Pool, connName string) (reqID, connID int64) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx,
		`INSERT INTO db_connections (name, connection_url) VALUES ($1, 'postgres://unused') RETURNING id`,
		connName).Scan(&connID); err != nil {
		t.Fatal(err)
	}
	var migID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO migrations (connection_id, version, name, up_sql, down_sql, checksum)
		 VALUES ($1, 1, 'm', 'SELECT 1', 'SELECT 2', 'x') RETURNING id`, connID).Scan(&migID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO migration_requests (migration_id, connection_id, action, migration_revision, checksum, status)
		 VALUES ($1, $2, 'up', 1, 'x', 'queued') RETURNING id`, migID, connID).Scan(&reqID); err != nil {
		t.Fatal(err)
	}
	return reqID, connID
}

func enqueue(t *testing.T, pool *pgxpool.Pool, q *Queue, reqID, connID int64) int64 {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	id, err := q.Enqueue(ctx, tx, reqID, connID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return id
}

type jobRow struct {
	Status   string
	Attempts int
}

func readJob(t *testing.T, pool *pgxpool.Pool, id int64) jobRow {
	t.Helper()
	var r jobRow
	if err := pool.QueryRow(context.Background(),
		`SELECT status, attempts FROM migration_jobs WHERE id = $1`, id).Scan(&r.Status, &r.Attempts); err != nil {
		t.Fatal(err)
	}
	return r
}

func makeAvailable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE migration_jobs SET available_at = now()`); err != nil {
		t.Fatal(err)
	}
}

func TestEnqueueIsIdempotentPerRequest(t *testing.T) {
	pool := testutil.NewPool(t)
	q := New(pool)
	reqID, connID := seedRequest(t, pool, "a")
	first := enqueue(t, pool, q, reqID, connID)
	if again := enqueue(t, pool, q, reqID, connID); again != first {
		t.Errorf("enqueue ulang membuat job baru: %d != %d", again, first)
	}
}

func TestWorkerOutcomes(t *testing.T) {
	pool := testutil.NewPool(t)
	q := New(pool)
	q.MaxAttempts = 2
	reqID, connID := seedRequest(t, pool, "a")
	jobID := enqueue(t, pool, q, reqID, connID)

	results := []error{Busy(errors.New("lock")), Retry(errors.New("koneksi putus")), Retry(errors.New("koneksi putus"))}
	calls := 0
	var gaveUp error
	w := NewWorker(pool, func(context.Context, Job) error {
		err := results[calls]
		calls++
		return err
	}, activity.NewWithWriter(io.Discard))
	w.Backoff = 0
	w.OnGiveUp = func(_ context.Context, _ Job, err error) { gaveUp = err }

	ctx := context.Background()
	step := func() {
		t.Helper()
		makeAvailable(t, pool)
		if ok, err := w.ProcessOne(ctx); !ok || err != nil {
			t.Fatalf("ProcessOne: ok=%v err=%v", ok, err)
		}
	}

	step() // busy
	if r := readJob(t, pool, jobID); r.Status != "pending" || r.Attempts != 0 {
		t.Errorf("setelah busy: %+v, want pending tanpa menghabiskan attempt", r)
	}
	step() // retry, attempt 1 dari 2
	if r := readJob(t, pool, jobID); r.Status != "pending" || r.Attempts != 1 {
		t.Errorf("setelah retry: %+v, want pending attempt 1", r)
	}
	step() // retry pada attempt terakhir -> menyerah
	if r := readJob(t, pool, jobID); r.Status != "failed" || r.Attempts != 2 {
		t.Errorf("setelah attempt habis: %+v, want failed", r)
	}
	if gaveUp == nil {
		t.Error("OnGiveUp tidak dipanggil")
	}
	if ok, _ := w.ProcessOne(ctx); ok {
		t.Error("job gagal permanen diambil lagi")
	}
}

func TestWorkerRecoversPanic(t *testing.T) {
	pool := testutil.NewPool(t)
	q := New(pool)
	q.MaxAttempts = 1
	reqID, connID := seedRequest(t, pool, "a")
	jobID := enqueue(t, pool, q, reqID, connID)

	w := NewWorker(pool, func(context.Context, Job) error { panic("boom") }, activity.NewWithWriter(io.Discard))
	if ok, err := w.ProcessOne(context.Background()); !ok || err != nil {
		t.Fatalf("ProcessOne: ok=%v err=%v", ok, err)
	}
	if r := readJob(t, pool, jobID); r.Status != "failed" {
		t.Errorf("panic handler: status %s, want failed", r.Status)
	}
}

func TestClaimSkipsBusyConnection(t *testing.T) {
	pool := testutil.NewPool(t)
	q := New(pool)
	reqA1, connA := seedRequest(t, pool, "a")
	reqB, connB := seedRequest(t, pool, "b")
	jobA1 := enqueue(t, pool, q, reqA1, connA)
	jobB := enqueue(t, pool, q, reqB, connB)

	// Job kedua pada database A dibuat lewat request baru di koneksi yang sama.
	var migID, reqA2 int64
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT migration_id FROM migration_requests WHERE id = $1`, reqA1).Scan(&migID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO migration_requests (migration_id, connection_id, action, migration_revision, checksum, status)
		 VALUES ($1, $2, 'forward', 1, 'x', 'cancelled') RETURNING id`, migID, connA).Scan(&reqA2); err != nil {
		t.Fatal(err)
	}
	jobA2 := enqueue(t, pool, q, reqA2, connA)

	// Job A1 sedang running (lease aktif).
	if _, err := pool.Exec(ctx,
		`UPDATE migration_jobs SET status = 'running', locked_at = now(), attempts = 1 WHERE id = $1`, jobA1); err != nil {
		t.Fatal(err)
	}

	var claimed []int64
	w := NewWorker(pool, func(_ context.Context, j Job) error {
		claimed = append(claimed, j.ID)
		return nil
	}, activity.NewWithWriter(io.Discard))
	for {
		ok, err := w.ProcessOne(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
	}
	if len(claimed) != 1 || claimed[0] != jobB {
		t.Errorf("job diproses = %v, want hanya job database B (%d); job A2 (%d) harus menunggu A1", claimed, jobB, jobA2)
	}
}
