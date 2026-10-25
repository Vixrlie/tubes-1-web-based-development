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
	Get(ctx context.Context, connID int64) (*Target, error)
	Evict(connID int64)
	Close()
}

// StubRegistry adalah registry default yang fail-closed bila target DB belum
// dihubungkan oleh aplikasi utama.
type StubRegistry struct{}

var _ Registry = StubRegistry{}

func (StubRegistry) Get(ctx context.Context, connID int64) (*Target, error) {
	return nil, ErrConnNotFound
}

func (StubRegistry) Evict(connID int64) {}

func (StubRegistry) Close() {}

// connectTimeout membatasi percobaan koneksi ke database target supaya halaman
// tidak menggantung lama saat host tidak merespons.
const connectTimeout = 5 * time.Second

// URLRegistry menyimpan koneksi database target per connID. Koneksi dibuka
// sekali (lazy) sesuai skema URL di db_connections (postgres:// atau mysql://)
// lalu dipakai ulang, lalu Evict menutup saat koneksi diubah/dihapus, dan Close
// menutup semua saat aplikasi berhenti.
type URLRegistry struct {
	mu       sync.Mutex
	targets  map[int64]*Target
	platform *pgxpool.Pool
	closed   bool
}

var _ Registry = (*URLRegistry)(nil)

func NewURLRegistry(platform *pgxpool.Pool) *URLRegistry {
	return &URLRegistry{
		targets:  map[int64]*Target{},
		platform: platform,
	}
}

func (r *URLRegistry) Get(ctx context.Context, connID int64) (*Target, error) {
	if r == nil || r.platform == nil {
		return nil, ErrConnNotFound
	}

	r.mu.Lock()
	if target, ok := r.targets[connID]; ok && target != nil {
		r.mu.Unlock()
		return target, nil
	}
	r.mu.Unlock()

	var rawURL string
	if err := r.platform.QueryRow(ctx,
		`SELECT connection_url FROM db_connections WHERE id = $1`, connID,
	).Scan(&rawURL); err != nil {
		return nil, ErrConnNotFound
	}

	target, err := OpenTarget(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		target.Close()
		return nil, ErrConnNotFound
	}
	// Keduluan pembuat lain: pakai yang sudah ada, tutup yang baru.
	if existing, ok := r.targets[connID]; ok && existing != nil {
		target.Close()
		return existing, nil
	}
	r.targets[connID] = target
	return target, nil
}

// Evict menutup dan melupakan koneksi sebuah target (dipanggil admin saat
// koneksi diubah atau dihapus supaya kredensial lama tidak dipakai lagi).
func (r *URLRegistry) Evict(connID int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if target, ok := r.targets[connID]; ok {
		delete(r.targets, connID)
		target.Close()
	}
}

// Close menutup semua koneksi target. Aman dipanggil lebih dari sekali.
func (r *URLRegistry) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	for id, target := range r.targets {
		delete(r.targets, id)
		target.Close()
	}
}
