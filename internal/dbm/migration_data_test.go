package dbm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/db"
	"github.com/Vixrlie/tubes-1-web-based-development/migrations"
)

// emptySchemaPool membuat schema acak tanpa menjalankan migrasi apa pun.
func emptySchemaPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test (dilewati dengan -short)")
	}
	url := os.Getenv("TEST_DB_URL")
	if url == "" {
		url = "postgres://test:test@localhost:5434/test?sslmode=disable"
	}
	ctx := context.Background()
	admin, err := db.NewPool(ctx, url)
	if err != nil {
		t.Fatalf("konek test-db: %v", err)
	}
	t.Cleanup(admin.Close)

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	schema := "t_" + hex.EncodeToString(b)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// migrationsBefore mengambil file migrasi platform dengan nama < limit.
func migrationsBefore(t *testing.T, limit string) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() >= limit {
			continue
		}
		body, err := fs.ReadFile(migrations.FS, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = &fstest.MapFile{Data: body}
	}
	return out
}

// Data Milestone 1 (migration_history) harus terbawa ke model workflow baru.
func TestMigration0004CarriesMilestone1History(t *testing.T) {
	ctx := context.Background()
	pool := emptySchemaPool(t)
	if err := db.Migrate(ctx, pool, migrationsBefore(t, "0004")); err != nil {
		t.Fatalf("migrasi 0001-0003: %v", err)
	}

	var userID, connID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (username, password_hash) VALUES ('m1', 'x') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO db_connections (name, connection_url) VALUES ('m1db', 'postgres://x') RETURNING id`).Scan(&connID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO migration_history (connection_id, version, sql, checksum, status, error, created_by, executed_by, executed_at)
		 VALUES ($1, 202601010000, '-- draft lama\nCREATE TABLE a (id INT)', 'c1', 'draft',   NULL,   $2, NULL, NULL),
		        ($1, 202601020000, '-- sudah jalan\nCREATE TABLE b (id INT)', 'c2', 'applied', NULL,   $2, $2,   now()),
		        ($1, 202601030000, '-- gagal\nCREATE TABLE c (',          'c3', 'failed',  'syntax', $2, $2,   now())`,
		connID, userID); err != nil {
		t.Fatal(err)
	}

	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrasi 0004: %v", err)
	}

	states, err := loadStates(ctx, pool, connID)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 3 {
		t.Fatalf("migrations = %d, want 3", len(states))
	}
	byState := map[string]int{}
	for _, s := range states {
		if s.Reversible {
			t.Error("migration M1 tanpa SQL down harus irreversible")
		}
		byState[s.State()]++
	}
	if byState[StateDraft] != 1 || byState[StateActive] != 1 || byState[StateFailed] != 1 {
		t.Errorf("state hasil migrasi data = %v, want 1 draft, 1 active, 1 failed", byState)
	}

	var badChecksums int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM migrations WHERE checksum <> encode(sha256(convert_to(
	            up_sql || chr(30) || COALESCE(down_sql, '') || chr(30) || COALESCE(irreversible_reason, ''), 'UTF8')), 'hex')`).
		Scan(&badChecksums); err != nil {
		t.Fatal(err)
	}
	if badChecksums != 0 {
		t.Errorf("%d checksum tidak sesuai DefinitionChecksum", badChecksums)
	}
	var stillThere bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('migration_history') IS NOT NULL`).Scan(&stillThere); err != nil {
		t.Fatal(err)
	}
	if stillThere {
		t.Error("tabel migration_history lama masih ada")
	}
}
