package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSPAHandler(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><div id=\"root\">SPA</div>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := SPAHandler(dir)

	t.Run("serves static asset directly", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "console.log('ok')") {
			t.Fatalf("got %d %q", rec.Code, rec.Body.String())
		}
	})

	t.Run("falls back to index.html for SPA client routes", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/users/42", nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<div id=\"root\">SPA</div>") {
			t.Fatalf("got %d %q", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("expected Cache-Control: no-store, got %q", rec.Header().Get("Cache-Control"))
		}
	})

	t.Run("never falls back to HTML for unknown /api routes", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/unknown", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404", rec.Code)
		}
		if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("expected JSON content type, got %q", rec.Header().Get("Content-Type"))
		}
	})

	t.Run("returns 503 hint when dist build is missing", func(t *testing.T) {
		missing := SPAHandler(filepath.Join(dir, "nonexistent"))
		rec := httptest.NewRecorder()
		missing.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("got %d, want 503", rec.Code)
		}
	})
}
