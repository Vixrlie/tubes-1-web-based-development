package dbm

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

type AppRenderer interface {
	RenderApp(w http.ResponseWriter, name string, data any)
}

type Config struct {
	Platform   *pgxpool.Pool
	Registry   Registry
	Policy     access.Policy
	Renderer   AppRenderer
	Shell      web.ShellStore
	Migrations MigrationStore
}

type Handlers struct {
	cfg    Config
	runner *MigrationRunner
}

func NewHandlers(cfg Config) *Handlers {
	return &Handlers{
		cfg:    cfg,
		runner: NewMigrationRunner(cfg.Platform, cfg.Registry),
	}
}

func (h *Handlers) Routes(r chi.Router) {
	r.Get("/databases", h.listDatabases)
	r.Get("/databases/{id}/schema", h.schemaPage)
	r.Get("/databases/{id}/tables/{t}", h.tablePage)
	r.Get("/databases/{id}/tables/{t}/data", h.tableDataPage)
	h.migrationRoutes(r)
}

type groupRow struct {
	ID   int64
	Name string
}

type databasesPage struct {
	web.AppShell
	Groups []groupRow
}

func (h *Handlers) shell(ctx context.Context, r *http.Request, currentDB int64, activeNav, title string) web.AppShell {
	return web.BuildShell(ctx, h.cfg.Shell, web.ShellOptions{
		Title:                title,
		CurrentDB:            currentDB,
		ActiveNav:            activeNav,
		CanManageUsers:       true,
		CanManageConnections: false,
	})
}

func (h *Handlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	h.cfg.Renderer.RenderApp(w, name, data)
}

func (h *Handlers) serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("dbm: %s: %v", what, err)
	http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
}

func (h *Handlers) listDatabases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me, _ := auth.UserFromContext(ctx)

	groups, err := h.listUserGroups(ctx, me.ID)
	if err != nil {
		h.serverError(w, "daftar group", err)
		return
	}

	page := databasesPage{
		AppShell: h.shell(ctx, r, 0, "databases", "Daftar database"),
		Groups:   groups,
	}
	h.render(w, http.StatusOK, "dbm/databases.html", page)
}

const listUserGroupsSQL = `
SELECT g.id, g.name
  FROM memberships m
  JOIN groups g ON g.id = m.group_id
 WHERE m.user_id = $1
 ORDER BY g.name`

