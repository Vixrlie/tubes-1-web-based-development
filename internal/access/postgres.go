package access

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PGLevelSource struct {
	pool *pgxpool.Pool
}

func NewPGLevelSource(pool *pgxpool.Pool) *PGLevelSource {
	return &PGLevelSource{pool: pool}
}

// Levels mengambil semua level grant yang dimiliki user pada koneksi, melalui
// group tempat user menjadi anggota. User nonaktif tidak mendapat level apa pun.
func (s *PGLevelSource) Levels(ctx context.Context, userID int64, connID int64) ([]Action, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT g.level
		   FROM memberships m
		   JOIN grants g ON g.group_id = m.group_id
		   JOIN users u  ON u.id = m.user_id
		  WHERE m.user_id = $1
		    AND g.connection_id = $2
		    AND u.is_active`, userID, connID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var levels []Action
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if a := Action(raw); Rank(a) > 0 {
			levels = append(levels, a)
		}
	}
	return levels, rows.Err()
}
