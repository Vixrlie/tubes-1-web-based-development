package dbm

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// pageSizeBaris adalah jumlah baris per halaman data viewer.
const pageSizeBaris = 50

// schemaGroup adalah satu schema beserta tabelnya untuk pohon di halaman schema.
type schemaGroup struct {
	Name   string
	Tables []TableInfo
}

// selectedTable adalah tabel terpilih di penjelajah schema.
type selectedTable struct {
	TableInfo
	Columns []ColumnInfo
}

// databasesPage adalah data halaman daftar database (dbm/databases.html).
type databasesPage struct {
	web.AppShell
	Groups []groupName
}

// groupName hanya untuk kartu keanggotaan user.
type groupName struct {
	Name string
}

// groupsForUser membaca nama-nama group yang dianggotei user (untuk kartu keanggotaan).
func (h *Handlers) groupsForUser(r *http.Request, userID int64) []groupName {
	rows, err := h.platform.Query(r.Context(),
		`SELECT g.name
		   FROM memberships m
		   JOIN groups g ON g.id = m.group_id
		  WHERE m.user_id = $1
		  ORDER BY g.name`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []groupName
	for rows.Next() {
		var g groupName
		if err := rows.Scan(&g.Name); err != nil {
			return nil
		}
		out = append(out, g)
	}
	return out
}

// list menampilkan database yang boleh dibuka user (sidebar + kartu keanggotaan).
func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	me, _ := auth.UserFromContext(r.Context())
	shell := h.pageShell(r, "Daftar database", 0, "databases")
	shell.Breadcrumbs = []web.Crumb{{Label: "Databases"}}
	h.render(w, http.StatusOK, "dbm/databases.html", databasesPage{
		AppShell: shell,
		Groups:   h.groupsForUser(r, me.ID),
	})
}

// summary mengarahkan /databases/{id} ke penjelajah schema.
func (h *Handlers) summary(w http.ResponseWriter, r *http.Request) {
	connID, ok := parseConnID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if _, allowed := h.requireAccess(w, r, connID, access.ActionView); !allowed {
		return
	}
	http.Redirect(w, r, "/databases/"+strconv.FormatInt(connID, 10)+"/schema", http.StatusSeeOther)
}

// schemaPage adalah data penjelajah schema (dbm/schema.html).
type schemaPage struct {
	web.AppShell
	Connection web.DatabaseLink
	CanEdit    bool
	Schemas    []schemaGroup
	Selected   *selectedTable
	Notice     string
	Error      string
}

// schema menampilkan pohon schema/tabel dan pratinjau kolom tabel terpilih.
func (h *Handlers) schema(w http.ResponseWriter, r *http.Request) {
	connID, ok := parseConnID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	conn, allowed := h.requireAccess(w, r, connID, access.ActionView)
	if !allowed {
		return
	}
	pool, ok := h.targetPool(w, r, conn)
	if !ok {
		return
	}

	page := schemaPage{
		AppShell:  dbCrumb(h.pageShell(r, "Penjelajah Schema", connID, "databases"), conn, "Schema"),
		Connection: conn,
		CanEdit:    conn.Level == "edit",
		Notice:     migrationNotices[r.URL.Query().Get("status")],
	}

	tables, err := ListSchemasTables(r.Context(), pool)
	if err != nil {
		page.Error = msgIntrospectFailed
		h.render(w, http.StatusOK, "dbm/schema.html", page)
		return
	}

	// Kelompokkan per schema, pertahankan urutan abjad dari query.
	bySchema := map[string][]TableInfo{}
	var schemaOrder []string
	for _, t := range tables {
		if _, seen := bySchema[t.Schema]; !seen {
			schemaOrder = append(schemaOrder, t.Schema)
		}
		bySchema[t.Schema] = append(bySchema[t.Schema], t)
	}
	for _, s := range schemaOrder {
		page.Schemas = append(page.Schemas, schemaGroup{Name: s, Tables: bySchema[s]})
	}

	// Tabel terpilih lewat ?table=schema.name. Validasi dengan lookup catalog
	// (tabel harus ada di hasil introspeksi) sebelum dipakai query kolom.
	if raw := r.URL.Query().Get("table"); raw != "" {
		schemaName, tableName, found := strings.Cut(raw, ".")
		if found && schemaName != "" && tableName != "" && !strings.Contains(tableName, ".") {
			var info *TableInfo
			for i := range tables {
				if tables[i].Schema == schemaName && tables[i].Name == tableName {
					info = &tables[i]
					break
				}
			}
			if info == nil {
				http.NotFound(w, r)
				return
			}
			cols, err := ListColumns(r.Context(), pool, schemaName, tableName)
			if err != nil {
				page.Error = msgIntrospectFailed
			} else {
				page.Selected = &selectedTable{TableInfo: *info, Columns: cols}
			}
		}
	}

	h.render(w, http.StatusOK, "dbm/schema.html", page)
}

