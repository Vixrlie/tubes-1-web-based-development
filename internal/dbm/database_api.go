package dbm

// JSON API database management untuk SPA: daftar database yang boleh
// diakses, schema, struktur dan data tabel, snapshot untuk schema diff, dan
// EXPLAIN. Semua route memeriksa izin per database lewat access.Policy.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

// databaseAPIRoutes dipasang di dalam r.Route("/api").
func (h *Handlers) databaseAPIRoutes(r chi.Router) {
	r.Get("/databases", h.apiListDatabases)
}

// databaseScopedRoutes dipasang di dalam r.Route("/api/databases/{connID}").
func (h *Handlers) databaseScopedRoutes(r chi.Router) {
	r.Get("/", h.apiGetDatabase)
	r.Post("/reconnect", h.apiReconnect)
	r.Get("/schema", h.apiSchema)
	r.Get("/snapshot", h.apiSnapshot)
	r.Get("/tables/{schema}/{table}", h.apiDescribeTable)
	r.Get("/tables/{schema}/{table}/rows", h.apiTableRows)
	r.Post("/explain", h.apiExplain)
}

// DatabaseItem = satu koneksi pada daftar database.
type DatabaseItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Engine      string `json:"engine"`
	Status      string `json:"status"` // connected | disconnected
	Permission  string `json:"permission"`
	SchemaCount *int   `json:"schema_count"`
	TableCount  *int   `json:"table_count"`
	Error       string `json:"error,omitempty"`
}

const unreachableMessage = "Tidak dapat terhubung ke database. Coba lagi beberapa saat."

func engineLabel(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err == nil && strings.HasPrefix(strings.ToLower(u.Scheme), "mysql") {
		return "MySQL"
	}
	return "PostgreSQL"
}

// permission mengembalikan "edit", "view", atau "" bila tidak boleh melihat.
func (h *Handlers) permission(ctx context.Context, userID, connID int64) (string, error) {
	view, err := h.policy.Can(ctx, userID, connID, access.ActionView)
	if err != nil || !view {
		return "", err
	}
	if edit, err := h.policy.Can(ctx, userID, connID, access.ActionEdit); err == nil && edit {
		return "edit", nil
	}
	return "view", nil
}

