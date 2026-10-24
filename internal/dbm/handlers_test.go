package dbm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// fakePolicy mencatat panggilan Can dan mengembalikan hasil per aksi.
type fakePolicy struct {
	byAction map[access.Action]bool
	err      error
	calls    int
	lastUser int64
	lastConn int64
	lastAct  access.Action
}

func (p *fakePolicy) Can(ctx context.Context, userID int64, connID int64, a access.Action) (bool, error) {
	p.calls++
	p.lastUser, p.lastConn, p.lastAct = userID, connID, a
	if p.err != nil {
		return false, p.err
	}
	if p.byAction != nil {
		return p.byAction[a], nil
	}
	return false, nil
}

// testRenderer memakai renderer sungguhan supaya template ikut teruji.
func testRenderer(t *testing.T) *web.Renderer {
	t.Helper()
	r, err := web.NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	return r
}

// newTestHandlers menyusun handler dengan pool platform dari test-db.
func newTestHandlers(t *testing.T, pool *pgxpool.Pool, policy access.Policy, registry Registry) *Handlers {
	t.Helper()
	cfg := Config{
		Platform: pool,
		Policy:   policy,
		Registry: registry,
		Renderer: testRenderer(t),
	}
	if pool != nil {
		cfg.Shell = NewPGShellStore(pool)
	}
	return NewHandlers(cfg)
}

// withUser menaruh user login di context request.
func withUser(r *http.Request, u auth.User) *http.Request {
	return r.WithContext(auth.WithUser(r.Context(), u))
}

