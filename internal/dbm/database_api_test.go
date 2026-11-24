package dbm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

func TestParseRef(t *testing.T) {
	s, tb, cols := parseRef("public.users(id, tenant_id)")
	if s != "public" || tb != "users" || len(cols) != 2 || cols[1] != "tenant_id" {
		t.Fatalf("parseRef = %q %q %v", s, tb, cols)
	}
	if s, tb, cols := parseRef("orders"); s != "public" || tb != "orders" || len(cols) != 0 {
		t.Fatalf("tanpa schema/kolom = %q %q %v", s, tb, cols)
	}
}

func TestJSONCell(t *testing.T) {
	if jsonCell(nil) != nil {
		t.Fatal("NULL harus null")
	}
	if jsonCell([]byte("abc")) != "abc" {
		t.Fatal("[]byte harus string")
	}
	if jsonCell(int64(1)<<60) != "1152921504606846976" {
		t.Fatal("int64 besar harus string agar tidak kehilangan presisi di JS")
	}
	if jsonCell(int64(42)) != int64(42) {
		t.Fatal("int64 kecil tetap angka")
	}
}

func TestEngineLabel(t *testing.T) {
	if engineLabel("mysql://u:p@h/db") != "MySQL" || engineLabel("postgres://u:p@h/db") != "PostgreSQL" {
		t.Fatal("engineLabel salah")
	}
}

// dbAPI = handler API dengan registry yang menunjuk ke target fixture.
func (f *workflowFixture) dbAPI() http.Handler {
	h := NewHandlers(Config{
		Platform: f.pool,
		Policy:   access.NewPolicy(access.NewPGLevelSource(f.pool)),
		Registry: fixedRegistry{target: &Target{DB: f.targetDB, Dialect: Postgres{}}},
		Workflow: f.wf,
	})
	r := chi.NewRouter()
	h.APIRoutes(r)
	return r
}

func (f *workflowFixture) get(h http.Handler, as auth.User, method, path, body string) (int, []byte) {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithUser(req.Context(), as))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func decodeInto[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("JSON tidak valid: %v (%s)", err, raw)
	}
	return v
}

