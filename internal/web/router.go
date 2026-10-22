package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Isinya route buat semua hal. Pake masing-masing feature packagenya (bisa tanpa
// ngedit shared file)
type Routes interface {
	Routes(r chi.Router)
}

// Base router
func NewRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Handle("/static/*", http.FileServer(http.FS(StaticFS)))
	return r
}
