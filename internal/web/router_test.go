package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
)

func TestRequestIDMiddlewareHeaderAndContext(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(RequestID)

	var contextRequestID string
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		contextRequestID = activity.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusNoContent)
	}
	headerRequestID := res.Header().Get("X-Request-ID")
	if headerRequestID == "" {
		t.Fatal("X-Request-ID response header is empty")
	}
	if contextRequestID == "" {
		t.Fatal("request ID was not added to request context")
	}
	if contextRequestID != headerRequestID {
		t.Fatalf("context request ID %q does not match response header %q", contextRequestID, headerRequestID)
	}
}
