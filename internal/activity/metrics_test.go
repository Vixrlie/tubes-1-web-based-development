package activity

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMetricsReportsMigrationOutcomes(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://localhost/test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	metrics := &Metrics{}
	active := metrics.StartMigration()
	metrics.StartMigration()(true)
	metrics.StartMigration()(false)
	metrics.ObserveLock(time.Second)

	res := httptest.NewRecorder()
	metrics.Handler(pool, func() sql.DBStats {
		return sql.DBStats{InUse: 2, WaitDuration: time.Second}
	}).ServeHTTP(res, httptest.NewRequest("GET", "/metrics", nil))
	active(true)
	for _, line := range []string{
		"pathgres_migration_running 1",
		`pathgres_migrations_total{outcome="completed"} 1`,
		`pathgres_migrations_total{outcome="failed"} 1`,
		"pathgres_migration_duration_seconds_count 2",
		"pathgres_migration_lock_duration_seconds_sum 1",
		`pathgres_db_connections{pool="platform",state="idle"} 0`,
		`pathgres_db_connections{pool="target",state="active"} 2`,
		`pathgres_db_pool_wait_seconds_total{pool="target"} 1`,
	} {
		if !strings.Contains(res.Body.String(), line+"\n") {
			t.Errorf("missing metric sample %q", line)
		}
	}
}