// describeDatabase mengisi status koneksi dan jumlah schema/tabel.
func (h *Handlers) describeDatabase(ctx context.Context, item *DatabaseItem) *Target {
	target, err := h.registry.Get(ctx, item.ID)
	if err != nil || target == nil {
		item.Status, item.Error = "disconnected", unreachableMessage
		return nil
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := target.DB.PingContext(pingCtx); err != nil {
		item.Status, item.Error = "disconnected", unreachableMessage
		return nil
	}
	item.Status = "connected"
	if summaries, err := ListTableSummaries(ctx, target); err == nil {
		schemas := map[string]bool{}
		for _, s := range summaries {
			schemas[s.Schema] = true
		}
		sc, tc := len(schemas), len(summaries)
		item.SchemaCount, item.TableCount = &sc, &tc
	}
	return target
}

func (h *Handlers) loadDatabase(ctx context.Context, user auth.User, connID int64) (DatabaseItem, error) {
	item := DatabaseItem{ID: connID}
	var rawURL string
	err := h.platform.QueryRow(ctx, `SELECT name, connection_url FROM db_connections WHERE id = $1`, connID).Scan(&item.Name, &rawURL)
	if err != nil {
		return item, ErrConnNotFound
	}
	perm, err := h.permission(ctx, user.ID, connID)
	if err != nil {
		return item, err
	}
	if perm == "" {
		return item, ErrForbidden
	}
	item.Permission, item.Engine = perm, engineLabel(rawURL)
	return item, nil
}

func (h *Handlers) apiListDatabases(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	rows, err := h.platform.Query(r.Context(), `SELECT id, name, connection_url FROM db_connections ORDER BY name`)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	type conn struct {
		id        int64
		name, raw string
	}
	var conns []conn
	for rows.Next() {
		var c conn
		if err := rows.Scan(&c.id, &c.name, &c.raw); err != nil {
			rows.Close()
			h.apiFail(w, r, err)
			return
		}
		conns = append(conns, c)
	}
	rows.Close()
	items := make([]DatabaseItem, 0, len(conns))
	for _, c := range conns {
		perm, err := h.permission(r.Context(), user.ID, c.id)
		if err != nil || perm == "" {
			continue // database tanpa izin tidak ditampilkan
		}
		item := DatabaseItem{ID: c.id, Name: c.name, Engine: engineLabel(c.raw), Permission: perm}
		h.describeDatabase(r.Context(), &item)
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) apiGetDatabase(w http.ResponseWriter, r *http.Request) {
	item, _, ok := h.apiDatabase(w, r, access.ActionView)
	if ok {
		writeJSON(w, http.StatusOK, item)
	}
}

// apiReconnect membuang koneksi lama di registry lalu mencoba terhubung lagi.
func (h *Handlers) apiReconnect(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return
	}
	item, err := h.loadDatabase(r.Context(), user, connID)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	h.registry.Evict(connID)
	if h.describeDatabase(r.Context(), &item); item.Status != "connected" {
		writeAPIError(w, http.StatusServiceUnavailable, "unreachable", unreachableMessage)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// apiDatabase: izin + koneksi target. false = respons sudah dikirim.
func (h *Handlers) apiDatabase(w http.ResponseWriter, r *http.Request, action access.Action) (DatabaseItem, *Target, bool) {
	user, ok := apiActor(w, r)
	if !ok {
		return DatabaseItem{}, nil, false
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return DatabaseItem{}, nil, false
	}
	item, err := h.loadDatabase(r.Context(), user, connID)
	if err != nil {
		h.apiFail(w, r, err)
		return item, nil, false
	}
	if action == access.ActionEdit && item.Permission != "edit" {
		h.apiFail(w, r, ErrForbidden)
		return item, nil, false
	}
	target := h.describeDatabase(r.Context(), &item)
	if target == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "unreachable", unreachableMessage)
		return item, nil, false
	}
	return item, target, true
}

// SchemaTable / SchemaRelation / SchemaData = bentuk schema untuk SPA.
type SchemaTable struct {
	Name        string   `json:"name"`
	ColumnCount int      `json:"column_count"`
	RowEstimate int64    `json:"row_estimate"`
	PrimaryKey  []string `json:"primary_key"`
	Kind        string   `json:"kind"`
}

type SchemaRelation struct {
	Name       string `json:"name"`
	FromTable  string `json:"from_table"`
	FromColumn string `json:"from_column"`
	ToSchema   string `json:"to_schema"`
	ToTable    string `json:"to_table"`
	ToColumn   string `json:"to_column"`
}

type SchemaData struct {
	Name      string           `json:"name"`
	Tables    []SchemaTable    `json:"tables"`
	Relations []SchemaRelation `json:"relations"`
}

func csvList(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseRef memecah "schema.tabel(kolom, ...)".
func parseRef(ref string) (schema, table string, cols []string) {
	open := strings.Index(ref, "(")
	name := ref
	if open >= 0 {
		name = ref[:open]
		cols = csvList(strings.TrimSuffix(ref[open+1:], ")"))
	}
	schema, table = split2(name)
	return schema, table, cols
}

func loadSchemas(ctx context.Context, target *Target) ([]SchemaData, error) {
	summaries, err := ListTableSummaries(ctx, target)
	if err != nil {
		return nil, err
	}
	bySchema := map[string]*SchemaData{}
	var order []string
	get := func(name string) *SchemaData {
		if s, ok := bySchema[name]; ok {
			return s
		}
		s := &SchemaData{Name: name, Tables: []SchemaTable{}, Relations: []SchemaRelation{}}
		bySchema[name] = s
		order = append(order, name)
		return s
	}
	for _, t := range summaries {
		s := get(t.Schema)
		s.Tables = append(s.Tables, SchemaTable{Name: t.Name, ColumnCount: t.Columns, RowEstimate: max(t.RowEstimate, 0),
			PrimaryKey: csvList(t.PK), Kind: "table"})
	}
	if fks, err := ListForeignKeys(ctx, target); err == nil {
		for _, fk := range fks {
			fs, ft, fc := parseRef(fk.From)
			ts, tt, tc := parseRef(fk.ToRef)
			for i := range fc {
				rel := SchemaRelation{Name: fk.Name, FromTable: ft, FromColumn: fc[i], ToSchema: ts, ToTable: tt}
				if i < len(tc) {
					rel.ToColumn = tc[i]
				}
				get(fs).Relations = append(get(fs).Relations, rel)
			}
		}
	}
	out := make([]SchemaData, 0, len(order))
	for _, name := range order {
		out = append(out, *bySchema[name])
	}
	return out, nil
}

func (h *Handlers) apiSchema(w http.ResponseWriter, r *http.Request) {
	item, target, ok := h.apiDatabase(w, r, access.ActionView)
	if !ok {
		return
	}
	schemas, err := loadSchemas(r.Context(), target)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"database": item, "schemas": schemas})
}

