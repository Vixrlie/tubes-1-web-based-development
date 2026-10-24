package dbm

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// AppRenderer merender halaman app shell (sama dengan antarmuka di admin).
type AppRenderer interface {
	RenderApp(w http.ResponseWriter, name string, data any)
}

// Config adalah dependensi handlers database management.
type Config struct {
	// Platform adalah pool database platform (baca db_connections dan migration_history).
	Platform *pgxpool.Pool
	// Policy menentukan izin user per koneksi; wajib diisi (bila nil, DenyAll).
	Policy access.Policy
	// Registry menyediakan pool database target per connID.
	Registry Registry
	// Renderer merender template dbm/*.
	Renderer AppRenderer
	// Shell menyuplai daftar database untuk sidebar app shell.
	Shell web.ShellStore
	// Store membaca dan menyimpan migration di database platform.
	Store MigrationStore
}

// Handlers menangani fitur database management. Semua route yang mengakses
// database target memeriksa izin lewat access.Policy di backend.
type Handlers struct {
	platform *pgxpool.Pool
	policy   access.Policy
	registry Registry
	renderer AppRenderer
	shell    web.ShellStore
	store    MigrationStore
	runner   *MigrationRunner
}

func NewHandlers(cfg Config) *Handlers {
	policy := cfg.Policy
	if policy == nil {
		policy = access.DenyAll{}
	}
	registry := cfg.Registry
	if registry == nil {
		registry = StubRegistry{}
	}
	store := cfg.Store
	if store == nil && cfg.Platform != nil {
		store = NewPGMigrationStore(cfg.Platform)
	}
	var runner *MigrationRunner
	if cfg.Platform != nil {
		runner = NewMigrationRunner(cfg.Platform, registry)
	}
	return &Handlers{
		platform: cfg.Platform,
		policy:   policy,
		registry: registry,
		renderer: cfg.Renderer,
		shell:    cfg.Shell,
		store:    store,
		runner:   runner,
	}
}

// Routes mendaftarkan route /databases/*. Rute ini dipasang di router
// protected (AuthMiddleware + CSRFProtect), jadi user sudah ada di context.
func (h *Handlers) Routes(r chi.Router) {
	r.Route("/databases", func(r chi.Router) {
		r.Get("/", h.list)
		r.Get("/{connID}", h.summary)
		r.Get("/{connID}/schema", h.schema)
		r.Get("/{connID}/tables/{table}", h.table)
		r.Get("/{connID}/tables/{table}/data", h.tableData)
		r.Get("/{connID}/migrations", h.migrationList)
		r.Get("/{connID}/migrations/new", h.migrationNew)
		r.Post("/{connID}/migrations", h.migrationCreate)
		r.Get("/{connID}/migrations/{mid}", h.migrationDetail)
		r.Get("/{connID}/migrations/{mid}/edit", h.migrationEdit)
		r.Post("/{connID}/migrations/{mid}", h.migrationUpdate)
		r.Get("/{connID}/migrations/{mid}/preview", h.migrationPreview)
		r.Post("/{connID}/migrations/{mid}/execute", h.migrationExecute)
	})
}

// --- helper bersama ---------------------------------------------------------

func (h *Handlers) serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("dbm: %s: %v", what, err) // jangan pernah log kredensial
	http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
}

func (h *Handlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	h.renderer.RenderApp(w, name, data)
}

// shell merakit app shell untuk halaman dbm.
func (h *Handlers) pageShell(r *http.Request, title string, currentDB int64, activeNav string) web.AppShell {
	me, _ := auth.UserFromContext(r.Context())
	return web.BuildShell(r.Context(), h.shell, web.ShellOptions{
		Title:                title,
		CurrentDB:            currentDB,
		ActiveNav:            activeNav,
		CanManageUsers:       me.IsPlatformAdmin,
		CanManageConnections: me.IsPlatformAdmin,
	})
}

// dbCrumb menetapkan rantai breadcrumb halaman database:
// Databases > {nama database} > {halaman saat ini}.
func dbCrumb(sh web.AppShell, conn web.DatabaseLink, leaf string) web.AppShell {
	sh.Breadcrumbs = []web.Crumb{
		{Label: "Databases", Href: "/databases"},
		{Label: conn.Name, Href: "/databases/" + strconv.FormatInt(conn.ID, 10) + "/schema"},
		{Label: leaf},
	}
	return sh
}

// parseConnID membaca {connID} dari URL. false bila tidak valid.
func parseConnID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "connID"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// loadConnection mengambil info tampilan koneksi (tanpa connection_url).
// Level diisi dari keputusan policy terhadap user yang sedang login.
func (h *Handlers) loadConnection(ctx context.Context, userID, connID int64) (web.DatabaseLink, error) {
	var link web.DatabaseLink
	var rawURL string
	err := h.platform.QueryRow(ctx,
		`SELECT id, name, COALESCE(description, ''), connection_url
		   FROM db_connections WHERE id = $1`, connID).
		Scan(&link.ID, &link.Name, &link.Description, &rawURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return web.DatabaseLink{}, ErrConnNotFound
	}
	if err != nil {
		return web.DatabaseLink{}, err
	}
	link.HostPort = hostPort(rawURL)
	link.Level = "view"
	if ok, err := h.policy.Can(ctx, userID, connID, access.ActionEdit); err == nil && ok {
		link.Level = "edit"
	}
	return link, nil
}

