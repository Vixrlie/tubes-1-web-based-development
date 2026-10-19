package dbm

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Registry mengelola target connection & live pool. 'Own' lifecycle dari poolnya
type Registry interface {
	Get(ctx context.Context, connID int64) (*pgxpool.Pool, error)
}

type StubRegistry struct{}

// Get func
func (StubRegistry) Get(ctx context.Context, connID int64) (*pgxpool.Pool, error) {
	return nil, nil
}
