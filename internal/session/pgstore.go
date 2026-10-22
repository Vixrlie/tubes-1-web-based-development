package session

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGStore struct {
	pool *pgxpool.Pool
}

var _ Store = (*PGStore)(nil)

func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func (s *PGStore) Insert(ctx context.Context, n NewRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions
		   (token_hash, user_id, csrf_secret, created_at, last_seen_at, rotated_at, absolute_expires_at)
		 VALUES ($1, $2, $3, $4, $4, $4, $5)`,
		n.TokenHash, n.UserID, n.CSRFSecret, n.Now, n.AbsoluteExpiresAt)
	return err
}

func (s *PGStore) FindByTokenHash(ctx context.Context, hash string) (Record, error) {
	var (
		r         Record
		prevHash  *string
		prevUntil *time.Time
		revoked   *time.Time
	)
	err := s.pool.QueryRow(ctx,
		`SELECT s.id, s.user_id, u.username, u.is_platform_admin, u.is_active,
		        s.token_hash, s.previous_token_hash, s.previous_valid_until, s.csrf_secret,
		        s.last_seen_at, s.rotated_at, s.absolute_expires_at, s.revoked_at
		   FROM sessions s
		   JOIN users u ON u.id = s.user_id
		  WHERE s.token_hash = $1 OR s.previous_token_hash = $1
		  LIMIT 1`, hash).
		Scan(&r.ID, &r.UserID, &r.Username, &r.IsPlatformAdmin, &r.IsActive,
			&r.TokenHash, &prevHash, &prevUntil, &r.CSRFSecret,
			&r.LastSeenAt, &r.RotatedAt, &r.AbsoluteExpiresAt, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	if prevHash != nil {
		r.PreviousTokenHash = *prevHash
	}
	if prevUntil != nil {
		r.PreviousValidUntil = *prevUntil
	}
	if revoked != nil {
		r.RevokedAt = *revoked
	}
	return r, nil
}

func (s *PGStore) Touch(ctx context.Context, id int64, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE sessions SET last_seen_at = $2 WHERE id = $1`, id, at)
	return err
}

func (s *PGStore) Rotate(ctx context.Context, id int64, oldHash, newHash string, at, previousValidUntil time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE sessions
		    SET previous_token_hash  = $3,
		        previous_valid_until = $5,
		        token_hash           = $4,
		        rotated_at           = $2,
		        last_seen_at         = $2
		  WHERE id = $1 AND token_hash = $3 AND revoked_at IS NULL`,
		id, at, oldHash, newHash, previousValidUntil)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PGStore) Revoke(ctx context.Context, tokenHash string, at time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = $2
		  WHERE (token_hash = $1 OR previous_token_hash = $1) AND revoked_at IS NULL`,
		tokenHash, at)
	return err
}

func (s *PGStore) RevokeByUser(ctx context.Context, userID int64, at time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`,
		userID, at)
	return err
}
