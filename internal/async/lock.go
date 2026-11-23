package async

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const DefaultLockTTL = 30 * time.Second

// LockManager mengelola distributed lock per target database lintas worker,
// termasuk pembaruan lease (heartbeat) dan pemulihan lock kedaluwarsa saat crash.
type LockManager interface {
	Acquire(ctx context.Context, connID, jobID int64, workerID string, ttl time.Duration, allowBlocked bool, parentJobID *int64) (Lock, error)
	Renew(ctx context.Context, connID, jobID int64, workerID string, ttl time.Duration) error
	Release(ctx context.Context, connID, jobID int64, workerID string) error
	RecoverExpired(ctx context.Context) ([]Lock, error)
	StartHeartbeat(ctx context.Context, connID, jobID int64, workerID string, ttl time.Duration) (stop func())
}

// PGLockManager adalah implementasi LockManager berbasis tabel migration_locks di platform-db.
type PGLockManager struct {
	pool *pgxpool.Pool
}

var _ LockManager = (*PGLockManager)(nil)

func NewPGLockManager(pool *pgxpool.Pool) *PGLockManager {
	return &PGLockManager{pool: pool}
}

// Acquire memperoleh lock eksklusif untuk satu connection_id.
// Jika allowBlocked false, Acquire menolak jika ada job lain yang masih berstatus
// 'partial' atau 'partially_reverted' pada database yang sama.
// Jika lease sebelumnya sudah kedaluwarsa (worker crash), Acquire mengambil alih lock secara atomik.
func (m *PGLockManager) Acquire(
	ctx context.Context,
	connID, jobID int64,
	workerID string,
	ttl time.Duration,
	allowBlocked bool,
	parentJobID *int64,
) (Lock, error) {
	if ttl <= 0 {
		ttl = DefaultLockTTL
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return Lock{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if !allowBlocked {
		excludeID := jobID
		if parentJobID != nil {
			excludeID = *parentJobID
		}
		var blocked bool
		err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				  FROM migration_jobs
				 WHERE connection_id = $1
				   AND id <> $2
				   AND id <> $3
				   AND status IN ('partial', 'partially_reverted')
			)`, connID, jobID, excludeID).Scan(&blocked)
		if err != nil {
			return Lock{}, err
		}
		if blocked {
			return Lock{}, ErrDatabaseBlocked
		}
	}

	expiresAt := time.Now().UTC().Add(ttl)
	var lock Lock
	err = tx.QueryRow(ctx, `
		INSERT INTO migration_locks (connection_id, job_id, worker_id, locked_at, lease_expires_at)
		VALUES ($1, $2, $3, now(), $4)
		ON CONFLICT (connection_id) DO UPDATE
		   SET job_id = EXCLUDED.job_id,
		       worker_id = EXCLUDED.worker_id,
		       locked_at = now(),
		       lease_expires_at = EXCLUDED.lease_expires_at
		 WHERE migration_locks.lease_expires_at <= now()
		    OR (migration_locks.job_id = EXCLUDED.job_id AND migration_locks.worker_id = EXCLUDED.worker_id)
		RETURNING connection_id, job_id, worker_id, locked_at, lease_expires_at`,
		connID, jobID, workerID, expiresAt,
	).Scan(&lock.ConnectionID, &lock.JobID, &lock.WorkerID, &lock.LockedAt, &lock.LeaseExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lock{}, ErrDatabaseLocked
	}
	if err != nil {
		return Lock{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Lock{}, err
	}
	return lock, nil
}

// Renew memperpanjang lease lock yang masih dimiliki oleh worker ini.
func (m *PGLockManager) Renew(ctx context.Context, connID, jobID int64, workerID string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = DefaultLockTTL
	}
	expiresAt := time.Now().UTC().Add(ttl)
	tag, err := m.pool.Exec(ctx, `
		UPDATE migration_locks
		   SET lease_expires_at = $4
		 WHERE connection_id = $1
		   AND job_id = $2
		   AND worker_id = $3`,
		connID, jobID, workerID, expiresAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDatabaseLocked
	}
	return nil
}

// Release melepas lock milik worker ini setelah eksekusi selesai.
func (m *PGLockManager) Release(ctx context.Context, connID, jobID int64, workerID string) error {
	_, err := m.pool.Exec(ctx, `
		DELETE FROM migration_locks
		 WHERE connection_id = $1
		   AND job_id = $2
		   AND worker_id = $3`,
		connID, jobID, workerID,
	)
	return err
}

// RecoverExpired menghapus semua lock yang lease-nya sudah kedaluwarsa akibat worker crash
// dan mengembalikan daftar lock yang dipulihkan.
func (m *PGLockManager) RecoverExpired(ctx context.Context) ([]Lock, error) {
	rows, err := m.pool.Query(ctx, `
		DELETE FROM migration_locks
		 WHERE lease_expires_at <= now()
		RETURNING connection_id, job_id, worker_id, locked_at, lease_expires_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expired []Lock
	for rows.Next() {
		var l Lock
		if err := rows.Scan(&l.ConnectionID, &l.JobID, &l.WorkerID, &l.LockedAt, &l.LeaseExpiresAt); err != nil {
			return nil, err
		}
		expired = append(expired, l)
	}
	return expired, rows.Err()
}

// StartHeartbeat menjalankan goroutine yang memperpanjang lease secara berkala
// setiap ttl/3 hingga fungsi stop() dipanggil atau ctx dibatalkan.
func (m *PGLockManager) StartHeartbeat(
	ctx context.Context,
	connID, jobID int64,
	workerID string,
	ttl time.Duration,
) func() {
	if ttl <= 0 {
		ttl = DefaultLockTTL
	}
	interval := ttl / 3
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}

	hbCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				renewCtx, renewCancel := context.WithTimeout(hbCtx, interval)
				_ = m.Renew(renewCtx, connID, jobID, workerID, ttl)
				renewCancel()
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			wg.Wait()
		})
	}
}
