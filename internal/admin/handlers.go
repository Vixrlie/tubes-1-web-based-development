package admin

import (
	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

// Handle layar administrasi
type Handlers struct{}

func NewHandlers() *Handlers {
	return &Handlers{}
}

// Routes dipasang di bawah /admin dan dibungkus RequireAdmin.
func (h *Handlers) Routes(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		h.usersRoutes(r)       // Arin
		h.groupsRoutes(r)      // Vincent
		h.connectionsRoutes(r) // Majiid
	})
}
