package db_test

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/db"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

func TestMigrateAppliesAndIsIdempotent(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()

	migrations := fstest.MapFS{
		"0001_a.sql": &fstest.MapFile{Data: []byte("CREATE TABLE mig_a (id BIGINT PRIMARY KEY)")},
		"0002_b.sql": &fstest.MapFile{Data: []byte("CREATE TABLE mig_b (id BIGINT PRIMARY KEY)")},
	}

	if err := db.Migrate(ctx, pool, migrations); err != nil {
		t.Fatalf("Migrate pertama: %v", err)
	}

	// Kedua tabel terbentuk dan tercatat di schema_migrations.
	for _, table := range []string{"mig_a", "mig_b"} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, table).
			Scan(&exists); err != nil {
			t.Fatalf("cek %s: %v", table, err)
		}
		if !exists {
			t.Errorf("tabel %s tidak terbentuk", table)
		}
	}

	// Jalankan ulang: tidak error, tidak menduplikasi catatan.
	if err := db.Migrate(ctx, pool, migrations); err != nil {
		t.Fatalf("Migrate ulang: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE version IN ('0001_a.sql', '0002_b.sql')`).Scan(&count); err != nil {
		t.Fatalf("hitung catatan: %v", err)
	}
	if count != 2 {
		t.Errorf("catatan = %d, want 2 (tanpa duplikat)", count)
	}
}

func TestMigrateRollsBackFailedFile(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()

	broken := fstest.MapFS{
		"0001_ok.sql":  &fstest.MapFile{Data: []byte("CREATE TABLE mig_ok (id BIGINT PRIMARY KEY)")},
		"0002_bad.sql": &fstest.MapFile{Data: []byte("CREATE TABLE mig_bad (id BIGINT PRIMARY KEY); SELECT tidak_ada();")},
	}

	if err := db.Migrate(ctx, pool, broken); err == nil {
		t.Fatal("migrasi rusak tidak menghasilkan error")
	}

	// File pertama sukses dan tercatat; file rusak tidak tercatat dan tidak
	// meninggalkan tabel parsial.
	var okRecorded bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '0001_ok.sql')`).Scan(&okRecorded); err != nil {
		t.Fatalf("cek catatan: %v", err)
	}
	if !okRecorded {
		t.Error("file sukses tidak tercatat")
	}
	var badRecorded bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '0002_bad.sql')`).Scan(&badRecorded); err != nil {
		t.Fatalf("cek catatan: %v", err)
	}
	if badRecorded {
		t.Error("file gagal ikut tercatat")
	}
	var badTable bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'mig_bad')`).Scan(&badTable); err != nil {
		t.Fatalf("cek tabel: %v", err)
	}
	if badTable {
		t.Error("tabel parsial dari migrasi gagal tidak ter-rollback")
	}
}

func TestMigrateSkipsNonSQL(t *testing.T) {
	pool := testutil.NewPool(t)
	mixed := fstest.MapFS{
		"README.md":      &fstest.MapFile{Data: []byte("bukan sql")},
		"0001_c.sql":     &fstest.MapFile{Data: []byte("CREATE TABLE mig_c (id BIGINT PRIMARY KEY)")},
		"sub/0002_d.sql": &fstest.MapFile{Data: []byte("CREATE TABLE mig_d (id BIGINT PRIMARY KEY)")},
	}
	if err := db.Migrate(context.Background(), pool, mixed); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	var exists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'mig_c')`).Scan(&exists); err != nil {
		t.Fatalf("cek: %v", err)
	}
	if !exists {
		t.Error("file sql tidak dijalankan")
	}
	var subExists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'mig_d')`).Scan(&subExists); err != nil {
		t.Fatalf("cek sub: %v", err)
	}
	if subExists {
		t.Error("file di subfolder ikut dijalankan; seharusnya dilewati")
	}
	_ = fs.SkipDir
}
