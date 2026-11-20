package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

// --- unit: parse/bangun connection URL --------------------------------------

func TestParseConnectionURL(t *testing.T) {
	host, port, database, username := parseConnectionURL(
		"postgres://alice:s3cret@db.example.com:5433/shop?sslmode=disable")
	if host != "db.example.com" || port != "5433" || database != "shop" || username != "alice" {
		t.Errorf("parse = (%q,%q,%q,%q)", host, port, database, username)
	}

	// Port kosong -> default 5432; path tanpa slash.
	host, port, database, username = parseConnectionURL("postgres://bob@srv/db2")
	if host != "srv" || port != "5432" || database != "db2" || username != "bob" {
		t.Errorf("parse default = (%q,%q,%q,%q)", host, port, database, username)
	}

	// URL rusak tidak boleh panik dan mengembalikan string kosong.
	host, port, database, username = parseConnectionURL(":::bukan-url")
	if host != "" || port != "" || database != "" || username != "" {
		t.Errorf("parse rusak = (%q,%q,%q,%q), want semua kosong", host, port, database, username)
	}
}

func TestBuildConnectionURL(t *testing.T) {
	in := ConnectionInput{Host: "db.internal", Port: "5434", Database: "shop", Username: "alice", Password: "rahasia"}
	raw, err := buildConnectionURL(in, "")
	if err != nil {
		t.Fatalf("buildConnectionURL: %v", err)
	}
	if !strings.Contains(raw, "alice:rahasia@db.internal:5434/shop") {
		t.Errorf("URL = %q, tidak sesuai format", raw)
	}

	// Password kosong saat update: pakai password lama.
	kept, err := buildConnectionURL(ConnectionInput{Host: "db.internal", Port: "5434", Database: "shop", Username: "alice"}, raw)
	if err != nil {
		t.Fatalf("buildConnectionURL (reuse): %v", err)
	}
	if !strings.Contains(kept, "alice:rahasia@") {
		t.Errorf("password lama tidak dipertahankan: %q", kept)
	}

	// Tanpa password sama sekali -> ditolak.
	if _, err := buildConnectionURL(ConnectionInput{Host: "h", Port: "1", Database: "d", Username: "u"}, ""); err != ErrPasswordRequired {
		t.Errorf("err = %v, want ErrPasswordRequired", err)
	}
}

func TestValidateConnection(t *testing.T) {
	full := ConnectionInput{Name: "prod", Host: "h", Port: "5432", Database: "d", Username: "u", Password: "p"}
	if errs := ValidateConnection(full, true); len(errs) != 0 {
		t.Errorf("input validpunya error: %v", errs)
	}
	if errs := ValidateConnection(full, false); len(errs) != 0 {
		t.Errorf("update tanpa password punya error: %v", errs)
	}

	empty := ConnectionInput{}
	errs := ValidateConnection(empty, true)
	for _, field := range []string{"name", "host", "port", "database", "username", "password"} {
		if _, ok := errs[field]; !ok {
			t.Errorf("field %q tidak punya error meski kosong", field)
		}
	}

	for _, badPort := range []string{"0", "65536", "abc", "-1"} {
		in := full
		in.Port = badPort
		if _, ok := ValidateConnection(in, false)["port"]; !ok {
			t.Errorf("port %q diterima", badPort)
		}
	}
}

// --- integration: store CRUD + cascade --------------------------------------

func TestConnectionStoreCRUD(t *testing.T) {
	pool := testutil.NewPool(t)
	store := NewPGConnectionStore(pool)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash) VALUES ('crud-admin', 'x') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("user: %v", err)
	}

	in := ConnectionInput{Name: "toko", Description: "DB toko", Host: "127.0.0.1", Port: "5433", Database: "shop", Username: "target", Password: "rahasia"}
	created, err := store.Create(ctx, in, userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Host != "127.0.0.1" || created.Port != "5433" || created.Database != "shop" || created.Username != "target" {
		t.Errorf("field terurai = %+v", created)
	}

	// Duplikat nama -> sentinel.
	dup := in
	dup.Description = "kedua"
	if _, err := store.Create(ctx, dup, 1); err != ErrConnectionNameTaken {
		t.Errorf("duplikat: err = %v, want ErrConnectionNameTaken", err)
	}

	// List berisi satu baris.
	rows, err := store.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("List = %d baris, err=%v", len(rows), err)
	}

	// Update: password kosong dipertahankan, field tampilan berubah.
	upd := ConnectionInput{Name: "toko-v2", Description: "diperbarui", Host: "127.0.0.1", Port: "5433", Database: "shop", Username: "target"}
	updated, err := store.Update(ctx, created.ID, upd)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "toko-v2" {
		t.Errorf("name = %q", updated.Name)
	}

	// Get by id cocok.
	got, err := store.Get(ctx, created.ID)
	if err != nil || got.Name != "toko-v2" {
		t.Errorf("Get = %+v, err=%v", got, err)
	}

	// Delete lalu Get -> not found.
	if err := store.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, created.ID); err != ErrConnectionNotFound {
		t.Errorf("Get setelah delete: err = %v, want ErrConnectionNotFound", err)
	}
	if err := store.Delete(ctx, created.ID); err != ErrConnectionNotFound {
		t.Errorf("delete kedua: err = %v, want ErrConnectionNotFound", err)
	}
}

