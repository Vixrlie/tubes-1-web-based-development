package dbm

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

// Handlers menangani fitur database management dengan periksa izin berbasis
// access.Policy. Semua route yang mengakses target DB harus lewat helper ini.
type Handlers struct {
	policy  access.Policy
	registry Registry
}

// NewHandlers membangun DBM handler dengan policy yang sudah dipasang.
func NewHandlers(policy access.Policy, registry Registry) *Handlers {
	if policy == nil {
		policy = access.DenyAll{}
	}
	if registry == nil {
		registry = StubRegistry{}
	}
	return &Handlers{policy: policy, registry: registry}
}

// Routes mendaftarkan route DBM. Rute ini dipasang di router protected setelah
// AuthMiddleware, sehingga user sudah ada di context.
func (h *Handlers) Routes(r chi.Router) {
	r.Route("/db/{connID}", func(r chi.Router) {
		r.Use(RequireAccess(h.policy, access.ActionView))
		r.Get("/browser", h.browser)
		r.Get("/query", h.query)
		r.Get("/schema", h.schema)
	})

	r.Route("/db/{connID}/migrations", func(r chi.Router) {
		r.Use(RequireAccess(h.policy, access.ActionEdit))
		r.Get("/", h.migrationList)
		r.Post("/apply", h.migrationApply)
	})
}

// RequireAccess adalah middleware umum untuk memeriksa hak akses pada koneksi target.
func RequireAccess(policy access.Policy, action access.Action) func(http.Handler) http.Handler {
	if policy == nil {
		policy = access.DenyAll{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := auth.UserFromContext(r.Context())
			if !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			connID, err := strconv.ParseInt(chi.URLParam(r, "connID"), 10, 64)
			if err != nil || connID <= 0 {
				http.NotFound(w, r)
				return
			}

			allowed, err := policy.Can(r.Context(), user.ID, connID, action)
			if err != nil {
				log.Printf("dbm access check failed: %v", err)
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if !allowed {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (h *Handlers) browser(w http.ResponseWriter, r *http.Request) {
	pool, err := h.getPool(r.Context(), w, r)
	if err != nil {
		return
	}

	var dbName, schemaName, versionInfo string
	if err := pool.QueryRow(r.Context(), `SELECT current_database(), current_schema(), version()`).Scan(&dbName, &schemaName, &versionInfo); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	out := fmt.Sprintf("database: %s\nschema: %s\nversion: %s", dbName, schemaName, versionInfo)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(out))
}

func (h *Handlers) query(w http.ResponseWriter, r *http.Request) {
	pool, err := h.getPool(r.Context(), w, r)
	if err != nil {
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("sql"))
	if query == "" {
		query = "SELECT 1 AS ok;"
	}
	rows, err := pool.Query(r.Context(), query)
	if err != nil {
		http.Error(w, "query failed", http.StatusBadRequest)
		return
	}
	defer rows.Close()

	var first []any
	if rows.Next() {
		first, err = rows.Values()
		if err != nil && err != pgx.ErrNoRows {
			http.Error(w, "query failed", http.StatusBadRequest)
			return
		}
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "query failed", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if len(first) == 0 {
		_, _ = w.Write([]byte("query ok"))
		return
	}
	_, _ = w.Write([]byte(fmt.Sprintf("%v", first)))
}

func (h *Handlers) schema(w http.ResponseWriter, r *http.Request) {
	pool, err := h.getPool(r.Context(), w, r)
	if err != nil {
		return
	}

	rows, err := pool.Query(r.Context(), `SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() ORDER BY table_name`)
	if err != nil {
		http.Error(w, "schema unavailable", http.StatusServiceUnavailable)
		return
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			http.Error(w, "schema unavailable", http.StatusServiceUnavailable)
			return
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "schema unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if len(tables) == 0 {
		_, _ = w.Write([]byte("schema: no tables"))
		return
	}
	_, _ = w.Write([]byte("schema: " + strings.Join(tables, ", ")))
}

func (h *Handlers) migrationList(w http.ResponseWriter, r *http.Request) {
	pool, err := h.getPool(r.Context(), w, r)
	if err != nil {
		return
	}

	rows, err := pool.Query(r.Context(), `SELECT version, status FROM migration_history ORDER BY version`)
	if err != nil {
		// Some target DBs may have no migration history table yet; still return a
		// valid empty list in that case rather than crashing the app.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("migrations: none"))
		return
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var version int
		var status string
		if err := rows.Scan(&version, &status); err != nil {
			http.Error(w, "migration list unavailable", http.StatusServiceUnavailable)
			return
		}
		out = append(out, fmt.Sprintf("%d:%s", version, status))
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "migration list unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if len(out) == 0 {
		_, _ = w.Write([]byte("migrations: none"))
		return
	}
	_, _ = w.Write([]byte("migrations: " + strings.Join(out, ", ")))
}

func (h *Handlers) migrationApply(w http.ResponseWriter, r *http.Request) {
	pool, err := h.getPool(r.Context(), w, r)
	if err != nil {
		return
	}

	sql := strings.TrimSpace(r.FormValue("sql"))
	if sql == "" {
		sql = "SELECT 1 AS ok;"
	}
	rows, err := pool.Query(r.Context(), sql)
	if err != nil {
		http.Error(w, "migration apply failed", http.StatusBadRequest)
		return
	}
	rows.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("migration apply ok"))
}

func (h *Handlers) getPool(ctx context.Context, w http.ResponseWriter, r *http.Request) (*pgxpool.Pool, error) {
	connID, err := strconv.ParseInt(chi.URLParam(r, "connID"), 10, 64)
	if err != nil || connID <= 0 {
		http.NotFound(w, r)
		return nil, err
	}
	pool, err := h.registry.Get(ctx, connID)
	if err != nil || pool == nil {
		http.Error(w, "database connection unavailable", http.StatusServiceUnavailable)
		return nil, err
	}
	return pool, nil
}
