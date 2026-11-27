package admin

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

// Option enables a section of the admin interface or its dependencies.
type Option func(*Handlers)

// Handlers handles administration screens.
type Handlers struct {
	users       *userHandlers
	groups      *groupHandlers
	connections *connectionHandlers
	dashboard   *dashboardHandlers
	logger      *activity.Logger
}

func WithActivityLogger(logger *activity.Logger) Option {
	return func(h *Handlers) { h.logger = logger }
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
		r.Use(AdminActivityMiddleware(h.logger))

		h.dashboardRoutes(r)
		h.usersRoutes(r)
		h.groupsRoutes(r)
		h.connectionsRoutes(r)

		r.Get("/access", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/groups", http.StatusSeeOther)
		})
	})
}