func TestConnectionDeleteCascadesGrantsAndMigrations(t *testing.T) {
	pool := testutil.NewPool(t)
	store := NewPGConnectionStore(pool)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash) VALUES ('cascade-admin', 'x') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("user: %v", err)
	}

	conn, err := store.Create(ctx, ConnectionInput{Name: "cascade-db", Host: "h", Port: "5432", Database: "d", Username: "u", Password: "p"}, userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var groupID int64
	if err := pool.QueryRow(ctx, `INSERT INTO groups (name) VALUES ('tim') RETURNING id`).Scan(&groupID); err != nil {
		t.Fatalf("group: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO grants (group_id, connection_id, level) VALUES ($1, $2, 'edit')`, groupID, conn.ID); err != nil {
		t.Fatalf("grant: %v", err)
	}
	// Rantai lengkap: definisi, pengajuan, eksekusi final, dan rollback yang
	// merujuknya. Trigger append-only tidak boleh menghalangi cascade.
	var migrationID, requestID, execID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO migrations (connection_id, version, name, up_sql, down_sql, checksum, locked_at)
		 VALUES ($1, 1, 'm', 'SELECT 1', 'SELECT 2', 'x', now()) RETURNING id`,
		conn.ID).Scan(&migrationID); err != nil {
		t.Fatalf("migration: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO migration_requests (migration_id, connection_id, action, migration_revision, checksum, status)
		 VALUES ($1, $2, 'up', 1, 'x', 'success') RETURNING id`,
		migrationID, conn.ID).Scan(&requestID); err != nil {
		t.Fatalf("request: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO migration_executions (request_id, migration_id, connection_id, action,
		        migration_version, migration_revision, sql_executed, status)
		 VALUES ($1, $2, $3, 'up', 1, 1, 'SELECT 1', 'success') RETURNING id`,
		requestID, migrationID, conn.ID).Scan(&execID); err != nil {
		t.Fatalf("execution: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO migration_executions (request_id, migration_id, connection_id, action,
		        reverts_execution_id, migration_version, migration_revision, sql_executed, status)
		 VALUES ($1, $2, $3, 'rollback', $4, 1, 1, 'SELECT 2', 'success')`,
		requestID, migrationID, conn.ID, execID); err != nil {
		t.Fatalf("rollback execution: %v", err)
	}

	if err := store.Delete(ctx, conn.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var left int
	if err := pool.QueryRow(ctx,
		`SELECT (SELECT COUNT(*) FROM grants WHERE connection_id = $1) +
		        (SELECT COUNT(*) FROM migrations WHERE connection_id = $1) +
		        (SELECT COUNT(*) FROM migration_requests WHERE connection_id = $1) +
		        (SELECT COUNT(*) FROM migration_executions WHERE connection_id = $1)`, conn.ID).Scan(&left); err != nil {
		t.Fatalf("hitung sisa: %v", err)
	}
	if left != 0 {
		t.Errorf("%d baris grants/migration tersisa setelah koneksi dihapus", left)
	}
}

// --- handler: konfirmasi hapus ----------------------------------------------

type connViewStub struct{ names []string }

func (s *connViewStub) RenderApp(w http.ResponseWriter, name string, data any) {
	s.names = append(s.names, name)
}

func (s *connViewStub) Render(w http.ResponseWriter, name string, data any) {}

func (s *connViewStub) RenderConnectionTest(w http.ResponseWriter, p web.ConnectionTestPanel) {}

func connRequest(method, target string, form url.Values, id string) *http.Request {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, target, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestDeleteConnectionDirectPost(t *testing.T) {
	// Konfirmasi hapus kini memakai dialog browser (template fe-components:
	// onsubmit confirm()), sehingga server menerima POST langsung.
	pool := testutil.NewPool(t)
	store := NewPGConnectionStore(pool)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash) VALUES ('conn-admin', 'x') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("user: %v", err)
	}
	row, err := store.Create(ctx, ConnectionInput{Name: "konfirm-db", Host: "h", Port: "5432", Database: "d", Username: "u", Password: "p"}, userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	idStr := strconv.FormatInt(row.ID, 10)

	c := &connectionHandlers{store: store, view: &connViewStub{}}

	// Koneksi tidak ada -> 404.
	w := httptest.NewRecorder()
	c.remove(w, connRequest(http.MethodPost, "/admin/connections/999999/delete", nil, "999999"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("hapus tak dikenal: status = %d, want 404", w.Code)
	}

	// POST langsung (konfirmasi JS di sisi klien) -> 303 dan benar-benar terhapus.
	w2 := httptest.NewRecorder()
	c.remove(w2, connRequest(http.MethodPost, "/admin/connections/x/delete", nil, idStr))
	if w2.Code != http.StatusSeeOther {
		t.Fatalf("hapus: status = %d, want 303; body=%s", w2.Code, w2.Body.String())
	}
	if loc := w2.Header().Get("Location"); loc != "/admin/connections" {
		t.Errorf("Location = %q", loc)
	}
	if _, err := store.Get(ctx, row.ID); err != ErrConnectionNotFound {
		t.Errorf("koneksi masih ada setelah hapus: %v", err)
	}
}
