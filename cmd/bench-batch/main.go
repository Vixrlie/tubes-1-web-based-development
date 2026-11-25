// Command bench-batch menyiapkan dataset besar di database target lalu
// membandingkan satu UPDATE tanpa batching dengan migration per batch pada
// dataset yang sama (ketentuan pengujian Milestone 2).
//
// Diadaptasi dari cmd/bench-batch di feat/async; kini memakai dbm.BatchRun
// yang juga dipakai worker.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/dbm"
)

func main() {
	dsn := flag.String("dsn", "postgres://target:target@localhost:5433/sample?sslmode=disable", "DSN PostgreSQL database target")
	rows := flag.Int("rows", 2_000_000, "jumlah row dataset pengujian")
	batchSize := flag.Int("batch-size", 50_000, "ukuran batch per transaksi")
	keep := flag.Bool("keep", false, "jangan hapus tabel benchmark setelah selesai")
	flag.Parse()

	ctx := context.Background()
	target, err := dbm.OpenTarget(ctx, *dsn)
	if err != nil {
		log.Fatalf("gagal koneksi ke database target: %v", err)
	}
	defer target.Close()

	const table = "bench_batch_items"
	if !*keep {
		defer func() {
			_, _ = target.DB.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)
		}()
	}

	fmt.Printf("Menyiapkan tabel %s dengan %d row...\n", table, *rows)
	seedStart := time.Now()
	for _, q := range []string{
		`DROP TABLE IF EXISTS ` + table,
		`CREATE TABLE ` + table + ` (id BIGSERIAL PRIMARY KEY, payload TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'initial')`,
	} {
		if _, err := target.DB.ExecContext(ctx, q); err != nil {
			log.Fatalf("menyiapkan tabel: %v", err)
		}
	}
	if _, err := target.DB.ExecContext(ctx,
		`INSERT INTO `+table+` (payload) SELECT 'item-' || i || '-' || repeat('a', 32) FROM generate_series(1, $1::int) i`,
		*rows); err != nil {
		log.Fatalf("seed dataset: %v", err)
	}
	fmt.Printf("Seed selesai dalam %v\n\n", time.Since(seedStart).Round(time.Millisecond))

	fmt.Println("1) UPDATE seluruh row TANPA batching (satu transaksi)...")
	singleStart := time.Now()
	res, err := target.DB.ExecContext(ctx, `UPDATE `+table+` SET status = 'unbatched', payload = md5(payload)`)
	if err != nil {
		log.Fatalf("UPDATE tunggal gagal: %v", err)
	}
	singleDur := time.Since(singleStart)
	singleRows, _ := res.RowsAffected()
	fmt.Printf("   %d row dalam %v\n", singleRows, singleDur.Round(time.Millisecond))
	if singleDur < time.Second {
		fmt.Println("   CATATAN: durasi < 1 detik; naikkan -rows agar memenuhi ketentuan pengujian.")
	}

	fmt.Printf("\n2) Dataset yang sama PER BATCH (batch_size=%d)...\n", *batchSize)
	run := dbm.BatchRun{
		DB: target.DB, Dialect: target.Dialect,
		Config:    dbm.BatchConfig{Table: table, Column: "id", Size: *batchSize},
		Key:       time.Now().UnixNano(),
		Direction: dbm.BatchUp,
		SQL:       `UPDATE ` + table + ` SET status = 'batched', payload = md5(payload) WHERE id BETWEEN $1 AND $2`,
	}
	var slowest time.Duration
	last := time.Now()
	run.Progress = func(cp dbm.BatchCheckpoint, total, committedRows int64) error {
		if d := time.Since(last); d > slowest {
			slowest = d
		}
		last = time.Now()
		return nil
	}
	batchStart := time.Now()
	out, err := run.Run(ctx)
	batchDur := time.Since(batchStart)
	if err != nil {
		log.Fatalf("migration per batch gagal setelah %d batch: %v", out.Batches, err)
	}
	fmt.Printf("   %d batch, %d row dalam %v\n", out.Batches, out.Rows, batchDur.Round(time.Millisecond))
	if out.Batches > 0 {
		fmt.Printf("   Rata-rata per batch: %v, batch terlama: %v\n",
			(batchDur / time.Duration(out.Batches)).Round(time.Millisecond), slowest.Round(time.Millisecond))
	}
	_, _ = target.DB.ExecContext(ctx, `DELETE FROM _pathgres_batch_checkpoints WHERE batch_key = $1`, run.Key)
}
