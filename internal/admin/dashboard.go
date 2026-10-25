package admin

// Halaman dashboard admin (/admin): ringkasan jumlah user, group, dan koneksi
// database platform + jalan pintas ke tiap halaman kelola.

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// GroupLister menyediakan jumlah group untuk dashboard.
type GroupLister interface {
	ListGroups(ctx context.Context) ([]Group, error)
}

type dashboardHandlers struct {
	users  UserLister
	groups GroupLister
	conns  ConnectionLister
	view   auth.Renderer
}

// WithDashboard mengaktifkan halaman ringkasan /admin
func WithDashboard(users UserLister, groups GroupLister, conns ConnectionLister, view auth.Renderer) Option {
	return func(h *Handlers) {
		h.dashboard = &dashboardHandlers{users: users, groups: groups, conns: conns, view: view}
	}
}

func (h *Handlers) dashboardRoutes(r chi.Router) {
	if h.dashboard == nil {
		return
	}
	r.Get("/", h.dashboard.index)
}

// dashboardPage = data templates/admin/dashboard.html fe-components.
type dashboardPage struct {
	web.Shell
	Crumbs []web.Crumb
	Stats  []web.DetailField
	Links  []web.Button
	Note   string
}

// statField mengubah hasil hitung menjadi baris ringkasan; galat -> "-".
func statField(label string, n int, note string, err error) web.DetailField {
	if err != nil {
		log.Printf("admin dashboard: hitung %s: %v", label, err)
		return web.DetailField{Label: label, Value: "-", Note: "gagal memuat"}
	}
	return web.DetailField{Label: label, Value: strconv.Itoa(n), Note: note}
}

func (d *dashboardHandlers) index(w http.ResponseWriter, r *http.Request) {
	me, _ := auth.UserFromContext(r.Context())

	stats := []web.DetailField{}
	if d.users != nil {
		users, err := d.users.List(r.Context())
		stats = append(stats, statField("User", len(users), "terdaftar di platform", err))
	}
	if d.groups != nil {
		groups, err := d.groups.ListGroups(r.Context())
		stats = append(stats, statField("Group", len(groups), "pembagian izin akses", err))
	}
	if d.conns != nil {
		conns, err := d.conns.ListConnections(r.Context())
		stats = append(stats, statField("Koneksi", len(conns), "database target terdaftar", err))
	}

	page := dashboardPage{
		Shell: web.Shell{
			Nav: "admin",
			User: &web.ShellUser{
				Username:        me.Username,
				IsPlatformAdmin: me.IsPlatformAdmin,
				CSRFToken:       session.CSRFToken(r.Context()),
			},
		},
		Crumbs: []web.Crumb{
			{Label: "Admin", Href: "/admin/users"},
			{Label: "Dashboard"},
		},
		Stats: stats,
		Links: []web.Button{
			{Label: "Kelola Pengguna", Href: "/admin/users"},
			{Label: "Kelola Grup", Href: "/admin/groups"},
			{Label: "Kelola Koneksi", Href: "/admin/connections"},
		},
		Note: "Perubahan apapun di halaman kelola tercatat di audit log platform (siapa, kapan, apa).",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	d.view.Render(w, "admin/dashboard.html", page)
}