// tablePage adalah data detail tabel (dbm/table.html).
type tablePage struct {
	web.AppShell
	Connection  web.DatabaseLink
	Schema      string
	Table       string
	Tab         string
	TabTitle    string
	ItemCount   int
	Columns     []ColumnInfo
	Constraints []ConstraintInfo
	Indexes     []IndexInfo
	Error       string
}

// table menampilkan columns/constraints/indexes sesuai ?tab=.
func (h *Handlers) table(w http.ResponseWriter, r *http.Request) {
	connID, ok := parseConnID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	conn, allowed := h.requireAccess(w, r, connID, access.ActionView)
	if !allowed {
		return
	}
	pool, ok := h.targetPool(w, r, conn)
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

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "columns"
	}

	page := tablePage{
		AppShell:  dbCrumb(h.pageShell(r, tableName, connID, "databases"), conn, tableName),
		Connection: conn,
		Schema:     schemaName,
		Table:      tableName,
		Tab:        tab,
	}

	switch tab {
	case "constraints":
		page.TabTitle = "Constraints"
		page.Constraints, err = ListConstraints(r.Context(), pool, schemaName, tableName)
	case "indexes":
		page.TabTitle = "Indexes"
		page.Indexes, err = ListIndexes(r.Context(), pool, schemaName, tableName)
	default:
		tab = "columns"
		page.Tab = tab
		page.TabTitle = "Columns"
		page.Columns, err = ListColumns(r.Context(), pool, schemaName, tableName)
	}
	if err != nil {
		page.Error = msgIntrospectFailed
	}
	page.ItemCount = len(page.Columns) + len(page.Constraints) + len(page.Indexes)

	h.render(w, http.StatusOK, "dbm/table.html", page)
}

// tableDataPage adalah data halaman data viewer (dbm/table_data.html).
type tableDataPage struct {
	web.AppShell
	Connection web.DatabaseLink
	Schema     string
	Table      string
	Columns    []string
	Rows       [][]string
	Page       int
	TotalPages int
	HasPrev    bool
	HasNext    bool
	PrevPage   int
	NextPage   int
	Error      string
}

// tableData menampilkan isi tabel dengan paginasi (hanya-baca).
func (h *Handlers) tableData(w http.ResponseWriter, r *http.Request) {
	connID, ok := parseConnID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	conn, allowed := h.requireAccess(w, r, connID, access.ActionView)
	if !allowed {
		return
	}
	pool, ok := h.targetPool(w, r, conn)
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

	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			page = n
		}
	}

	dataPage := tableDataPage{
		AppShell:  dbCrumb(h.pageShell(r, tableName, connID, "databases"), conn, tableName),
		Connection: conn,
		Schema:     schemaName,
		Table:      tableName,
		Page:       page,
	}

	total, err := CountRows(r.Context(), pool, schemaName, tableName)
	if err != nil {
		dataPage.Error = msgIntrospectFailed
		h.render(w, http.StatusOK, "dbm/table_data.html", dataPage)
		return
	}

	totalPages := int((total + pageSizeBaris - 1) / pageSizeBaris)
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
		dataPage.Page = page
	}
	dataPage.TotalPages = totalPages
	dataPage.HasPrev = page > 1
	dataPage.HasNext = page < totalPages
	dataPage.PrevPage = page - 1
	dataPage.NextPage = page + 1

	offset := (page - 1) * pageSizeBaris
	columns, rows, err := ListRows(r.Context(), pool, schemaName, tableName, pageSizeBaris, offset)
	if err != nil {
		dataPage.Error = msgIntrospectFailed
	}
	dataPage.Columns = columns
	dataPage.Rows = rows

	h.render(w, http.StatusOK, "dbm/table_data.html", dataPage)
}

// splitTableParam memecah "schema.tabel" dari URL. false bila format salah.
func splitTableParam(param string) (schemaName, tableName string, ok bool) {
	schemaName, tableName, found := strings.Cut(param, ".")
	if !found || schemaName == "" || tableName == "" {
		return "", "", false
	}
	if strings.Contains(tableName, ".") {
		return "", "", false
	}
	return schemaName, tableName, true
}