// insertConnection mendaftarkan koneksi target ke platform dan mengembalikan id.
func insertConnection(t *testing.T, pool *pgxpool.Pool, name, url string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO db_connections (name, connection_url) VALUES ($1, $2) RETURNING id`,
		name, url).Scan(&id); err != nil {
		t.Fatalf("insert db_connection: %v", err)
	}
	return id
}

// insertUser membuat user platform (untuk FK created_by/executed_by).
func insertUser(t *testing.T, pool *pgxpool.Pool, username string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		username).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// chiRequest membuat request GET/POST dengan URL param chi.
func chiRequest(method, target string, params map[string]string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// --- unit (tanpa database) --------------------------------------------------

func TestDenyRenders403Page(t *testing.T) {
	h := newTestHandlers(t, nil, &fakePolicy{}, nil)
	r := chiRequest(http.MethodGet, "/databases/1/schema", map[string]string{"connID": "1"})
	r = withUser(r, auth.User{ID: 7, Username: "alice", IsActive: true})
	w := httptest.NewRecorder()

	h.schema(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"Akses dibatasi", "ERROR / 403", "Tidak ada akses", "/databases"} {
		if !strings.Contains(body, want) {
			t.Errorf("halaman 403 tidak memuat %q", want)
		}
	}
}

func TestMissingUserIsForbidden(t *testing.T) {
	h := newTestHandlers(t, nil, &fakePolicy{byAction: map[access.Action]bool{access.ActionView: true}}, nil)
	r := chiRequest(http.MethodGet, "/databases/1/schema", map[string]string{"connID": "1"})
	w := httptest.NewRecorder()

	h.schema(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestBadConnIDIsNotFound(t *testing.T) {
	h := newTestHandlers(t, nil, &fakePolicy{byAction: map[access.Action]bool{access.ActionView: true}}, nil)
	for _, raw := range []string{"0", "-3", "abc"} {
		r := chiRequest(http.MethodGet, "/databases/"+raw+"/schema", map[string]string{"connID": raw})
		r = withUser(r, auth.User{ID: 1, IsActive: true})
		w := httptest.NewRecorder()
		h.schema(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("connID %q: status = %d, want %d", raw, w.Code, http.StatusNotFound)
		}
	}
}

func TestPolicyErrorIsServerError(t *testing.T) {
	h := newTestHandlers(t, nil, &fakePolicy{err: errors.New("db down")}, nil)
	r := chiRequest(http.MethodGet, "/databases/1/schema", map[string]string{"connID": "1"})
	r = withUser(r, auth.User{ID: 1, IsActive: true})
	w := httptest.NewRecorder()

	h.schema(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestViewOnlyDeniedOnEditRoutes(t *testing.T) {
	policy := &fakePolicy{byAction: map[access.Action]bool{
		access.ActionView: true,
		access.ActionEdit: false,
	}}
	h := newTestHandlers(t, nil, policy, nil)

	cases := []struct {
		name   string
		call   func(http.ResponseWriter, *http.Request)
		params map[string]string
		target string
	}{
		{"editor baru", h.migrationNew, map[string]string{"connID": "1"}, "/databases/1/migrations/new"},
		{"buat draft", h.migrationCreate, map[string]string{"connID": "1"}, "/databases/1/migrations"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := chiRequest(http.MethodGet, tc.target, tc.params)
			r = withUser(r, auth.User{ID: 4, IsActive: true})
			w := httptest.NewRecorder()
			tc.call(w, r)
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
			}
		})
	}

	if policy.lastAct != access.ActionEdit {
		t.Fatalf("aksi terakhir = %q, want edit", policy.lastAct)
	}
}

func TestSplitTableParam(t *testing.T) {
	cases := []struct {
		in    string
		ok    bool
		schema, table string
	}{
		{"public.users", true, "public", "users"},
		{"analytics.orders", true, "analytics", "orders"},
		{"users", false, "", ""},
		{"", false, "", ""},
		{"a.b.c", false, "", ""},
		{".x", false, "", ""},
		{"x.", false, "", ""},
	}
	for _, tc := range cases {
		s, tbl, ok := splitTableParam(tc.in)
		if ok != tc.ok || s != tc.schema || tbl != tc.table {
			t.Errorf("splitTableParam(%q) = (%q,%q,%v), want (%q,%q,%v)",
				tc.in, s, tbl, ok, tc.schema, tc.table, tc.ok)
		}
	}
}

func TestStubRegistryFailsClosed(t *testing.T) {
	if _, err := (StubRegistry{}).Get(context.Background(), 1); !errors.Is(err, ErrConnNotFound) {
		t.Fatalf("StubRegistry.Get err = %v, want ErrConnNotFound", err)
	}
}

// --- integration (test-db) --------------------------------------------------

func TestSchemaPageListsTables(t *testing.T) {
	pool := testutil.NewPool(t)
	// Fixture: schema + tabel di database platform itu sendiri (sebagai "target").
	if _, err := pool.Exec(context.Background(),
		`CREATE TABLE IF NOT EXISTS sample_items (id BIGSERIAL PRIMARY KEY, label TEXT NOT NULL)`); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	connID := insertConnection(t, pool, "schema-fixture", pool.Config().ConnString())

	h := newTestHandlers(t, pool, &fakePolicy{byAction: map[access.Action]bool{access.ActionView: true}},
		NewPoolRegistry(map[int64]*pgxpool.Pool{connID: pool}))
	r := chiRequest(http.MethodGet, "/databases/"+strconv.FormatInt(connID, 10)+"/schema",
		map[string]string{"connID": strconv.FormatInt(connID, 10)})
	r = withUser(r, auth.User{ID: 7, Username: "alice", IsActive: true})
	w := httptest.NewRecorder()

	h.schema(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "sample_items") {
		t.Errorf("schema page tidak menampilkan tabel sample_items")
	}
	if !strings.Contains(body, "Penjelajah Schema") {
		t.Errorf("schema page tidak menampilkan judul")
	}
}

func TestSchemaUnknownTableIsNotFound(t *testing.T) {
	pool := testutil.NewPool(t)
	connID := insertConnection(t, pool, "unknown-table", pool.Config().ConnString())

	h := newTestHandlers(t, pool, &fakePolicy{byAction: map[access.Action]bool{access.ActionView: true}},
		NewPoolRegistry(map[int64]*pgxpool.Pool{connID: pool}))
	target := "/databases/" + strconv.FormatInt(connID, 10) + "/schema?table=public.tidak_ada"
	r := chiRequest(http.MethodGet, target, map[string]string{"connID": strconv.FormatInt(connID, 10)})
	r.URL.RawQuery = "table=public.tidak_ada"
	r = withUser(r, auth.User{ID: 7, IsActive: true})
	w := httptest.NewRecorder()

	h.schema(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestTableInjectionRejected(t *testing.T) {
	pool := testutil.NewPool(t)
	if _, err := pool.Exec(context.Background(),
		`CREATE TABLE IF NOT EXISTS safe_tbl (id BIGSERIAL PRIMARY KEY)`); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	connID := insertConnection(t, pool, "inject-fixture", pool.Config().ConnString())

	h := newTestHandlers(t, pool, &fakePolicy{byAction: map[access.Action]bool{access.ActionView: true}},
		NewPoolRegistry(map[int64]*pgxpool.Pool{connID: pool}))

	// Nama tabel berbahaya harus ditolak lewat validasi catalog, bukan di-query.
	for _, bad := range []string{
		"public.safe_tbl;DROP_TABLE_x",
		"public.nonexistent",
		"no_dot_param",
	} {
		r := chiRequest(http.MethodGet,
			"/databases/x/tables/"+bad, map[string]string{"connID": strconv.FormatInt(connID, 10), "table": bad})
		r = withUser(r, auth.User{ID: 7, IsActive: true})
		w := httptest.NewRecorder()
		h.table(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("table %q: status = %d, want 404; body=%s", bad, w.Code, w.Body.String())
		}
	}
}

func TestMigrationExecuteSuccess(t *testing.T) {
	pool := testutil.NewPool(t)
	connID := insertConnection(t, pool, "exec-ok", pool.Config().ConnString())

	user := insertUser(t, pool, "creator-exec-ok")

	h := newTestHandlers(t, pool, &fakePolicy{byAction: map[access.Action]bool{
		access.ActionView: true, access.ActionEdit: true,
	}}, NewPoolRegistry(map[int64]*pgxpool.Pool{connID: pool}))

	m, err := h.store.CreateDraft(context.Background(), connID,
		"CREATE TABLE exec_ok_items (id BIGSERIAL PRIMARY KEY)", user)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	form := strings.NewReader("confirm_name=exec-ok&confirm_execute=on")
	r := httptest.NewRequest(http.MethodPost,
		"/databases/x/migrations/"+strconv.FormatInt(m.ID, 10)+"/execute", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("connID", strconv.FormatInt(connID, 10))
	rctx.URLParams.Add("mid", strconv.FormatInt(m.ID, 10))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = withUser(r, auth.User{ID: user, IsActive: true})
	w := httptest.NewRecorder()
	h.migrationExecute(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Location"), "status=executed") {
		t.Fatalf("Location = %q, want status=executed", w.Header().Get("Location"))
	}

	// Pastikan status applied dan tabel benar-benar terbuat di target.
	got, err := h.store.Get(context.Background(), m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusApplied {
		t.Fatalf("status = %q, want applied; error=%q", got.Status, got.Error)
	}
	var exists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'exec_ok_items')`).
		Scan(&exists); err != nil {
		t.Fatalf("cek tabel: %v", err)
	}
	if !exists {
		t.Fatal("tabel exec_ok_items tidak terbuat di target")
	}
	if got.ExecutedBy == nil || *got.ExecutedBy != user {
		t.Fatalf("executed_by = %v, want %d", got.ExecutedBy, user)
	}
}

