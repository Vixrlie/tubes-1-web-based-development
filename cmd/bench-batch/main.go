package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/async"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/dbm"
)

type singleTargetRegistry struct {
	target *dbm.Target
}

func (r singleTargetRegistry) Get(context.Context, int64) (*dbm.Target, error) { return r.target, nil }
func (r singleTargetRegistry) Evict(int64)                                     {}
func (r singleTargetRegistry) Close()                                          { r.target.Close() }

// noopBenchStore menyimpan progres batch di memory khusus untuk utilitas benchmark CLI.
type noopBenchStore struct {
	batches int
	rows    int64
}

func (s *noopBenchStore) Enqueue(context.Context, async.EnqueueInput) (async.Job, error) {
	return async.Job{}, nil
}
func (s *noopBenchStore) GetJob(context.Context, int64) (async.Job, error) { return async.Job{}, nil }
func (s *noopBenchStore) ListJobsByConnection(context.Context, int64, int) ([]async.Job, error) {
	return nil, nil
}
func (s *noopBenchStore) DequeueNext(context.Context) (*async.Job, bool, error) {
	return nil, false, nil
}
func (s *noopBenchStore) MarkSucceeded(context.Context, int64) error                  { return nil }
func (s *noopBenchStore) MarkReverted(context.Context, int64) error                   { return nil }
func (s *noopBenchStore) MarkFailed(context.Context, int64, string) error             { return nil }
func (s *noopBenchStore) MarkPartial(context.Context, int64, string) error            { return nil }
func (s *noopBenchStore) MarkPartiallyReverted(context.Context, int64, string) error  { return nil }
func (s *noopBenchStore) ScheduleRetry(context.Context, int64, time.Time, string) error { return nil }
func (s *noopBenchStore) RequeueWithoutAttempt(context.Context, int64, time.Time) error { return nil }
func (s *noopBenchStore) UpdateBatchBounds(context.Context, int64, string, string) error {
	return nil
}
func (s *noopBenchStore) RecordBatchProgress(_ context.Context, _ int64, _ string, deltaBatches int, deltaRows int64) error {
	s.batches += deltaBatches
	s.rows += deltaRows
	return nil
}
func (s *noopBenchStore) SaveCheckpoint(_ context.Context, cp async.Checkpoint) (async.Checkpoint, error) {
	return cp, nil
}
func (s *noopBenchStore) ListCheckpoints(context.Context, int64) ([]async.Checkpoint, error) {
	return nil, nil
}
func (s *noopBenchStore) ListCommittedCheckpointsDesc(context.Context, int64) ([]async.Checkpoint, error) {
	return nil, nil
}
func (s *noopBenchStore) MarkCheckpointReverted(context.Context, int64) error            { return nil }
func (s *noopBenchStore) HasBlockingJob(context.Context, int64, int64) (bool, error)     { return false, nil }
func (s *noopBenchStore) RecoverStaleRunningJobs(context.Context) (int64, error)         { return 0, nil }

func main() {
	dsn := flag.String("dsn", "postgres://target:target@localhost:5433/sample?sslmode=disable", "PostgreSQL target DSN")
	rows := flag.Int("rows", 500000, "jumlah row dataset pengujian")
	batchSize := flag.Int("batch-size", 25000, "ukuran batch per transaksi")
	keep := flag.Bool("keep", false, "jangan hapus tabel benchmark setelah selesai")
	flag.Parse()

	ctx := context.Background()
	target, err := dbm.OpenTarget(ctx, *dsn)
	if err != nil {
		log.Fatalf("gagal koneksi ke target DB: %v", err)
	}
	defer target.Close()

	const tableName = "bench_batch_items"
	if !*keep {
		defer func() {
			_, _ = target.DB.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+tableName)
		}()
	}

	fmt.Printf("Menyiapkan tabel %s dengan %d row...\n", tableName, *rows)
	seedStart := time.Now()
	if _, err := target.DB.ExecContext(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, tableName)); err != nil {
		log.Fatalf("gagal drop tabel lama: %v", err)
	}
	if _, err := target.DB.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE %s (
			id      BIGSERIAL PRIMARY KEY,
			payload TEXT NOT NULL,
			status  TEXT NOT NULL DEFAULT 'initial'
		)`, tableName)); err != nil {
		log.Fatalf("gagal membuat tabel: %v", err)
	}
	if _, err := target.DB.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (payload)
		SELECT 'item-' || i || '-' || repeat('a', 32)
		  FROM generate_series(1, $1) i`, tableName), *rows); err != nil {
		log.Fatalf("gagal seed dataset: %v", err)
	}
	fmt.Printf("Seed selesai dalam %v\n\n", time.Since(seedStart).Round(time.Millisecond))

	// 1. Pengujian UPDATE tunggal tanpa batching.
	fmt.Println("1) Menjalankan UPDATE seluruh row TANPA batching...")
	singleStart := time.Now()
	res, err := target.DB.ExecContext(ctx, fmt.Sprintf(
		`UPDATE %s SET status = 'unbatched', payload = md5(payload)`, tableName))
	if err != nil {
		log.Fatalf("single UPDATE gagal: %v", err)
	}
	singleDur := time.Since(singleStart)
	singleRows, _ := res.RowsAffected()
	fmt.Printf("   Selesai: %d row dalam %v\n", singleRows, singleDur.Round(time.Millisecond))
	if singleDur < time.Second {
		fmt.Println("   CATATAN: durasi < 1 detik; naikkan flag -rows (mis. -rows 1000000) agar > 1 detik di mesin ini.")
	}

	// 2. Pengujian dataset yang sama menggunakan Migration per Batch.
	fmt.Printf("\n2) Menjalankan UPDATE dataset yang sama PER BATCH (batch_size=%d)...\n", *batchSize)
	store := &noopBenchStore{}
	exec := async.NewTargetExecutor(store, singleTargetRegistry{target: target}, nil)
	job := &async.Job{
		ID:           time.Now().UnixNano(),
		ConnectionID: 1,
		Action:       async.ActionUp,
		Mode:         async.ModeBatch,
		SQLUp:        fmt.Sprintf(`UPDATE %s SET status = 'batched', payload = md5(payload) WHERE id >= $1 AND id <= $2`, tableName),
		BatchTable:   tableName,
		BatchColumn:  "id",
		BatchSize:    *batchSize,
	}

	batchStart := time.Now()
	execRes := exec.Execute(ctx, job)
	batchDur := time.Since(batchStart)
	if execRes.Err != nil || execRes.Status != async.StatusSuccess {
		log.Fatalf("batch UPDATE gagal: status=%s err=%v", execRes.Status, execRes.Err)
	}

	fmt.Printf("   Selesai: %d batch, %d row dalam %v\n", store.batches, store.rows, batchDur.Round(time.Millisecond))
	if store.batches > 0 {
		fmt.Printf("   Rata-rata per batch: %v\n", (batchDur / time.Duration(store.batches)).Round(time.Millisecond))
	}
}
