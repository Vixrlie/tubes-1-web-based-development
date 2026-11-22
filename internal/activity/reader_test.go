package activity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLokiReaderRealJSONContract(t *testing.T) {
	stamp := "1795344000000000000"
	line := `{"time":"2026-11-22T12:00:00Z","action":"sql.result","status":"failed","user_id":"42","request_id":"req-real","query":"SELECT secret FROM users","parameters":"private-value","password":"private-password","sqlstate":"42501","error":"private dsn"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" || r.URL.Query().Get("direction") != "backward" {
			t.Errorf("unexpected Loki path")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "streams", "result": []any{map[string]any{"stream": map[string]string{"service_name": "pathgres"}, "values": [][]string{{stamp, line}}}}}})
	}))
	defer server.Close()
	reader, err := NewLokiReader(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	f := ActivityFilter{Start: time.Unix(0, 1), End: time.Now(), Page: 999, PageSize: 8}
	page, err := reader.List(context.Background(), f)
	if err != nil || page.Total != 1 || page.Page != 1 {
		t.Fatalf("page: %+v, %v", page, err)
	}
	row := page.Items[0]
	if row.Detail["query"] != "[redacted]" || row.Detail["parameters"] != "[redacted]" || row.Detail["error"] != "operation failed" {
		t.Fatalf("unsafe detail: %+v", row.Detail)
	}
	if _, ok := row.Detail["password"]; ok {
		t.Fatal("password leaked")
	}
	detail, err := reader.Get(context.Background(), row.ID)
	if err != nil || detail.ID != row.ID {
		t.Fatalf("detail: %v", err)
	}
	if _, err := reader.Get(context.Background(), "bad-id"); !errors.Is(err, ErrActivityNotFound) {
		t.Fatal(err)
	}
}

func TestLokiQueryEscapesLiteralSearch(t *testing.T) {
	query := activityQuery(ActivityFilter{Search: `" } |= "secret`, Status: "failed", Category: "sql"})
	if !strings.Contains(query, `status="failed"`) || !strings.Contains(query, `action=~"sql\\..*"`) || !strings.Contains(query, `\"`) {
		t.Fatalf("unexpected query: %s", query)
	}
}

func TestLokiReaderUnavailableAndNoRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.invalid", http.StatusFound)
	}))
	defer server.Close()
	reader, _ := NewLokiReader(server.URL)
	_, err := reader.List(context.Background(), ActivityFilter{Start: time.Now().Add(-time.Hour), End: time.Now(), Page: 1, PageSize: 8})
	if !errors.Is(err, ErrActivityUnavailable) {
		t.Fatalf("redirect must not be followed: %v", err)
	}
}
