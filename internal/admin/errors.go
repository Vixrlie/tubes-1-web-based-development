package admin

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// constraintOf mengembalikan nama constraint dari error PostgreSQL, atau ""
// bila error bukan pelanggaran constraint. Dipakai bersama oleh store admin.
func constraintOf(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}
