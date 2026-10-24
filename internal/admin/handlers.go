package admin

import (
	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

// Option mengaktifkan satu bagian halaman admin. Tiap pemilik bagian membuat
// Option sendiri di file-nya, jadi NewHandlers tidak perlu diubah lagi.
type Option func(*Handlers)

type Handlers struct {
	users       *userHandlers
	groups      *groupHandlers
	connections *connectionHandlers
}

func NewHandlers(opts ...Option) *Handlers {
	h := &Handlers{}
	for _, o := range opts {
		o(h)
	}
	return h
}

func (h *Handlers) Routes(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		h.usersRoutes(r)
		h.groupsRoutes(r)
		h.connectionsRoutes(r)
	})
}
