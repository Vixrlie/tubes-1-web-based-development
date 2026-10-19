package admin

import "github.com/go-chi/chi/v5"

// Handle layar administrasi
type Handlers struct{}

// Constructor untuk handler. Buat hubungin store & policy nanti
func NewHandlers() *Handlers {
	return &Handlers{}
}

// Router buat page admin
func (h *Handlers) Routes(r chi.Router) {}
