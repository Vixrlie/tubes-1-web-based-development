package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/db"
	"github.com/Vixrlie/tubes-1-web-based-development/migrations"
)

// Sama dengan service test-db di docker-compose.yml dan CI.
const defaultTestDBURL = "postgres://test:test@localhost:5434/test?sslmode=disable"

// NewPool memberi pool ke test-db yang terisolasi: setiap pemanggilan membuat
// schema acak, menjalankan migrasi di dalamnya, dan menghapusnya saat test selesai.
// Test dilewati saat go test -short.
func NewPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test (dilewati dengan -short)")
	}

	url := os.Getenv("TEST_DB_URL")
	if url == "" {
		url = defaultTestDBURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := db.NewPool(ctx, url)
	if err != nil {
		t.Fatalf("tidak bisa konek ke test-db (sudah 'docker compose up -d'?): %v", err)
	}
	t.Cleanup(admin.Close)

	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	schema := "t_" + hex.EncodeToString(b) // huruf kecil dan angka saja, aman tanpa quoting

	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("buat schema: %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(c, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Logf("gagal menghapus schema %s: %v", schema, err)
		}
	})

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DB_URL: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("buat pool: %v", err)
	}
	t.Cleanup(pool.Close) // dijalankan lebih dulu daripada DROP SCHEMA

	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrasi skema: %v", err)
	}
	return pool
}