// TableColumn / TableConstraint / TableIndex / TableDescription = struktur tabel.
type ColumnRef struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Column string `json:"column"`
}

type TableColumn struct {
	Name       string     `json:"name"`
	DataType   string     `json:"data_type"`
	Nullable   bool       `json:"nullable"`
	Default    *string    `json:"default"`
	PrimaryKey bool       `json:"primary_key"`
	References *ColumnRef `json:"references,omitempty"`
}

type TableConstraint struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Columns    []string `json:"columns"`
	Definition string   `json:"definition"`
}

type TableIndex struct {
	Name       string   `json:"name"`
	Columns    []string `json:"columns"`
	Unique     bool     `json:"unique"`
	Definition string   `json:"definition"`
}

type TableDescription struct {
	Schema      string            `json:"schema"`
	Name        string            `json:"name"`
	Columns     []TableColumn     `json:"columns"`
	Constraints []TableConstraint `json:"constraints"`
	Indexes     []TableIndex      `json:"indexes"`
	Permission  string            `json:"permission"`
}

var errTableNotFound = errors.New("tabel tidak ditemukan")

func describeTable(ctx context.Context, target *Target, schema, table string) (TableDescription, error) {
	d := TableDescription{Columns: []TableColumn{}, Constraints: []TableConstraint{}, Indexes: []TableIndex{}}
	canonSchema, canonTable, ok, err := resolveTable(ctx, target, schema, table)
	if err != nil {
		return d, err
	}
	if !ok {
		return d, errTableNotFound
	}
	d.Schema = canonSchema
	d.Name = canonTable
	cols, err := ListColumns(ctx, target, canonSchema, canonTable)
	if err != nil {
		return d, err
	}
	cons, err := ListConstraints(ctx, target, canonSchema, canonTable)
	if err != nil {
		return d, err
	}
	pk := map[string]bool{}
	refs := map[string]*ColumnRef{}
	for _, c := range cons {
		columns := csvList(c.Columns)
		def := fmt.Sprintf("%s (%s)", c.Type, strings.Join(columns, ", "))
		switch c.Type {
		case "PRIMARY KEY":
			for _, col := range columns {
				pk[col] = true
			}
		case "FOREIGN KEY":
			rs, rt, rc := parseRef(c.Reference)
			def += fmt.Sprintf(" REFERENCES %s.%s(%s)", rs, rt, strings.Join(rc, ", "))
			if len(columns) == 1 && len(rc) >= 1 {
				refs[columns[0]] = &ColumnRef{Schema: rs, Table: rt, Column: rc[0]}
			}
		}
		d.Constraints = append(d.Constraints, TableConstraint{Name: c.Name, Type: c.Type, Columns: columns, Definition: def})
	}
	for _, c := range cols {
		d.Columns = append(d.Columns, TableColumn{Name: c.Name, DataType: c.Type, Nullable: c.Nullable, Default: c.Default,
			PrimaryKey: pk[c.Name], References: refs[c.Name]})
	}
	if idx, err := ListIndexes(ctx, target, canonSchema, canonTable); err == nil {
		for _, i := range idx {
			unique := ""
			if i.Unique {
				unique = "UNIQUE "
			}
			d.Indexes = append(d.Indexes, TableIndex{Name: i.Name, Columns: csvList(i.Columns), Unique: i.Unique,
				Definition: fmt.Sprintf("CREATE %sINDEX %s ON %s USING %s (%s)", unique, i.Name, target.QuoteTable(canonSchema, canonTable), i.Method, i.Columns)})
		}
	}
	return d, nil
}

