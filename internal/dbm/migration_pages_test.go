package dbm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

func (f *workflowFixture) pages() http.Handler {
	renderer, err := web.NewRenderer()
	if err != nil {
		f.t.Fatal(err)
	}
	h := NewHandlers(Config{
		Platform: f.pool,
		Policy:   access.NewPolicy(access.NewPGLevelSource(f.pool)),
		Registry: fixedRegistry{target: f.wf.registry.(fixedRegistry).target},
		Renderer: renderer,
		Workflow: f.wf,
	})
	r := chi.NewRouter()
	h.Routes(r)
	h.ApprovalRoutes(r)
	return r
}

func (f *workflowFixture) page(h http.Handler, as auth.User, method, path string, form url.Values) *httptest.ResponseRecorder {
	f.t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req = req.WithContext(auth.WithUser(req.Context(), as))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "gagal render halaman") {
		f.t.Fatalf("%s %s: template gagal dirender", method, path)
	}
	return rec
}

func TestMigrationPagesRender(t *testing.T) {
	f := newWorkflowFixture(t)
	h := f.pages()
	m := f.create("halaman", "CREATE TABLE halaman (id INT)", "DROP TABLE halaman")
	req := f.approveAll(m.ID, ActionUp)
	f.drain()
	exec := f.request(req.ID).Executions[0]

	base := fmt.Sprintf("/databases/%d", f.connID)
	for _, path := range []string{
		base + "/migrations",
		base + "/migrations?hide_rollback=1",
		base + "/migrations/new",
		fmt.Sprintf("%s/migrations/%d", base, m.ID),
		fmt.Sprintf("%s/migrations/%d/preview", base, m.ID),
		fmt.Sprintf("%s/requests/%d", base, req.ID),
		fmt.Sprintf("%s/executions/%d", base, exec.ID),
		"/approvals",
	} {
		rec := f.page(h, f.editor, http.MethodGet, path, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status %d", path, rec.Code)
		}
	}

	// Riwayat menampilkan kolom sesuai desain dan checkbox Hide rollback.
	body := f.page(h, f.editor, http.MethodGet, base+"/migrations", nil).Body.String()
	for _, want := range []string{"Execution ID", "Migration / Action", "Actor", "Executed at", "Hide rollback", fmt.Sprintf("E%d", exec.ID)} {
		if !strings.Contains(body, want) {
			t.Errorf("halaman riwayat tidak memuat %q", want)
		}
	}

	// Form editor: galat validasi dirender ulang dengan input tetap terisi.
	rec := f.page(h, f.editor, http.MethodPost, base+"/migrations", url.Values{"up_sql": {"CREATE TABLE z (id INT)"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "CREATE TABLE z") {
		t.Errorf("editor tanpa down: status %d", rec.Code)
	}

	// Viewer tidak boleh membuka editor.
	if rec := f.page(h, f.viewer, http.MethodGet, base+"/migrations/new", nil); rec.Code != http.StatusForbidden {
		t.Errorf("viewer membuka editor: status %d", rec.Code)
	}

	// Pengajuan rollback lewat form berakhir di halaman review.
	rec = f.page(h, f.editor, http.MethodPost, fmt.Sprintf("%s/migrations/%d/requests", base, m.ID), url.Values{"action": {ActionRollback}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/requests/") {
		t.Errorf("submit rollback via form: status %d location %q", rec.Code, rec.Header().Get("Location"))
	}
}
