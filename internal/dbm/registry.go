package dbm

import (
	"context"
	"errors"

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

type PGRegistry struct {
	platform *pgxpool.Pool
}

func NewPGRegistry(platform *pgxpool.Pool) *PGRegistry {
	return &PGRegistry{platform: platform}
}

func (r *PGRegistry) Get(ctx context.Context, connID int64) (*pgxpool.Pool, error) {
	if r == nil || r.platform == nil {
		return nil, ErrConnNotFound
	}

	var url string
	if err := r.platform.QueryRow(ctx,
		`SELECT connection_url FROM db_connections WHERE id = $1`, connID,
	).Scan(&url); err != nil {
		return nil, ErrConnNotFound
	}
	if url == "" {
		return nil, ErrConnNotFound
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func (r *PGRegistry) Evict(connID int64) {}

func (r *PGRegistry) Close() {}

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

func (StubRegistry) Evict(connID int64) {}

func (StubRegistry) Close() {}
