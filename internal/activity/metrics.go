package activity

import (
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Metrics struct {
	mu                                sync.Mutex
	running, completed, failed, locks int64
	duration, lockDuration            time.Duration
}

func (m *Metrics) StartMigration() func(bool) {
	if m == nil {
		return func(bool) {}
	}
	start := time.Now()
	m.mu.Lock()
	m.running++
	m.mu.Unlock()
	return func(success bool) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.running--
		m.duration += time.Since(start)
		if success {
			m.completed++
		} else {
			m.failed++
		}
	}
}

func (m *Metrics) ObserveLock(duration time.Duration) {
	if m != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.lockDuration += duration
		m.locks++
	}
}

func (m *Metrics) Handler(pool *pgxpool.Pool, targetStats func() sql.DBStats) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		m.mu.Lock()
		running, completed, failed, locks := m.running, m.completed, m.failed, m.locks
		duration, lockDuration := m.duration, m.lockDuration
		m.mu.Unlock()
		fmt.Fprintf(w, "# TYPE pathgres_migration_running gauge\npathgres_migration_running %d\n", running)
		fmt.Fprintf(w, "# TYPE pathgres_migrations_total counter\npathgres_migrations_total{outcome=\"completed\"} %d\npathgres_migrations_total{outcome=\"failed\"} %d\n", completed, failed)
		fmt.Fprintf(w, "# TYPE pathgres_migration_duration_seconds summary\npathgres_migration_duration_seconds_sum %g\npathgres_migration_duration_seconds_count %d\n", duration.Seconds(), completed+failed)
		fmt.Fprintf(w, "# TYPE pathgres_migration_lock_duration_seconds summary\npathgres_migration_lock_duration_seconds_sum %g\npathgres_migration_lock_duration_seconds_count %d\n", lockDuration.Seconds(), locks)
		stat := pool.Stat()
		target := targetStats()
		fmt.Fprintf(w, "# TYPE pathgres_db_connections gauge\npathgres_db_connections{pool=\"platform\",state=\"active\"} %d\npathgres_db_connections{pool=\"platform\",state=\"idle\"} %d\n", stat.AcquiredConns(), stat.IdleConns())
		fmt.Fprintf(w, "pathgres_db_connections{pool=\"target\",state=\"active\"} %d\npathgres_db_connections{pool=\"target\",state=\"idle\"} %d\n", target.InUse, target.Idle)
		fmt.Fprintf(w, "# TYPE pathgres_db_pool_wait_seconds_total counter\npathgres_db_pool_wait_seconds_total{pool=\"platform\"} %g\npathgres_db_pool_wait_seconds_total{pool=\"target\"} %g\n", stat.EmptyAcquireWaitTime().Seconds(), target.WaitDuration.Seconds())
	}
}
