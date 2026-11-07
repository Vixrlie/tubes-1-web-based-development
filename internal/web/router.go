package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
)

// RequestID middleware: kasih ID unik tiap request, disimpan di context
// (via activity.WithRequestID) dan header X-Request-ID. Ini kunci
// observability — semua log aktivitas dari satu request bisa ditelusuri
// dengan ID yang sama.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := middleware.GetReqID(r.Context())
		ctx := activity.WithRequestID(r.Context(), reqID)
		w.Header().Set("X-Request-ID", reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Isinya route buat semua hal. Pake masing-masing feature packagenya (bisa tanpa
// ngedit shared file)
type Routes interface {
	Routes(r chi.Router)
}

// Base router
func NewRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID) // generate ID unik per request
	r.Use(RequestID)            // simpan ID ke context + header respons
	r.Use(middleware.Recoverer)
	r.Handle("/static/*", http.FileServer(http.FS(StaticFS)))
	return r
}
