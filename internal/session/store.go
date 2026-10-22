package session

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("session tidak ditemukan")

// Record adalah satu baris session beserta data user yang dibutuhkan middleware.
// Kolom nullable di database diwakili zero time / string kosong.
type Record struct {
	ID int64

	UserID          int64
	Username        string
	IsPlatformAdmin bool
	IsActive        bool

	TokenHash          string
	PreviousTokenHash  string
	PreviousValidUntil time.Time
	CSRFSecret         string

	LastSeenAt        time.Time
	RotatedAt         time.Time
	AbsoluteExpiresAt time.Time
	RevokedAt         time.Time
}

type NewRecord struct {
	TokenHash         string
	UserID            int64
	CSRFSecret        string
	Now               time.Time
	AbsoluteExpiresAt time.Time
}

type Store interface {
	Insert(ctx context.Context, n NewRecord) error

	// FindByTokenHash mencari berdasarkan token saat ini ATAU token sebelumnya.
	// Mengembalikan ErrNotFound bila tidak ada.
	FindByTokenHash(ctx context.Context, hash string) (Record, error)

	Touch(ctx context.Context, id int64, at time.Time) error

	// Rotate mengganti token hanya jika token saat ini masih oldHash.
	// Mengembalikan false bila keduluan request lain.
	Rotate(ctx context.Context, id int64, oldHash, newHash string, at, previousValidUntil time.Time) (bool, error)

	// Revoke mencabut session yang token saat ini atau sebelumnya cocok.
	// Tidak ada baris yang cocok bukan error.
	Revoke(ctx context.Context, tokenHash string, at time.Time) error
	RevokeByUser(ctx context.Context, userID int64, at time.Time) error
}
