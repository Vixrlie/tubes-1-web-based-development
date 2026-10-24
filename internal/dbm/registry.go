package dbm

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrConnNotFound = errors.New("koneksi database target tidak ditemukan")

type Registry interface {
	Get(ctx context.Context, connID int64) (*pgxpool.Pool, error)
	Evict(connID int64)
	Close()
}

// StubRegistry adalah registry default yang fail-closed bila target DB belum
// dihubungkan oleh aplikasi utama.
type StubRegistry struct{}

var _ Registry = StubRegistry{}

func (StubRegistry) Get(ctx context.Context, connID int64) (*pgxpool.Pool, error) {
	return nil, ErrConnNotFound
}

func (StubRegistry) Evict(connID int64) {}

func (StubRegistry) Close() {}

// connectTimeout membatasi percobaan koneksi ke database target supaya halaman
// tidak menggantung lama saat host tidak merespons.
const connectTimeout = 5 * time.Second

// PGRegistry menyimpan pool database target per connID. Pool dibuat sekali
// (lazy) lalu dipakai ulang; Evict menutup pool saat koneksi diubah/dihapus,
// dan Close menutup semua pool saat aplikasi berhenti.
type PGRegistry struct {
	mu       sync.Mutex
	pools    map[int64]*pgxpool.Pool
	platform *pgxpool.Pool
	closed   bool
}

var _ Registry = (*PGRegistry)(nil)

func NewPGRegistry(platform *pgxpool.Pool) *PGRegistry {
	return &PGRegistry{
		pools:    map[int64]*pgxpool.Pool{},
		platform: platform,
	}
}

func (r *PGRegistry) Get(ctx context.Context, connID int64) (*pgxpool.Pool, error) {
	if r == nil || r.platform == nil {
		return nil, ErrConnNotFound
	}

	r.mu.Lock()
	if pool, ok := r.pools[connID]; ok && pool != nil {
		r.mu.Unlock()
		return pool, nil
	}
	r.mu.Unlock()

	var url string
	if err := r.platform.QueryRow(ctx,
		`SELECT connection_url FROM db_connections WHERE id = $1`, connID,
	).Scan(&url); err != nil {
		return nil, ErrConnNotFound
	}
	if url == "" {
		return nil, ErrConnNotFound
	}

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	pool, err := pgxpool.New(connectCtx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		pool.Close()
		return nil, ErrConnNotFound
	}
	// Keduluan pembuat lain: pakai yang sudah ada, tutup yang baru.
	if existing, ok := r.pools[connID]; ok && existing != nil {
		pool.Close()
		return existing, nil
	}
	r.pools[connID] = pool
	return pool, nil
}

// Evict menutup dan melupakan pool sebuah koneksi (dipanggil admin saat
// koneksi diubah atau dihapus supaya kredensial lama tidak dipakai lagi).
func (r *PGRegistry) Evict(connID int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if pool, ok := r.pools[connID]; ok {
		delete(r.pools, connID)
		if pool != nil {
			pool.Close()
		}
	}
}

// Close menutup semua pool. Aman dipanggil lebih dari sekali.
func (r *PGRegistry) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	for id, pool := range r.pools {
		delete(r.pools, id)
		if pool != nil {
			pool.Close()
		}
	}
}

// PoolRegistry adalah registry yang pool-nya sudah disiapkan pemanggil
// (dipakai integration test).
type PoolRegistry struct {
	pools map[int64]*pgxpool.Pool
}

func NewPoolRegistry(pools map[int64]*pgxpool.Pool) *PoolRegistry {
	return &PoolRegistry{pools: pools}
}

func (r *PoolRegistry) Get(ctx context.Context, connID int64) (*pgxpool.Pool, error) {
	if r == nil || r.pools == nil {
		return nil, ErrConnNotFound
	}
	pool, ok := r.pools[connID]
	if !ok || pool == nil {
		return nil, ErrConnNotFound
	}
	return pool, nil
}

func (r *PoolRegistry) Evict(connID int64) {}

func (r *PoolRegistry) Close() {}