func (h *Handlers) apiTableTarget(w http.ResponseWriter, r *http.Request) (DatabaseItem, *Target, string, string, bool) {
	item, target, ok := h.apiDatabase(w, r, access.ActionView)
	if !ok {
		return item, nil, "", "", false
	}
	return item, target, chi.URLParam(r, "schema"), chi.URLParam(r, "table"), true
}

func (h *Handlers) apiDescribeTable(w http.ResponseWriter, r *http.Request) {
	item, target, schema, table, ok := h.apiTableTarget(w, r)
	if !ok {
		return
	}
	d, err := describeTable(r.Context(), target, schema, table)
	if errors.Is(err, errTableNotFound) {
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	d.Permission = item.Permission
	writeJSON(w, http.StatusOK, d)
}

const (
	defaultPageSize = 25
	maxPageSize     = 100
)

// TablePage = satu halaman data tabel. Nilai NULL dikirim sebagai null.
type TablePage struct {
	Columns  []string         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Pages    int              `json:"pages"`
}

func positiveInt(raw string, def int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func (h *Handlers) apiTableRows(w http.ResponseWriter, r *http.Request) {
	_, target, schema, table, ok := h.apiTableTarget(w, r)
	if !ok {
		return
	}
	d, err := describeTable(r.Context(), target, schema, table)
	if errors.Is(err, errTableNotFound) {
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	size := min(positiveInt(r.URL.Query().Get("page_size"), defaultPageSize), maxPageSize)
	page := positiveInt(r.URL.Query().Get("page"), 1)
	desc := r.URL.Query().Get("direction") == "desc"
	out, err := readTablePage(r.Context(), target, d, page, size, desc)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// readTablePage membaca satu halaman, diurutkan menurut primary key bila ada
// agar paginasi stabil. Nama tabel/kolom sudah divalidasi lewat describeTable.
func readTablePage(ctx context.Context, target *Target, d TableDescription, page, size int, desc bool) (TablePage, error) {
	out := TablePage{Columns: []string{}, Rows: []map[string]any{}, PageSize: size}
	total, err := CountRows(ctx, target, d.Schema, d.Name)
	if err != nil {
		return out, err
	}
	out.Total = total
	out.Pages = max(1, int((total+int64(size)-1)/int64(size)))
	out.Page = min(page, out.Pages)

	var order []string
	for _, c := range d.Columns {
		if c.PrimaryKey {
			col := target.Dialect.QuoteIdent(c.Name)
			if desc {
				col += " DESC"
			}
			order = append(order, col)
		}
	}
	q := "SELECT * FROM " + target.QuoteTable(d.Schema, d.Name)
	if len(order) > 0 {
		q += " ORDER BY " + strings.Join(order, ", ")
	}
	q += " LIMIT $1 OFFSET $2"
	if target.Dialect.Name() == "mysql" {
		q = strings.Replace(strings.Replace(q, "$1", "?", 1), "$2", "?", 1)
	}
	rows, err := target.DB.QueryContext(ctx, q, size, (out.Page-1)*size)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return out, err
	}
	out.Columns = cols
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return out, err
		}
		rec := make(map[string]any, len(cols))
		for i, v := range vals {
			rec[cols[i]] = jsonCell(v)
		}
		out.Rows = append(out.Rows, rec)
	}
	return out, rows.Err()
}

// jsonCell mengubah nilai driver menjadi nilai JSON (string/number/bool/null).
func jsonCell(v any) any {
	switch val := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(val)
	case bool, string, float64, float32:
		return val
	case int64:
		// Bilangan di luar rentang aman JavaScript dikirim sebagai string.
		if val > 1<<53 || val < -(1<<53) {
			return strconv.FormatInt(val, 10)
		}
		return val
	case int32, int16, int8, int:
		return val
	case time.Time:
		return val.Format(time.RFC3339Nano)
	default:
		return formatValue(val)
	}
}

