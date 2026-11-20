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

func (f *workflowFixture) api() http.Handler {
	h := NewHandlers(Config{
		Platform: f.pool,
		Policy:   access.NewPolicy(access.NewPGLevelSource(f.pool)),
		Workflow: f.wf,
	})
	r := chi.NewRouter()
	h.APIRoutes(r)
	return r
}

func (f *workflowFixture) call(h http.Handler, as auth.User, method, path, body string) (int, map[string]any) {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithUser(req.Context(), as))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		f.t.Fatalf("%s %s: respons bukan JSON objek: %q", method, path, rec.Body.String())
	}
	return rec.Code, out
}

func TestAPIMigrationWorkflow(t *testing.T) {
	f := newWorkflowFixture(t)
	h := f.api()
	base := fmt.Sprintf("/api/databases/%d", f.connID)

	code, body := f.call(h, f.editor, http.MethodPost, base+"/migrations",
		`{"name":"tambah tabel","up_sql":"CREATE TABLE api_t (id INT)","down_sql":"DROP TABLE api_t"}`)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, body)
	}
	migID := int64(body["id"].(float64))

	code, body = f.call(h, f.editor, http.MethodPost, base+"/migrations", `{"up_sql":"SELECT 1"}`)
	if code != http.StatusUnprocessableEntity || body["code"] != "invalid_input" {
		t.Errorf("create tanpa down: %d %v", code, body)
	}
	code, body = f.call(h, f.editor, http.MethodPost, base+"/migrations", `{"up_sql":"SELECT 1","extra":true}`)
	if code != http.StatusBadRequest {
		t.Errorf("field tak dikenal: %d %v", code, body)
	}

	mig := fmt.Sprintf("%s/migrations/%d", base, migID)
	code, body = f.call(h, f.editor, http.MethodPost, mig+"/requests", `{"action":"up"}`)
	if code != http.StatusConflict || body["code"] != "validation_required" {
		t.Errorf("submit sebelum validasi: %d %v", code, body)
	}
	if code, body = f.call(h, f.editor, http.MethodPost, mig+"/validate", `{}`); code != http.StatusOK || body["ok"] != true {
		t.Fatalf("validate: %d %v", code, body)
	}
	code, body = f.call(h, f.viewer, http.MethodPost, mig+"/requests", `{"action":"up"}`)
	if code != http.StatusForbidden {
		t.Errorf("viewer submit: %d %v", code, body)
	}
	code, body = f.call(h, f.editor, http.MethodPost, mig+"/requests", `{"action":"up","note":"siap"}`)
	if code != http.StatusCreated || body["status"] != RequestPendingReview {
		t.Fatalf("submit: %d %v", code, body)
	}
	reqPath := fmt.Sprintf("/api/migration-requests/%d", int64(body["id"].(float64)))

	code, body = f.call(h, f.editor, http.MethodPost, reqPath+"/approve", `{}`)
	if code != http.StatusForbidden || body["code"] != "not_reviewer" {
		t.Errorf("approve non-leader: %d %v", code, body)
	}
	if code, body = f.call(h, f.leaderA, http.MethodPost, reqPath+"/approve", `{}`); code != http.StatusOK {
		t.Fatalf("approve A: %d %v", code, body)
	}
	code, body = f.call(h, f.leaderB, http.MethodPost, reqPath+"/approve", `{"note":"ok"}`)
	if code != http.StatusAccepted || body["job_id"] == nil {
		t.Fatalf("approve terakhir harus 202 + job_id: %d %v", code, body)
	}

	f.drain()
	code, body = f.call(h, f.viewer, http.MethodGet, reqPath, "")
	if code != http.StatusOK || body["status"] != RequestSuccess {
		t.Fatalf("status akhir: %d %v", code, body)
	}

	code, body = f.call(h, f.outsider, http.MethodGet, base+"/history", "")
	if code != http.StatusForbidden {
		t.Errorf("history tanpa akses: %d %v", code, body)
	}
}
