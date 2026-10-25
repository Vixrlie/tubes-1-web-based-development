package dbm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

type Config struct {
	Platform *pgxpool.Pool // baca db_connections
	Policy   access.Policy
	Registry Registry
	Renderer auth.Renderer
	// Migrations menyimpan & membaca riwayat migration (opsional; default
	// PGMigrationStore di atas Platform).
	Migrations MigrationStore
}

// Handlers melayani userflow database (daftar, schema, detail tabel). Semua route
// memeriksa izin via access.Policy; penolakan 403 polos, karena FE belum punya halaman error.
type Handlers struct {
	platform   *pgxpool.Pool
	policy     access.Policy
	registry   Registry
	renderer   auth.Renderer
	migrations MigrationStore
	runner     *MigrationRunner
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
	migrations := cfg.Migrations
	if migrations == nil && cfg.Platform != nil {
		migrations = NewPGMigrationStore(cfg.Platform)
	}
	h := &Handlers{
		platform:   cfg.Platform,
		policy:     policy,
		registry:   registry,
		renderer:   cfg.Renderer,
		migrations: migrations,
	}
	if cfg.Platform != nil {
		h.runner = NewMigrationRunner(cfg.Platform, registry)
	}
	return h
}

// Routes mendaftarkan route userflow /databases/* pada grup terproteksi
// (AuthMiddleware + CSRFProtect sudah terpasang di main).
func (h *Handlers) Routes(r chi.Router) {
	r.Route("/databases", func(r chi.Router) {
		r.Get("/", h.list)
		r.Get("/{connID}", h.summary)
		r.Get("/{connID}/schema", h.schema)
		r.Get("/{connID}/tables/{table}", h.tableDetail)
		r.Get("/{connID}/tables/{table}/data", h.tableData)
		// Halaman migration: riwayat, editor (draft), detail, preview & eksekusi.
		r.Get("/{connID}/migrations", h.migrationsList)
		r.Post("/{connID}/migrations", h.migrationsCreate)
		// Diff: canonical sekeluarga /migrations/* + alias pendek /diff.
		r.Get("/{connID}/migrations/diff", h.diffForm)
		r.Post("/{connID}/migrations/diff", h.diffRun)
		r.Get("/{connID}/diff", h.diffForm)
		r.Post("/{connID}/diff", h.diffRun)
		r.Get("/{connID}/migrations/new", h.migrationEditor)
		r.Get("/{connID}/migrations/{mID}", h.migrationDetail)
		r.Get("/{connID}/migrations/{mID}/preview", h.migrationPreview)
		r.Post("/{connID}/migrations/{mID}/explain", h.migrationExplain)
		r.Post("/{connID}/migrations/{mID}/execute", h.migrationExecute)
	})
}

// authorize memeriksa izin view user pada {connID} DI DALAM handler
// (param route pasti terisi pada titik ini). false = respons sudah dikirim
// (404 untuk id tak valid, 403 untuk izin ditolak/gagal dimuat; fail closed).
func (h *Handlers) authorize(w http.ResponseWriter, r *http.Request) (int64, bool) {
	return h.authorizeAction(w, r, access.ActionView)
}

// authorizeAction = authorize dengan aksi tertentu (edit untuk operasi tulis
// migration: buat draft & eksekusi).
func (h *Handlers) authorizeAction(w http.ResponseWriter, r *http.Request, action access.Action) (int64, bool) {
	connID, err := strconv.ParseInt(chi.URLParam(r, "connID"), 10, 64)
	if err != nil || connID <= 0 {
		http.NotFound(w, r)
		return 0, false
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return 0, false
	}
	allowed, err := h.policy.Can(r.Context(), user.ID, connID, action)
	if err != nil || !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return 0, false
	}
	return connID, true
}

// Helpers shell & data

func shellUser(ctx context.Context) *web.ShellUser {
	me, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil
	}
	return &web.ShellUser{
		Username:        me.Username,
		IsPlatformAdmin: me.IsPlatformAdmin,
		CSRFToken:       session.CSRFToken(ctx),
	}
}

