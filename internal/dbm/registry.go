package dbm

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrConnectionFailed = errors.New("koneksi database gagal")

type Registry interface {
	Get(ctx context.Context, connID int64) (*pgxpool.Pool, error)
	Evict(connID int64)
	Close()
}

type PGRegistry struct {
	mu       sync.Mutex
	pools    map[int64]*pgxpool.Pool
	platform *pgxpool.Pool
}

func NewPGRegistry(platform *pgxpool.Pool) *PGRegistry {
	return &PGRegistry{
		pools:    make(map[int64]*pgxpool.Pool),
		platform: platform,
	}
}

var _ Registry = (*PGRegistry)(nil)

func (r *PGRegistry) Get(ctx context.Context, connID int64) (*pgxpool.Pool, error) {
	r.mu.Lock()
	if p, ok := r.pools[connID]; ok {
		r.mu.Unlock()
		return p, nil
	}
	r.mu.Unlock()

	var connectionURL string
	err := r.platform.QueryRow(ctx,
		`SELECT connection_url FROM db_connections WHERE id = $1`, connID).Scan(&connectionURL)
	if err != nil {
		return nil, ErrConnectionFailed
	}

	config, err := pgxpool.ParseConfig(connectionURL)
	if err != nil {
		return nil, ErrConnectionFailed
	}
	config.MaxConns = 4
	config.MinConns = 0
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, ErrConnectionFailed
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, ErrConnectionFailed
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.pools[connID]; ok {
		pool.Close()
		return existing, nil
	}
	r.pools[connID] = pool
	return pool, nil
}

func (r *PGRegistry) Evict(connID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.pools[connID]; ok {
		delete(r.pools, connID)
		p.Close()
	}
}

func (r *PGRegistry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, p := range r.pools {
		delete(r.pools, id)
		p.Close()
	}
}

// StubRegistry dipakai sampai registry pool target siap dipakai.
type StubRegistry struct{}

var _ Registry = StubRegistry{}

func (StubRegistry) Get(ctx context.Context, connID int64) (*pgxpool.Pool, error) {
	return nil, nil
}

func (StubRegistry) Evict(connID int64) {}

func (StubRegistry) Close() {}