func (h *Handlers) listUserGroups(ctx context.Context, userID int64) ([]groupRow, error) {
	rows, err := h.cfg.Platform.Query(ctx, listUserGroupsSQL, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []groupRow
	for rows.Next() {
		var row groupRow
		if err := rows.Scan(&row.ID, &row.Name); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type tableRow struct {
	Schema      string
	Name        string
	RowEstimate int64
}

type schemaGroup struct {
	Name   string
	Tables []tableRow
}

type schemaPage struct {
	web.AppShell
	Connection web.DatabaseLink
	Schemas    []schemaGroup
	Tables     []tableRow
	Selected   *schemaTableDetail
	CanEdit    bool
	Error      string
}

type schemaTableDetail struct {
	Schema      string
	Name        string
	RowEstimate int64
	Columns     []columnRow
}

type columnRow struct {
	Name     string
	Type     string
	Nullable string
	Default  string
}

type tablePage struct {
	web.AppShell
	Connection  web.DatabaseLink
	Schema      string
	Table       string
	Tab         string
	TabTitle    string
	ItemCount   int
	Columns     []columnRow
	Constraints []ConstraintInfo
	Indexes     []IndexInfo
	RowEstimate int64
	Error       string
}

type tableDataPage struct {
	web.AppShell
	Connection web.DatabaseLink
	Schema     string
	Table      string
	Columns    []string
	Rows       [][]string
	Page       int
	TotalPages int
	PrevPage   int
	NextPage   int
	HasPrev    bool
	HasNext    bool
	Error      string
}

type error403Page struct {
	web.AppShell
	ConnectionName  string
	EffectiveLevel  string
	RequestedDB     int64
	Action          string
	RequestedAction string
}

type errorDBPage struct {
	web.AppShell
	ConnectionName string
}

func splitSchemaTable(t string) (schema, table string) {
	if i := strings.Index(t, "."); i >= 0 {
		return t[:i], t[i+1:]
	}
	return "public", t
}

func (h *Handlers) findLink(connID int64, links []web.DatabaseLink) *web.DatabaseLink {
	for i := range links {
		if links[i].ID == connID {
			return &links[i]
		}
	}
	return nil
}

func (h *Handlers) levelBadge(canView, canEdit bool) string {
	switch {
	case canEdit:
		return "View + Edit"
	case canView:
		return "View Only"
	default:
		return "Tidak ada akses"
	}
}

func (h *Handlers) schemaPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	connID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || connID <= 0 {
		http.NotFound(w, r)
		return
	}
	me, _ := auth.UserFromContext(ctx)

	shell := h.shell(ctx, r, connID, "databases", "Penjelajah Schema")
	link := h.findLink(connID, shell.Databases)
	if link == nil {
		http.NotFound(w, r)
		return
	}

	canView, _ := h.cfg.Policy.Can(ctx, me.ID, connID, access.ActionView)
	canEdit, _ := h.cfg.Policy.Can(ctx, me.ID, connID, access.ActionEdit)
	if !canView {
		h.render(w, http.StatusForbidden, "dbm/error_403.html", error403Page{
			AppShell:        shell,
			ConnectionName:  link.Name,
			EffectiveLevel:  h.levelBadge(canView, false),
			RequestedDB:     connID,
			Action:          "Lihat database",
			RequestedAction: "Lihat database",
		})
		return
	}

	pool, err := h.cfg.Registry.Get(ctx, connID)
	if err != nil {
		h.render(w, http.StatusOK, "dbm/error_db.html", errorDBPage{
			AppShell:       shell,
			ConnectionName: link.Name,
		})
		return
	}

	tables, err := ListSchemasTables(ctx, pool)
	if err != nil {
		h.render(w, http.StatusOK, "dbm/schema.html", schemaPage{
			AppShell:   shell,
			Connection: *link,
			Error:      err.Error(),
		})
		return
	}

	rows := make([]tableRow, 0, len(tables))
	for _, t := range tables {
		rows = append(rows, tableRow{Schema: t.Schema, Name: t.Name, RowEstimate: t.RowEstimate})
	}

	groups := groupTablesBySchema(rows)
	selected := r.URL.Query().Get("table")
	if selected == "" && len(rows) > 0 {
		selected = rows[0].Schema + "." + rows[0].Name
	}
	page := schemaPage{
		AppShell:   shell,
		Connection: *link,
		Schemas:    groups,
		Tables:     rows,
		CanEdit:    canEdit,
	}
	if selected != "" {
		detail := loadSchemaTableDetail(ctx, pool, selected)
		page.Selected = detail
	}
	h.render(w, http.StatusOK, "dbm/schema.html", page)
}

func groupTablesBySchema(rows []tableRow) []schemaGroup {
	var groups []schemaGroup
	index := make(map[string]int, len(rows))
	for _, row := range rows {
		pos, ok := index[row.Schema]
		if !ok {
			pos = len(groups)
			index[row.Schema] = pos
			groups = append(groups, schemaGroup{Name: row.Schema})
		}
		groups[pos].Tables = append(groups[pos].Tables, row)
	}
	return groups
}

func loadSchemaTableDetail(ctx context.Context, pool *pgxpool.Pool, qualified string) *schemaTableDetail {
	schema, table := splitSchemaTable(qualified)
	if schema == "" || table == "" {
		return nil
	}
	detail := &schemaTableDetail{Schema: schema, Name: table}
	cols, err := ListColumns(ctx, pool, schema, table)
	if err != nil {
		return detail
	}
	for _, c := range cols {
		def := ""
		if c.Default != nil {
			def = *c.Default
		}
		nullable := "Ya"
		if !c.Nullable {
			nullable = "Tidak"
		}
		detail.Columns = append(detail.Columns, columnRow{
			Name:     c.Name,
			Type:     c.Type,
			Nullable: nullable,
			Default:  def,
		})
	}
	return detail
}

func (h *Handlers) tablePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	connID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || connID <= 0 {
		http.NotFound(w, r)
		return
	}
	schema, table := splitSchemaTable(chi.URLParam(r, "t"))
	me, _ := auth.UserFromContext(ctx)

	shell := h.shell(ctx, r, connID, "databases", "Detail Tabel")
	link := h.findLink(connID, shell.Databases)
	if link == nil {
		http.NotFound(w, r)
		return
	}

	canView, _ := h.cfg.Policy.Can(ctx, me.ID, connID, access.ActionView)
	if !canView {
		h.render(w, http.StatusForbidden, "dbm/error_403.html", error403Page{
			AppShell:        shell,
			ConnectionName:  link.Name,
			EffectiveLevel:  h.levelBadge(canView, false),
			RequestedDB:     connID,
			Action:          "Lihat tabel",
			RequestedAction: "Lihat tabel",
		})
		return
	}

	pool, err := h.cfg.Registry.Get(ctx, connID)
	if err != nil {
		h.render(w, http.StatusOK, "dbm/error_db.html", errorDBPage{
			AppShell:       shell,
			ConnectionName: link.Name,
		})
		return
	}

	ok, err := validateTable(ctx, pool, schema, table)
	if err != nil {
		h.serverError(w, "validasi tabel", err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "columns"
	}
	tabTitle := map[string]string{"columns": "Columns", "constraints": "Constraints", "indexes": "Indexes"}[tab]
	if tabTitle == "" {
		tabTitle = "Columns"
	}

	page := tablePage{
		AppShell:   shell,
		Connection: *link,
		Schema:     schema,
		Table:      table,
		Tab:        tab,
		TabTitle:   tabTitle,
	}

	switch tab {
	case "columns":
		cols, err := ListColumns(ctx, pool, schema, table)
		if err != nil {
			page.Error = err.Error()
		} else {
			page.Columns = make([]columnRow, 0, len(cols))
			for _, c := range cols {
				def := ""
				if c.Default != nil {
					def = *c.Default
				}
				nullable := "Ya"
				if !c.Nullable {
					nullable = "Tidak"
				}
				page.Columns = append(page.Columns, columnRow{
					Name:     c.Name,
					Type:     c.Type,
					Nullable: nullable,
					Default:  def,
				})
			}
		}
	case "constraints":
		cons, err := ListConstraints(ctx, pool, schema, table)
		if err != nil {
			page.Error = err.Error()
		} else {
			page.Constraints = cons
		}
	case "indexes":
		idxs, err := ListIndexes(ctx, pool, schema, table)
		if err != nil {
			page.Error = err.Error()
		} else {
			page.Indexes = idxs
		}
	}

	switch tab {
	case "columns":
		page.ItemCount = len(page.Columns)
	case "constraints":
		page.ItemCount = len(page.Constraints)
	case "indexes":
		page.ItemCount = len(page.Indexes)
	}

	n, err := CountRows(ctx, pool, schema, table)
	if err != nil {
		page.RowEstimate = 0
	} else {
		page.RowEstimate = n
	}
	h.render(w, http.StatusOK, "dbm/table.html", page)
}

func (h *Handlers) tableDataPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	connID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || connID <= 0 {
		http.NotFound(w, r)
		return
	}
	schema, table := splitSchemaTable(chi.URLParam(r, "t"))
	me, _ := auth.UserFromContext(ctx)

	shell := h.shell(ctx, r, connID, "databases", "Data Tabel")
	link := h.findLink(connID, shell.Databases)
	if link == nil {
		http.NotFound(w, r)
		return
	}

	canView, _ := h.cfg.Policy.Can(ctx, me.ID, connID, access.ActionView)
	if !canView {
		h.render(w, http.StatusForbidden, "dbm/error_403.html", error403Page{
			AppShell:        shell,
			ConnectionName:  link.Name,
			EffectiveLevel:  h.levelBadge(canView, false),
			RequestedDB:     connID,
			Action:          "Lihat data",
			RequestedAction: "Lihat data tabel",
		})
		return
	}

	pool, err := h.cfg.Registry.Get(ctx, connID)
	if err != nil {
		h.render(w, http.StatusOK, "dbm/error_db.html", errorDBPage{
			AppShell:       shell,
			ConnectionName: link.Name,
		})
		return
	}

	ok, err := validateTable(ctx, pool, schema, table)
	if err != nil {
		h.serverError(w, "validasi tabel", err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}

	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	limit := 50
	offset := (page - 1) * limit

	columns, err := ListColumns(ctx, pool, schema, table)
	if err != nil {
		h.render(w, http.StatusOK, "dbm/table_data.html", tableDataPage{
			AppShell:   shell,
			Connection: *link,
			Schema:     schema,
			Table:      table,
			Error:      err.Error(),
		})
		return
	}
	colNames := make([]string, 0, len(columns))
	for _, c := range columns {
		colNames = append(colNames, c.Name)
	}

	_, dataRows, err := ListRows(ctx, pool, schema, table, limit, offset)
	if err != nil {
		h.render(w, http.StatusOK, "dbm/table_data.html", tableDataPage{
			AppShell:   shell,
			Connection: *link,
			Schema:     schema,
			Table:      table,
			Columns:    colNames,
			Error:      err.Error(),
		})
		return
	}

	n, err := CountRows(ctx, pool, schema, table)
	if err != nil {
		n = 0
	}
	totalPages := (int(n) + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	h.render(w, http.StatusOK, "dbm/table_data.html", tableDataPage{
		AppShell:   shell,
		Connection: *link,
		Schema:     schema,
		Table:      table,
		Columns:    colNames,
		Rows:       dataRows,
		Page:       page,
		TotalPages: totalPages,
		PrevPage:   page - 1,
		NextPage:   page + 1,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
	})
}