// formatRows memformat angka dengan pemisah titik gaya Indonesia (1.240).
func formatRows(n int64) string {
	if n < 0 {
		return "-"
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// connectionName mengambil nama koneksi tampilan dari platform.
func (h *Handlers) connectionName(ctx context.Context, connID int64) (string, error) {
	var name string
	err := h.platform.QueryRow(ctx,
		`SELECT name FROM db_connections WHERE id = $1`, connID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrConnNotFound
	}
	return name, err
}

// databaseShell membangun shell FE halaman database (Nav "database" + sidebar
// tree schema/tabel dari database target; aktif = "schema.tabel" yang dibuka).
func (h *Handlers) databaseShell(ctx context.Context, connID int64, connName, active string) web.Shell {
	sh := web.Shell{Nav: "database", User: shellUser(ctx)}
	pool, err := h.registry.Get(ctx, connID)
	if err != nil || pool == nil {
		return sh // sidebar minimal saat database tak terhubung
	}
	sums, err := ListTableSummaries(ctx, pool)
	if err != nil {
		return sh
	}

	activeSchema := ""
	if i := strings.IndexByte(active, '.'); i > 0 {
		activeSchema = active[:i]
	}
	var order []string
	bySchema := map[string][]web.SidebarTable{}
	for _, s := range sums {
		if _, seen := bySchema[s.Schema]; !seen {
			order = append(order, s.Schema)
		}
		id := s.Schema + "." + s.Name
		bySchema[s.Schema] = append(bySchema[s.Schema], web.SidebarTable{
			Name:   s.Name,
			Href:   fmt.Sprintf("/databases/%d/tables/%s", connID, id),
			Active: active == id,
		})
	}
	schemas := make([]web.SidebarSchema, 0, len(order))
	for _, name := range order {
		schemas = append(schemas, web.SidebarSchema{
			Name:   name,
			Href:   fmt.Sprintf("/databases/%d/schema", connID),
			Active: name == activeSchema,
			Tables: bySchema[name],
		})
	}
	sh.Sidebar = &web.SidebarDB{
		Name:             connName,
		Schemas:          schemas,
		MigrationsHref:   fmt.Sprintf("/databases/%d/migrations", connID),
		MigrationsActive: false,
	}
	return sh
}

// Daftar database

// list menampilkan kartu database yang boleh dibuka user (Access Control).
func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.platform.Query(r.Context(),
		`SELECT id, name, connection_url FROM db_connections ORDER BY name`)
	if err != nil {
		h.serverError(w, "daftar database", err)
		return
	}
	defer rows.Close()

	page := web.DatabasesPage{Shell: web.Shell{Nav: "databases", User: shellUser(r.Context())}}
	for rows.Next() {
		var id int64
		var name, rawURL string
		if err := rows.Scan(&id, &name, &rawURL); err != nil {
			h.serverError(w, "daftar database", err)
			return
		}
		// Hanya database yang punya izin tampil (AC-01 / UGA-02).
		allowed, err := h.policy.Can(r.Context(), userOrZero(r.Context()), id, access.ActionView)
		if err != nil || !allowed {
			continue
		}
		level := "view"
		if ok, err := h.policy.Can(r.Context(), userOrZero(r.Context()), id, access.ActionEdit); err == nil && ok {
			level = "edit"
		}
		card := web.DBCard{
			Name:   name,
			Access: level,
			Status: web.ConnConnected,
			Href:   fmt.Sprintf("/databases/%d/schema", id),
		}
		pool, err := h.registry.Get(r.Context(), id)
		if err != nil || pool == nil {
			card.Status = web.ConnFailed
			card.Error = "Tidak dapat terhubung ke database. Coba lagi beberapa saat."
			card.RetryHref = "/databases"
			card.Meta = schemeLabel(rawURL)
			page.Cards = append(page.Cards, card)
			continue
		}
		card.Meta = schemeLabel(rawURL)
		if n, err := countTables(r.Context(), pool); err == nil {
			card.Meta = fmt.Sprintf("%s · %d tabel", schemeLabel(rawURL), n)
		}
		page.Cards = append(page.Cards, card)
	}
	if err := rows.Err(); err != nil {
		h.serverError(w, "daftar database", err)
		return
	}
	h.render(w, http.StatusOK, "databases.html", page)
}

func userOrZero(ctx context.Context) int64 {
	me, ok := auth.UserFromContext(ctx)
	if !ok {
		return 0
	}
	return me.ID
}

func countTables(ctx context.Context, pool *Target) (int, error) {
	var n int
	err := pool.DB.QueryRowContext(ctx, pool.Dialect.Queries().CountTables).Scan(&n)
	return n, err
}

// summary mengarahkan /databases/{id} ke penjelajah schema (userflow).
func (h *Handlers) summary(w http.ResponseWriter, r *http.Request) {
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	http.Redirect(w, r, "/databases/"+strconv.FormatInt(connID, 10)+"/schema", http.StatusSeeOther)
}

func (h *Handlers) serverError(w http.ResponseWriter, what string, err error) {
	http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
	_ = what
	_ = err
}

// render merender halaman FE via layout (satu-satunya jalur render di cabang ini).
func (h *Handlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	h.renderer.Render(w, name, data)
}

// targetPool mengambil pool database target; gagal -> halaman 503 aman.
func (h *Handlers) targetPool(w http.ResponseWriter, r *http.Request, connID int64) (*Target, bool) {
	pool, err := h.registry.Get(r.Context(), connID)
	if err != nil || pool == nil {
		h.renderDbError(w)
		return nil, false
	}
	return pool, true
}

// renderDbError membalas kegagalan koneksi target tanpa membocorkan kredensial.
func (h *Handlers) renderDbError(w http.ResponseWriter) {
	http.Error(w, "database tidak dapat dihubungi", http.StatusServiceUnavailable)
}

// Penjelajah schema

// schemaPage adalah data halaman schema (templates/schema.html fe-components).
type schemaPage struct {
	web.Shell
	Title     string
	Crumbs    []web.Crumb
	Tables    web.Table
	Relations web.Table
}

func (h *Handlers) schema(w http.ResponseWriter, r *http.Request) {
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	name, err := h.connectionName(r.Context(), connID)
	if errors.Is(err, ErrConnNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, "nama koneksi", err)
		return
	}
	pool, ok := h.targetPool(w, r, connID)
	if !ok {
		return
	}
	sums, err := ListTableSummaries(r.Context(), pool)
	if err != nil {
		http.Error(w, "gagal membaca struktur database", http.StatusServiceUnavailable)
		return
	}

	// Satu schema → judul & crumb memakai namanya; banyak schema → netral.
	schemaNames := map[string]bool{}
	for _, s := range sums {
		schemaNames[s.Schema] = true
	}
	schemaLabel := "Schema"
	if len(schemaNames) == 1 {
		for n := range schemaNames {
			schemaLabel = "Schema " + n
		}
	}

	page := schemaPage{
		Shell: h.databaseShell(r.Context(), connID, name, ""),
		Title: schemaLabel,
		Crumbs: []web.Crumb{
			{Label: name, Href: "/databases"},
			{Label: strings.TrimPrefix(schemaLabel, "Schema ")},
		},
	}

	// Nama tabel disimpan "schema.tabel" supaya unik lintas schema.
	multi := len(schemaNames) > 1
	c := func(t string) web.TableCell { return web.TableCell{Text: t} }
	tables := web.Table{
		Title: "Daftar Tabel",
		Columns: []web.TableColumn{
			{Key: "name", Label: "Nama", Bold: true},
			{Key: "cols", Label: "Kolom"},
			{Key: "rows", Label: "Perkiraan Baris"},
			{Key: "pk", Label: "Primary Key"},
		},
		Empty: "Belum ada tabel di database ini.",
	}
	for _, s := range sums {
		label := s.Name
		if multi {
			label = s.Schema + "." + s.Name
		}
		pk := s.PK
		if pk == "" {
			pk = "-"
		}
		tables.Rows = append(tables.Rows, web.TableRow{
			"name": {Text: label, Href: fmt.Sprintf("/databases/%d/tables/%s.%s", connID, s.Schema, s.Name)},
			"cols": c(strconv.Itoa(s.Columns)),
			"rows": c(formatRows(s.RowEstimate)),
			"pk":   c(pk),
		})
	}
	page.Tables = tables

	rels := web.Table{
		Title: "Relasi (Foreign Key)",
		Columns: []web.TableColumn{
			{Key: "name", Label: "Nama"},
			{Key: "from", Label: "Dari"},
			{Key: "to", Label: "Mengacu ke"},
		},
		Empty: "Tidak ada relasi foreign key.",
	}
	if fks, err := ListForeignKeys(r.Context(), pool); err == nil {
		for _, f := range fks {
			rels.Rows = append(rels.Rows, web.TableRow{
				"name": c(f.Name),
				"from": c(f.From),
				"to":   {Text: f.ToRef, Variant: "info"},
			})
		}
	}
	page.Relations = rels

	h.render(w, http.StatusOK, "schema.html", page)
}