func TestDatabaseAPI(t *testing.T) {
	f := newWorkflowFixture(t)
	for _, q := range []string{
		`CREATE TABLE customers (id BIGINT PRIMARY KEY, email TEXT UNIQUE, note TEXT)`,
		`CREATE TABLE orders (id BIGINT PRIMARY KEY, customer_id BIGINT REFERENCES customers (id), total NUMERIC(10,2) NOT NULL DEFAULT 0)`,
		`CREATE INDEX orders_total_idx ON orders (total)`,
		`INSERT INTO customers SELECT i, 'c' || i || '@mail.com', CASE WHEN i % 2 = 0 THEN NULL ELSE 'catatan' END FROM generate_series(1, 30) i`,
	} {
		if _, err := f.targetDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	var schema string
	if err := f.targetDB.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	h := f.dbAPI()
	base := fmt.Sprintf("/api/databases/%d", f.connID)

	t.Run("daftar hanya database yang boleh diakses", func(t *testing.T) {
		code, raw := f.get(h, f.editor, http.MethodGet, "/api/databases", "")
		list := decodeInto[struct{ Items []DatabaseItem }](t, raw)
		if code != http.StatusOK || len(list.Items) != 1 || list.Items[0].Permission != "edit" || list.Items[0].Status != "connected" {
			t.Fatalf("editor: %d %s", code, raw)
		}
		_, raw = f.get(h, f.viewer, http.MethodGet, "/api/databases", "")
		if v := decodeInto[struct{ Items []DatabaseItem }](t, raw); len(v.Items) != 1 || v.Items[0].Permission != "view" {
			t.Fatalf("viewer: %s", raw)
		}
		_, raw = f.get(h, f.outsider, http.MethodGet, "/api/databases", "")
		if v := decodeInto[struct{ Items []DatabaseItem }](t, raw); len(v.Items) != 0 {
			t.Fatalf("outsider melihat database tanpa izin: %s", raw)
		}
	})

	t.Run("schema dan relasi", func(t *testing.T) {
		code, raw := f.get(h, f.viewer, http.MethodGet, base+"/schema", "")
		if code != http.StatusOK {
			t.Fatalf("%d %s", code, raw)
		}
		resp := decodeInto[struct {
			Database DatabaseItem
			Schemas  []SchemaData
		}](t, raw)
		var mine *SchemaData
		for i := range resp.Schemas {
			if resp.Schemas[i].Name == schema {
				mine = &resp.Schemas[i]
			}
		}
		if mine == nil {
			t.Fatalf("schema %s tidak ada", schema)
		}
		tables := map[string]SchemaTable{}
		for _, tb := range mine.Tables {
			tables[tb.Name] = tb
		}
		if tables["orders"].ColumnCount != 3 || len(tables["customers"].PrimaryKey) != 1 {
			t.Fatalf("tabel: %+v", tables)
		}
		var rel *SchemaRelation
		for i := range mine.Relations {
			if mine.Relations[i].Name == "orders_customer_id_fkey" {
				rel = &mine.Relations[i]
			}
		}
		if rel == nil || rel.FromTable != "orders" || rel.FromColumn != "customer_id" || rel.ToTable != "customers" || rel.ToColumn != "id" {
			t.Fatalf("relasi: %+v", rel)
		}
		if code, _ := f.get(h, f.outsider, http.MethodGet, base+"/schema", ""); code != http.StatusForbidden {
			t.Fatalf("outsider schema: %d", code)
		}
	})

	t.Run("struktur tabel", func(t *testing.T) {
		code, raw := f.get(h, f.viewer, http.MethodGet, base+"/tables/"+schema+"/orders", "")
		if code != http.StatusOK {
			t.Fatalf("%d %s", code, raw)
		}
		d := decodeInto[TableDescription](t, raw)
		if len(d.Columns) != 3 || !d.Columns[0].PrimaryKey || d.Columns[1].References == nil || d.Columns[1].References.Table != "customers" {
			t.Fatalf("kolom: %+v", d.Columns)
		}
		if len(d.Indexes) != 1 || d.Indexes[0].Name != "orders_total_idx" || d.Permission != "view" {
			t.Fatalf("index/permission: %+v %s", d.Indexes, d.Permission)
		}
		if code, _ := f.get(h, f.viewer, http.MethodGet, base+"/tables/"+schema+"/tidak_ada", ""); code != http.StatusNotFound {
			t.Fatalf("tabel tidak ada: %d", code)
		}
		// Nama tabel berbahaya ditolak oleh validasi information_schema.
		if code, _ := f.get(h, f.viewer, http.MethodGet, base+"/tables/"+schema+"/orders%3B%20DROP%20TABLE%20orders", ""); code != http.StatusNotFound {
			t.Fatalf("injeksi nama tabel: %d", code)
		}
	})

	t.Run("data tabel berhalaman dengan NULL", func(t *testing.T) {
		code, raw := f.get(h, f.viewer, http.MethodGet, base+"/tables/"+schema+"/customers/rows?page=2&page_size=10&direction=desc", "")
		if code != http.StatusOK {
			t.Fatalf("%d %s", code, raw)
		}
		p := decodeInto[TablePage](t, raw)
		if p.Total != 30 || p.Pages != 3 || p.Page != 2 || len(p.Rows) != 10 {
			t.Fatalf("halaman: total=%d pages=%d page=%d rows=%d", p.Total, p.Pages, p.Page, len(p.Rows))
		}
		if p.Rows[0]["id"] != float64(20) {
			t.Fatalf("urutan desc menurut PK salah: %v", p.Rows[0]["id"])
		}
		if p.Rows[0]["note"] != nil {
			t.Fatalf("NULL harus dikirim sebagai null, got %v", p.Rows[0]["note"])
		}
		_, raw = f.get(h, f.viewer, http.MethodGet, base+"/tables/"+schema+"/customers/rows?page=99&page_size=1000", "")
		if p := decodeInto[TablePage](t, raw); p.PageSize != maxPageSize || p.Page != 1 {
			t.Fatalf("page_size dibatasi dan page di-clamp: %+v", p)
		}
	})

	t.Run("snapshot untuk schema diff", func(t *testing.T) {
		code, raw := f.get(h, f.viewer, http.MethodGet, base+"/snapshot", "")
		if code != http.StatusOK {
			t.Fatalf("%d %s", code, raw)
		}
		snap := decodeInto[Snapshot](t, raw)
		found := map[string]bool{}
		for _, tb := range snap.Tables {
			if tb.Schema == schema {
				found[tb.Name] = len(tb.Columns) > 0
			}
		}
		if !found["orders"] || !found["customers"] {
			t.Fatalf("snapshot schema %s tidak memuat struktur orders/customers: %v", schema, found)
		}
	})

	t.Run("explain dan izin analyze", func(t *testing.T) {
		code, raw := f.get(h, f.viewer, http.MethodPost, base+"/explain", `{"sql":"SELECT * FROM customers WHERE id = 1"}`)
		if code != http.StatusOK {
			t.Fatalf("%d %s", code, raw)
		}
		res := decodeInto[struct{ Items []ExplainResult }](t, raw)
		if len(res.Items) != 1 || res.Items[0].Plan == "" || res.Items[0].Kind == "ddl" {
			t.Fatalf("explain: %+v", res.Items)
		}
		if code, _ := f.get(h, f.viewer, http.MethodPost, base+"/explain", `{"sql":"SELECT 1","analyze":true}`); code != http.StatusForbidden {
			t.Fatalf("viewer EXPLAIN ANALYZE harus 403, got %d", code)
		}
		code, raw = f.get(h, f.editor, http.MethodPost, base+"/explain", `{"sql":"DELETE FROM customers","analyze":true}`)
		if code != http.StatusOK {
			t.Fatalf("editor analyze: %d %s", code, raw)
		}
		var left int
		if err := f.targetDB.QueryRow(`SELECT COUNT(*) FROM customers`).Scan(&left); err != nil || left != 30 {
			t.Fatalf("EXPLAIN ANALYZE harus di-rollback: sisa=%d err=%v", left, err)
		}
		if code, _ := f.get(h, f.viewer, http.MethodPost, base+"/explain", `{"sql":"  "}`); code != http.StatusUnprocessableEntity {
			t.Fatalf("SQL kosong: %d", code)
		}
	})

	t.Run("migration batch lewat API", func(t *testing.T) {
		code, raw := f.get(h, f.editor, http.MethodPost, base+"/migrations",
			`{"up_sql":"UPDATE customers SET note = 'x' WHERE id BETWEEN $1 AND $2","down_sql":"UPDATE customers SET note = NULL WHERE id BETWEEN $1 AND $2","batch":{"table":"customers","column":"id","size":10}}`)
		if code != http.StatusCreated {
			t.Fatalf("create batch: %d %s", code, raw)
		}
		if m := decodeInto[Migration](t, raw); m.Batch == nil || m.Batch.Size != 10 {
			t.Fatalf("batch tidak ikut respons: %s", raw)
		}
		code, raw = f.get(h, f.editor, http.MethodPost, base+"/migrations",
			`{"up_sql":"UPDATE customers SET note = 'x'","down_sql":"UPDATE customers SET note = NULL","batch":{"table":"customers","column":"id","size":10}}`)
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("batch tanpa placeholder: %d %s", code, raw)
		}
	})
}
