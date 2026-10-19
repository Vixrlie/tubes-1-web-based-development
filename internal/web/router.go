package web

import "github.com/go-chi/chi/v5"

// Isinya route buat semua hal. Pake masing-masing feature packagenya (bisa tanpa
// ngedit shared file)
type Routes interface {
	Routes(r chi.Router)
}

// Base router
func NewRouter() chi.Router {
	return chi.NewRouter()
}