// Detail tabel

// tablePage adalah data detail tabel (templates/table_detail.html fe-components).
type tablePage struct {
	web.Shell
	Title  string
	Crumbs []web.Crumb
	Tabs   []web.TabItem
	Tables []web.Table
}

func (h *Handlers) tableDetail(w http.ResponseWriter, r *http.Request) {
	h.renderTable(w, r, r.URL.Query().Get("tab"))
}

// tableData = rute userflow /tables/{t}/data (tab data tanpa parameter).
func (h *Handlers) tableData(w http.ResponseWriter, r *http.Request) {
	h.renderTable(w, r, "data")
}

func (h *Handlers) renderTable(w http.ResponseWriter, r *http.Request, tab string) {
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	name, err := h.connectionName(r.Context(), connID)
	if errors.Is(err, ErrConnNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, "nama koneksi", err)
		return
	}
	pool, ok := h.targetPool(w, r, connID)
	if !ok {
		return
	}
	schemaName, tableName, ok := splitTableParam(chi.URLParam(r, "table"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	valid, err := validateTable(r.Context(), pool, schemaName, tableName)
	if err != nil {
		h.serverError(w, "validasi tabel", err)
		return
	}
	if !valid {
		http.NotFound(w, r)
		return
	}
	if tab != "constraints" && tab != "data" {
		tab = "columns"
	}

	schemaHref := fmt.Sprintf("/databases/%d/schema", connID)
	page := tablePage{
		Shell: h.databaseShell(r.Context(), connID, name, schemaName+"."+tableName),
		Title: "Tabel " + tableName,
		Crumbs: []web.Crumb{
			{Label: name, Href: "/databases"},
			{Label: schemaName, Href: schemaHref},
			{Label: tableName},
		},
		Tabs: web.NewTableTabs(tab, func(id string) string { return "?tab=" + id }),
	}

	switch tab {
	case "constraints":
		page.Tables = constraintTables(r, pool, schemaName, tableName)
	case "data":
		page.Tables = dataTables(r, w, pool, connID, schemaName, tableName, tab)
	default:
		page.Tables = columnTable(r, pool, schemaName, tableName)
	}
	h.render(w, http.StatusOK, "table_detail.html", page)
}

func columnTable(r *http.Request, pool *Target, schemaName, tableName string) []web.Table {
	t := web.Table{
		Title: "Column",
		Columns: []web.TableColumn{
			{Key: "name", Label: "Nama"},
			{Key: "type", Label: "Tipe"},
			{Key: "nullable", Label: "Nullable"},
			{Key: "default", Label: "Default"},
		},
		Empty: "Belum ada kolom.",
	}
	cols, err := ListColumns(r.Context(), pool, schemaName, tableName)
	if err != nil {
		return []web.Table{t}
	}
	for _, col := range cols {
		def := "-"
		if col.Default != nil && *col.Default != "" {
			def = *col.Default
		}
		nullable := "NO"
		if col.Nullable {
			nullable = "YES"
		}
		t.Rows = append(t.Rows, web.TableRow{
			"name":     {Text: col.Name},
			"type":     {Text: col.Type},
			"nullable": {Text: nullable},
			"default":  {Text: def},
		})
	}
	return []web.Table{t}
}

func constraintTables(r *http.Request, pool *Target, schemaName, tableName string) []web.Table {
	kindVariant := func(kind string) string {
		switch kind {
		case "PRIMARY KEY":
			return "info"
		case "UNIQUE":
			return "neutral"
		default:
			return ""
		}
	}
	cons := web.Table{
		Title: "Constraint",
		Columns: []web.TableColumn{
			{Key: "name", Label: "Nama"},
			{Key: "kind", Label: "Jenis"},
			{Key: "cols", Label: "Kolom"},
		},
		Empty: "Belum ada constraint.",
	}
	idx := web.Table{
		Title: "Index",
		Columns: []web.TableColumn{
			{Key: "name", Label: "Nama"},
			{Key: "cols", Label: "Kolom"},
			{Key: "unique", Label: "Unik"},
		},
		Empty: "Belum ada index.",
	}

	consList, err := ListConstraints(r.Context(), pool, schemaName, tableName)
	if err == nil {
		for _, k := range consList {
			cons.Rows = append(cons.Rows, web.TableRow{
				"name": {Text: k.Name},
				"kind": {Text: k.Type, Variant: kindVariant(k.Type)},
				"cols": {Text: k.Columns},
			})
		}
	}
	idxList, err := ListIndexes(r.Context(), pool, schemaName, tableName)
	if err == nil {
		for _, i := range idxList {
			unique := "Tidak"
			if i.Unique {
				unique = "Ya"
			}
			idx.Rows = append(idx.Rows, web.TableRow{
				"name":   {Text: i.Name},
				"cols":   {Text: i.Columns},
				"unique": {Text: unique},
			})
		}
	}
	return []web.Table{cons, idx}
}

// dataPageSize = jumlah baris per halaman data viewer.
const dataPageSize = 20

func dataTables(r *http.Request, w http.ResponseWriter, pool *Target, connID int64, schemaName, tableName, tab string) []web.Table {
	t := web.Table{
		Title: "Isi table",
		Tag:   web.TableCell{Text: "View-only", Variant: "neutral"},
		Empty: "Tabel kosong.",
	}

	total, err := CountRows(r.Context(), pool, schemaName, tableName)
	if err != nil {
		return []web.Table{t}
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			page = n
		}
	}
	totalPages := int((total + dataPageSize - 1) / dataPageSize)
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * dataPageSize

	columns, rows, err := ListRows(r.Context(), pool, schemaName, tableName, dataPageSize, offset)
	if err != nil {
		return []web.Table{t}
	}
	for _, col := range columns {
		t.Columns = append(t.Columns, web.TableColumn{Key: col, Label: col})
	}
	for _, row := range rows {
		line := web.TableRow{}
		for i, col := range columns {
			line[col] = web.TableCell{Text: row[i]}
		}
		t.Rows = append(t.Rows, line)
	}

	if total > 0 {
		start := offset + 1
		end := offset + len(rows)
		t.Pager = web.NewPager(
			fmt.Sprintf("Menampilkan %d–%d dari %s baris", start, end, formatRows(total)),
			page, totalPages,
			func(p int) string { return fmt.Sprintf("?tab=%s&page=%d", tab, p) },
		)
	}
	return []web.Table{t}
}

// splitTableParam memecah "schema.tabel" dari URL. false bila format salah.
func splitTableParam(param string) (schemaName, tableName string, ok bool) {
	schemaName, tableName, found := strings.Cut(param, ".")
	if !found || schemaName == "" || tableName == "" || strings.Contains(tableName, ".") {
		return "", "", false
	}
	return schemaName, tableName, true
}
