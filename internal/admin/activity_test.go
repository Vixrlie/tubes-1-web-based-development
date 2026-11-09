package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
)

func TestAdminActivityLogsSuccess(t *testing.T) {
	var output bytes.Buffer
	r := chi.NewRouter()
	r.Use(AdminActivityMiddleware(activity.NewWithWriter(&output)))
	r.Post("/admin/users/{id}/activate", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusSeeOther)
	})

	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/admin/users/42/activate", nil))
	if res.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", res.Code)
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON log: %v; output=%q", err, output.String())
	}
	for key, want := range map[string]string{
		"action": "admin.user.activate", "status": activity.StatusSuccess,
		"resource_id": "42", "http_status": "303",
	} {
		if entry[key] != want {
			t.Errorf("%s = %v, want %q", key, entry[key], want)
		}
	}
}

func TestAdminActivityLogsFailure(t *testing.T) {
	var output bytes.Buffer
	r := chi.NewRouter()
	r.Use(AdminActivityMiddleware(activity.NewWithWriter(&output)))
	r.Post("/admin/users", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal failure", http.StatusInternalServerError)
	})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/admin/users", nil))
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON log: %v; output=%q", err, output.String())
	}
	if entry["status"] != activity.StatusFailed || entry["action"] != "admin.user.create" || entry["level"] != "ERROR" {
		t.Errorf("unexpected failure event: %+v", entry)
	}
	if strings.Contains(output.String(), "internal failure") {
		t.Fatal("raw internal error leaked into activity log")
	}
}

func TestAdminActivitySkipsGET(t *testing.T) {
	var output bytes.Buffer
	r := chi.NewRouter()
	r.Use(AdminActivityMiddleware(activity.NewWithWriter(&output)))
	r.Get("/admin/users", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/admin/users", nil))
	if output.Len() != 0 {
		t.Fatalf("GET should not generate mutation log: %s", output.String())
	}
}

func TestAdminActivityDoesNotLogBody(t *testing.T) {
	var output bytes.Buffer
	r := chi.NewRouter()
	r.Use(AdminActivityMiddleware(activity.NewWithWriter(&output)))
	r.Post("/admin/users/{id}/password", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusSeeOther)
	})
	secret := "top-secret-password"
	req := httptest.NewRequest(http.MethodPost, "/admin/users/42/password", strings.NewReader("new_password="+secret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if strings.Contains(output.String(), secret) {
		t.Fatal("password leaked into activity log")
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if entry["action"] != "admin.user.reset_password" {
		t.Errorf("unexpected action: %v", entry["action"])
	}
}
