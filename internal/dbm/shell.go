package dbm

import (
	"context"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// PGShellStore membaca langsung dari tabel platform. Akses efektif diambil
// dari level tertinggi di seluruh group yang dianggotai user.
type PGShellStore struct {
	pool *pgxpool.Pool
}

func NewPGShellStore(pool *pgxpool.Pool) *PGShellStore {
	return &PGShellStore{pool: pool}
}

const listForUserSQL = `
SELECT dc.id, dc.name, COALESCE(dc.description, ''), dc.connection_url,
       BOOL_OR(g.level = 'edit') AS can_edit
  FROM memberships m
  JOIN grants g           ON g.group_id = m.group_id
  JOIN db_connections dc  ON dc.id = g.connection_id
 WHERE m.user_id = $1
 GROUP BY dc.id, dc.name, dc.description, dc.connection_url
 ORDER BY dc.name`

func (s *PGShellStore) ListForUser(ctx context.Context, userID int64) ([]web.DatabaseLink, error) {
	rows, err := s.pool.Query(ctx, listForUserSQL, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []web.DatabaseLink
	for rows.Next() {
		var link web.DatabaseLink
		var rawURL string
		var canEdit bool
		if err := rows.Scan(&link.ID, &link.Name, &link.Description, &rawURL, &canEdit); err != nil {
			return nil, err
		}
		link.HostPort = hostPort(rawURL)
		link.Level = "view"
		if canEdit {
			link.Level = "edit"
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// hostPort mengubah connection_url menjadi host:port untuk ditampilkan.
// Connection URL itu sendiri tidak pernah keluar ke template.
func hostPort(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if host == "" {
		return ""
	}
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	return host + ":" + port
}
