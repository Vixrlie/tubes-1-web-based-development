package admin

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

// Option mengaktifkan satu bagian halaman admin. Tiap pemilik bagian buat
// Option sendiri di file-nya, jadi NewHandlers tidak perlu diubah lagi
type Option func(*Handlers)

// Handle layar administrasi
type Handlers struct {
	users       *userHandlers       // Arin
	groups      *groupHandlers      // Vincent
	connections *connectionHandlers // Majiid
	dashboard   *dashboardHandlers  // ringkasan /admin
}

func NewHandlers(opts ...Option) *Handlers {
	h := &Handlers{}
	for _, o := range opts {
		o(h)
	}
	return h
}

// Routes dipasang di bawah /admin dan dibungkus RequireAdmin.
func (h *Handlers) Routes(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		h.dashboardRoutes(r)   // ringkasan platform (/admin/)
		h.usersRoutes(r)       // Arin
		h.groupsRoutes(r)      // Vincent
		h.connectionsRoutes(r) // Majiid
		r.Get("/access", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/groups", http.StatusSeeOther)
		})
	})
}