// accessPage adalah data halaman 403 (dbm/error_403.html).
type accessPage struct {
	web.AppShell
	ConnectionName  string
	EffectiveLevel  string
	RequestedAction string
	Action          string
}

// deny merender halaman 403 dengan level akses efektif user.
func (h *Handlers) deny(w http.ResponseWriter, r *http.Request, connID int64, action access.Action) {
	connName := "database"
	if h.platform != nil {
		var n string
		if err := h.platform.QueryRow(r.Context(),
			`SELECT name FROM db_connections WHERE id = $1`, connID).Scan(&n); err == nil {
			connName = n
		}
	}

	level := "Tidak ada akses"
	me, _ := auth.UserFromContext(r.Context())
	if me.ID != 0 && h.platform != nil {
		if ok, err := h.policy.Can(r.Context(), me.ID, connID, access.ActionEdit); err == nil && ok {
			level = "View + Edit"
		} else if ok, err := h.policy.Can(r.Context(), me.ID, connID, access.ActionView); err == nil && ok {
			level = "View Only"
		}
	}

	label := "Lihat schema, data, dan riwayat migration"
	requested := "View Only"
	if action == access.ActionEdit {
		label = "Membuat, mengubah, atau menjalankan migration"
		requested = "View + Edit"
	}

	shell := dbCrumb(h.pageShell(r, "Akses dibatasi", connID, "databases"),
		web.DatabaseLink{ID: connID, Name: connName}, "Akses dibatasi")
	h.render(w, http.StatusForbidden, "dbm/error_403.html", accessPage{
		AppShell:        shell,
		ConnectionName:  connName,
		EffectiveLevel:  level,
		RequestedAction: requested,
		Action:          label,
	})
}

// dbErrorPage adalah data halaman koneksi gagal (dbm/error_db.html).
type dbErrorPage struct {
	web.AppShell
	ConnectionName string
	CurrentDB      int64
}

// requireAccess memeriksa izin user untuk sebuah aksi pada koneksi.
// Mengembalikan false (dan sudah merender respons) bila ditolak atau koneksi
// tidak ditemukan. Ini satu-satunya gerbang izin untuk semua route dbm.
func (h *Handlers) requireAccess(w http.ResponseWriter, r *http.Request, connID int64, action access.Action) (web.DatabaseLink, bool) {
	me, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return web.DatabaseLink{}, false
	}

	allowed, err := h.policy.Can(r.Context(), me.ID, connID, action)
	if err != nil {
		h.serverError(w, "periksa izin", err)
		return web.DatabaseLink{}, false
	}
	if !allowed {
		h.deny(w, r, connID, action)
		return web.DatabaseLink{}, false
	}

	conn, err := h.loadConnection(r.Context(), me.ID, connID)
	if errors.Is(err, ErrConnNotFound) {
		http.NotFound(w, r)
		return web.DatabaseLink{}, false
	}
	if err != nil {
		h.serverError(w, "ambil koneksi", err)
		return web.DatabaseLink{}, false
	}
	return conn, true
}

// renderDBError merender halaman "koneksi gagal" yang aman (tanpa kredensial).
func (h *Handlers) renderDBError(w http.ResponseWriter, r *http.Request, conn web.DatabaseLink, err error) {
	log.Printf("dbm: koneksi target %d gagal: %v", conn.ID, err) // URL tidak ikut log
	shell := dbCrumb(h.pageShell(r, "Koneksi gagal", conn.ID, "databases"), conn, "Koneksi gagal")
	h.render(w, http.StatusServiceUnavailable, "dbm/error_db.html", dbErrorPage{
		AppShell:        shell,
		ConnectionName:  conn.Name,
		CurrentDB:       conn.ID,
	})
}

// targetPool mengambil pool database target, dengan pesan gagal yang aman.
func (h *Handlers) targetPool(w http.ResponseWriter, r *http.Request, conn web.DatabaseLink) (*pgxpool.Pool, bool) {
	pool, err := h.registry.Get(r.Context(), conn.ID)
	if err != nil || pool == nil {
		h.renderDBError(w, r, conn, err)
		return nil, false
	}
	return pool, true
}

// dbPageError adalah kesalahan yang menghalangi render halaman database
// (gagal membaca schema dsb.) — dirender sebagai alert di halaman itu sendiri.
const msgIntrospectFailed = "Gagal membaca struktur dari database. Database mungkin sedang tidak dapat dihubungi."