// Snapshot = struktur seluruh tabel satu database untuk schema diff di SPA.
type Snapshot struct {
	ID     int64              `json:"id"`
	Name   string             `json:"name"`
	Tables []TableDescription `json:"tables"`
}

func (h *Handlers) apiSnapshot(w http.ResponseWriter, r *http.Request) {
	item, target, ok := h.apiDatabase(w, r, access.ActionView)
	if !ok {
		return
	}
	summaries, err := ListTableSummaries(r.Context(), target)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	snap := Snapshot{ID: item.ID, Name: item.Name, Tables: []TableDescription{}}
	for _, t := range summaries {
		d, err := describeTable(r.Context(), target, t.Schema, t.Name)
		if err != nil {
			h.apiFail(w, r, err)
			return
		}
		d.Permission = item.Permission
		snap.Tables = append(snap.Tables, d)
	}
	writeJSON(w, http.StatusOK, snap)
}

// ExplainResult = hasil EXPLAIN satu statement.
type ExplainResult struct {
	N          int    `json:"n"`
	Statement  string `json:"statement"`
	Kind       string `json:"kind"`
	Note       string `json:"note,omitempty"`
	Scan       string `json:"scan,omitempty"`
	Index      string `json:"index,omitempty"`
	Join       string `json:"join,omitempty"`
	Cost       string `json:"cost,omitempty"`
	ActualTime string `json:"actual_time,omitempty"`
	Rows       string `json:"rows,omitempty"`
	Plan       string `json:"plan,omitempty"`
	Analyze    bool   `json:"analyze"`
	// Root plan untuk metrik di SPA; null bila statement tidak punya plan.
	Root        *ExplainRoot `json:"root"`
	PlanningMs  *float64     `json:"planning_ms"`
	ExecutionMs *float64     `json:"execution_ms"`
}

type ExplainRoot struct {
	Node        string  `json:"node"`
	Relation    string  `json:"relation"`
	StartupCost float64 `json:"startup_cost"`
	TotalCost   float64 `json:"total_cost"`
	Rows        float64 `json:"rows"`
	Width       float64 `json:"width"`
}

const maxExplainSQL = 50_000

// apiExplain: EXPLAIN cukup akses view; EXPLAIN ANALYZE benar-benar
// menjalankan statement (selalu di-ROLLBACK) sehingga butuh akses edit.
func (h *Handlers) apiExplain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SQL     string `json:"sql"`
		Analyze bool   `json:"analyze"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	action := access.ActionView
	if body.Analyze {
		action = access.ActionEdit
	}
	_, target, ok := h.apiDatabase(w, r, action)
	if !ok {
		return
	}
	if strings.TrimSpace(body.SQL) == "" || len(body.SQL) > maxExplainSQL {
		writeAPIError(w, http.StatusUnprocessableEntity, "invalid_input", "isi SQL untuk EXPLAIN, maksimal 50.000 karakter")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	items := ExplainAll(ctx, target, body.SQL, body.Analyze)
	out := make([]ExplainResult, 0, len(items))
	for _, it := range items {
		out = append(out, ExplainResult{N: it.N, Statement: it.Stmt, Kind: it.Kind, Note: it.Note,
			Scan: it.Summary.Scan, Index: it.Summary.Index, Join: it.Summary.Join, Cost: it.Summary.Cost,
			ActualTime: it.Summary.ActualTime, Rows: it.Summary.Rows, Plan: it.Tree, Analyze: it.Analyze,
			PlanningMs: it.PlanningMs, ExecutionMs: it.ExecutionMs})
		if it.Root.Node != "" {
			root := ExplainRoot(it.Root)
			out[len(out)-1].Root = &root
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