func TestMigrationExecuteFailureRollsBack(t *testing.T) {
	pool := testutil.NewPool(t)
	connID := insertConnection(t, pool, "exec-fail", pool.Config().ConnString())

	user := insertUser(t, pool, "creator-exec-fail")

	h := newTestHandlers(t, pool, &fakePolicy{byAction: map[access.Action]bool{
		access.ActionView: true, access.ActionEdit: true,
	}}, NewPoolRegistry(map[int64]*pgxpool.Pool{connID: pool}))

	// SQL pertama valid, kedua gagal — rollback harus menghapus yang pertama.
	sqlText := "CREATE TABLE failprobe_items (id BIGSERIAL PRIMARY KEY); SELECT this_function_does_not_exist();"
	m, err := h.store.CreateDraft(context.Background(), connID, sqlText, user)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	form := strings.NewReader("confirm_name=exec-fail&confirm_execute=on")
	r := httptest.NewRequest(http.MethodPost,
		"/databases/x/migrations/"+strconv.FormatInt(m.ID, 10)+"/execute", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("connID", strconv.FormatInt(connID, 10))
	rctx.URLParams.Add("mid", strconv.FormatInt(m.ID, 10))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = withUser(r, auth.User{ID: user, IsActive: true})
	w := httptest.NewRecorder()

	h.migrationExecute(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	got, err := h.store.Get(context.Background(), m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.Error == "" {
		t.Fatal("pesan error kosong untuk migration gagal")
	}
	// Rollback terverifikasi: objek dari statement pertama tidak boleh ada.
	var exists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'failprobe_items')`).
		Scan(&exists); err != nil {
		t.Fatalf("cek tabel: %v", err)
	}
	if exists {
		t.Fatal("tabel failprobe_items masih ada — rollback gagal")
	}
}

func TestMigrationExecuteConfirmMismatch(t *testing.T) {
	pool := testutil.NewPool(t)
	connID := insertConnection(t, pool, "exec-confirm", pool.Config().ConnString())

	user := insertUser(t, pool, "creator-exec-confirm")

	h := newTestHandlers(t, pool, &fakePolicy{byAction: map[access.Action]bool{
		access.ActionView: true, access.ActionEdit: true,
	}}, NewPoolRegistry(map[int64]*pgxpool.Pool{connID: pool}))

	m, err := h.store.CreateDraft(context.Background(), connID, "SELECT 1", user)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	form := strings.NewReader("confirm_name=salah&confirm_execute=on")
	r := httptest.NewRequest(http.MethodPost,
		"/databases/x/migrations/"+strconv.FormatInt(m.ID, 10)+"/execute", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("connID", strconv.FormatInt(connID, 10))
	rctx.URLParams.Add("mid", strconv.FormatInt(m.ID, 10))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = withUser(r, auth.User{ID: user, IsActive: true})
	w := httptest.NewRecorder()

	h.migrationExecute(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	// Migration masih draft — tidak dieksekusi.
	got, _ := h.store.Get(context.Background(), m.ID)
	if got.Status != StatusDraft {
		t.Fatalf("status = %q, want draft", got.Status)
	}
}

func TestConnectionUnreachableShowsErrorPage(t *testing.T) {
	pool := testutil.NewPool(t)
	// Koneksi terdaftar di platform, tapi registry tidak punya pool-nya
	// (mensimulasikan database target yang tidak dapat dihubungi).
	connID := insertConnection(t, pool, "unreachable-fixture", "postgres://127.0.0.1:1/none")
	h := newTestHandlers(t, pool, &fakePolicy{byAction: map[access.Action]bool{access.ActionView: true}},
		NewPoolRegistry(map[int64]*pgxpool.Pool{}))

	r := chiRequest(http.MethodGet, "/databases/x/schema", map[string]string{"connID": strconv.FormatInt(connID, 10)})
	r = withUser(r, auth.User{ID: 7, IsActive: true})
	w := httptest.NewRecorder()

	h.schema(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Koneksi gagal") || !strings.Contains(body, "Coba lagi") {
		t.Errorf("halaman koneksi gagal tidak lengkap: %s", body)
	}
	// Pastikan kredensial tidak bocor ke halaman.
	if strings.Contains(body, "postgres://") {
		t.Fatal("connection URL bocor ke halaman error")
	}
}

func TestUnknownConnectionIsNotFound(t *testing.T) {
	pool := testutil.NewPool(t)
	h := newTestHandlers(t, pool, &fakePolicy{byAction: map[access.Action]bool{access.ActionView: true}},
		NewPoolRegistry(map[int64]*pgxpool.Pool{}))

	r := chiRequest(http.MethodGet, "/databases/999999/schema", map[string]string{"connID": "999999"})
	r = withUser(r, auth.User{ID: 7, IsActive: true})
	w := httptest.NewRecorder()

	h.schema(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestSummaryRedirectsToSchema(t *testing.T) {
	pool := testutil.NewPool(t)
	connID := insertConnection(t, pool, "summary-fixture", pool.Config().ConnString())

	h := newTestHandlers(t, pool, &fakePolicy{byAction: map[access.Action]bool{access.ActionView: true}},
		NewPoolRegistry(map[int64]*pgxpool.Pool{connID: pool}))

	r := chiRequest(http.MethodGet, "/databases/x", map[string]string{"connID": strconv.FormatInt(connID, 10)})
	r = withUser(r, auth.User{ID: 7, IsActive: true})
	w := httptest.NewRecorder()

	h.summary(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	want := "/databases/" + strconv.FormatInt(connID, 10) + "/schema"
	if loc := w.Header().Get("Location"); loc != want {
		t.Fatalf("Location = %q, want %q", loc, want)
	}
}

func TestMigrationListShowsDraftAndFilter(t *testing.T) {
	pool := testutil.NewPool(t)
	connID := insertConnection(t, pool, "list-fixture", pool.Config().ConnString())

	user := insertUser(t, pool, "creator-list-fixture")

	h := newTestHandlers(t, pool, &fakePolicy{byAction: map[access.Action]bool{
		access.ActionView: true, access.ActionEdit: true,
	}}, NewPoolRegistry(map[int64]*pgxpool.Pool{connID: pool}))

	if _, err := h.store.CreateDraft(context.Background(), connID, "-- Buat tabel contoh\nCREATE TABLE x (id BIGINT PRIMARY KEY)", user); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	base := "/databases/" + strconv.FormatInt(connID, 10) + "/migrations"
	r := chiRequest(http.MethodGet, base, map[string]string{"connID": strconv.FormatInt(connID, 10)})
	r = withUser(r, auth.User{ID: 7, IsActive: true})
	w := httptest.NewRecorder()
	h.migrationList(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Buat tabel contoh") {
		t.Error("nama migration (dari komentar SQL) tidak tampil")
	}
	if !strings.Contains(body, "Draft") {
		t.Error("badge draft tidak tampil")
	}

	// Filter status yang tidak cocok -> empty state jujur.
	r2 := chiRequest(http.MethodGet, base, map[string]string{"connID": strconv.FormatInt(connID, 10)})
	r2.URL.RawQuery = "status=applied"
	r2 = withUser(r2, auth.User{ID: 7, IsActive: true})
	w2 := httptest.NewRecorder()
	h.migrationList(w2, r2)
	if !strings.Contains(w2.Body.String(), "Tidak ada migrasi dengan status ini") {
		t.Error("filter tanpa hasil tidak menampilkan empty state")
	}
}

func TestEditDraftThenExecuteChecksumGuard(t *testing.T) {
	pool := testutil.NewPool(t)
	connID := insertConnection(t, pool, "checksum-fixture", pool.Config().ConnString())

	user := insertUser(t, pool, "creator-checksum-fixture")

	h := newTestHandlers(t, pool, &fakePolicy{byAction: map[access.Action]bool{
		access.ActionView: true, access.ActionEdit: true,
	}}, NewPoolRegistry(map[int64]*pgxpool.Pool{connID: pool}))

	m, err := h.store.CreateDraft(context.Background(), connID, "SELECT 1", user)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	// Ubah draft lewat handler (store), lalu eksekusi harus tetap cocok checksum.
	if err := h.store.UpdateDraft(context.Background(), m.ID, "SELECT 2"); err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}

	form := strings.NewReader("confirm_name=checksum-fixture&confirm_execute=on")
	r := httptest.NewRequest(http.MethodPost,
		"/databases/x/migrations/"+strconv.FormatInt(m.ID, 10)+"/execute", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("connID", strconv.FormatInt(connID, 10))
	rctx.URLParams.Add("mid", strconv.FormatInt(m.ID, 10))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = withUser(r, auth.User{ID: user, IsActive: true})
	w := httptest.NewRecorder()

	h.migrationExecute(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	got, _ := h.store.Get(context.Background(), m.ID)
	if got.Status != StatusApplied {
		t.Fatalf("status = %q, want applied (update draft lalu eksekusi harus valid)", got.Status)
	}
}
