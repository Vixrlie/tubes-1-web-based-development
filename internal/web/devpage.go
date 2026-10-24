package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// DevRoutes: halaman uji komponen. Pasang HANYA saat development.
type DevRoutes struct{ R *Renderer }

func (d DevRoutes) Routes(r chi.Router) {
	r.Get("/dev/components", d.components)
}

type devData struct {
	Shell
	Avatars []*ShellUser
}

func devAvatars() []*ShellUser {
	return []*ShellUser{
		{Username: "arin", FullName: "Arin Dev"}, // inisial dari FullName
		{Username: "budi"},                       // inisial dari Username
	}
}

// Shell palsu supaya header + sidebar bisa dites tanpa login.
// nav: "none" | "admin" | "database"
func devShell(nav, active string, admin bool) Shell {
	s := Shell{Nav: nav}
	if nav != "admin" && nav != "database" {
		return s // User nil -> tanpa header & sidebar
	}
	s.User = &ShellUser{Username: "arin", FullName: "Arin Dev", IsPlatformAdmin: admin}
	if nav == "admin" {
		s.SideActive = active // users | groups | connections
	} else {
		s.Sidebar = devSidebar(active)
	}
	return s
}

func devSidebar(active string) *SidebarDB {
	href := func(a string) string { return "?nav=database&active=" + a }
	schema := func(name string, tables ...string) SidebarSchema {
		sc := SidebarSchema{Name: name, Href: href("schema:" + name), Active: active == "schema:"+name}
		for _, t := range tables {
			id := "table:" + name + "." + t
			sc.Tables = append(sc.Tables, SidebarTable{Name: t, Href: href(id), Active: active == id})
		}
		return sc
	}
	return &SidebarDB{
		Name: "nama_database",
		Schemas: []SidebarSchema{
			schema("public", "users", "orders", "products"),
			schema("analytics", "events"),
		},
		MigrationsHref:   href("migrations"),
		MigrationsActive: active == "migrations",
	}
}

func (d DevRoutes) components(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	d.R.Render(w, "dev/components.html", devData{
		Shell:   devShell(q.Get("nav"), q.Get("active"), q.Get("admin") != "0"),
		Avatars: devAvatars(),
	})
}
