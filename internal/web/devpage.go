package web

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

// DevRoutes: halaman uji komponen. Pasang HANYA saat development.
type DevRoutes struct{ R *Renderer }

func (d DevRoutes) Routes(r chi.Router) {
	r.Get("/dev/components", d.components)
	r.Get("/dev/databases", d.databases)
	r.Get("/dev/schema", d.schema)
}

type devData struct {
	Shell
	Avatars []*ShellUser
	Table   Table
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
	if nav != "admin" && nav != "database" && nav != "databases" {
		return s // User nil -> tanpa header & sidebar
	}
	s.User = &ShellUser{Username: "arin", FullName: "Arin Dev", IsPlatformAdmin: admin}
	switch nav {
	case "admin":
		s.SideActive = active // users | groups | connections
	case "database":
		s.Sidebar = devSidebar(active)
	}
	return s
}

func devSidebar(active string) *SidebarDB {
	// QueryEscape: ":" di "table:public.orders" jadi %3A. Tanpa ini html/template
	// menganggap "...active=table" sebagai skema URL dan mengganti href jadi #ZgotmplZ.
	href := func(a string) string { return "?nav=database&active=" + url.QueryEscape(a) }
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
		Table:   devTable(),
	})
}

// Data contoh untuk tabel di halaman dev.
func devTable() Table {
	c := func(t string) TableCell { return TableCell{Text: t} }
	row := func(name, hp, conn, connV, acc, accV, group string) TableRow {
		return TableRow{
			"name":   c(name),
			"host":   c(hp),
			"engine": c("PostgreSQL"),
			"conn":   {Text: conn, Variant: connV},
			"access": {Text: acc, Variant: accV},
			"group":  c(group),
		}
	}
	return Table{
		Title: "Daftar Tabel",
		Columns: []TableColumn{
			{Key: "name", Label: "Nama", Bold: true},
			{Key: "host", Label: "Host : Port"},
			{Key: "engine", Label: "Engine"},
			{Key: "conn", Label: "Koneksi"},
			{Key: "access", Label: "Akses"},
			{Key: "group", Label: "Nama Grup"},
		},
		Rows: []TableRow{
			row("hr_db", "10.0.0.6 : 5432", "Terhubung", "success", "View Only", "neutral", "Tim Analis"),
			row("shop_db", "10.0.0.6 : 5432", "Terhubung", "success", "View + Edit", "info", "Tim Backend"),
			row("log_db", "10.0.0.7 : 5432", "Gagal terhubung", "error", "View Only", "neutral", "Tim Analisis"),
		},
	}
}

// devDatabases membuat data contoh halaman Daftar Database (dipakai route dev dan test).
// Banyak kartu supaya terlihat perilaku scroll.
func devDatabases() DatabasesPage {
	var cards []DBCard
	for i := 0; i < 2; i++ {
		cards = append(cards,
			DBCard{Name: "inventory_db", Meta: "PostgreSQL · 1 schema · 7 tabel", Access: "edit", Href: "#"},
			DBCard{Name: "shop_db", Meta: "PostgreSQL · 2 schema · 12 tabel", Access: "edit", Href: "#"},
			DBCard{Name: "logs_db", Meta: "PostgreSQL · 1 schema · 3 tabel", Access: "view", Href: "#"},
			DBCard{Name: "hr_db", Meta: "PostgreSQL", Access: "view", Status: "failed",
				Error: "Tidak dapat terhubung ke database. Coba lagi beberapa saat.", RetryHref: "#"},
		)
	}
	return DatabasesPage{Shell: devShell("databases", "", true), Cards: cards}
}

// Halaman Daftar Database dengan data contoh: /dev/databases
func (d DevRoutes) databases(w http.ResponseWriter, r *http.Request) {
	d.R.Render(w, "databases.html", devDatabases())
}

// devSchema membuat data contoh halaman Schema (dipakai route dev dan test).
func devSchema() SchemaPage {
	c := func(t string) TableCell { return TableCell{Text: t} }
	return SchemaPage{
		Shell: devShell("database", "schema:public", true),
		Title: "Schema public",
		Crumbs: []Crumb{
			{Label: "shop_db", Href: "/dev/databases"},
			{Label: "public"},
		},
		Tables: Table{
			Title: "Daftar Tabel",
			Columns: []TableColumn{
				{Key: "name", Label: "Nama", Bold: true},
				{Key: "cols", Label: "Kolom"},
				{Key: "rows", Label: "Perkiraan Baris"},
				{Key: "pk", Label: "Primary Key"},
			},
			Rows: []TableRow{
				{"name": c("users"), "cols": c("5"), "rows": c("1.240"), "pk": c("id")},
				{"name": c("orders"), "cols": c("7"), "rows": c("8.412"), "pk": c("id")},
				{"name": c("products"), "cols": c("6"), "rows": c("320"), "pk": c("id")},
			},
		},
		Relations: Table{
			Title: "Relasi (Foreign Key)",
			Columns: []TableColumn{
				{Key: "name", Label: "Nama"},
				{Key: "from", Label: "Dari"},
				{Key: "to", Label: "Mengacu ke"},
			},
			Rows: []TableRow{
				{"name": c("orders_user_id_fkey"), "from": c("orders.user_id"), "to": {Text: "users.id", Variant: "info"}},
				{"name": c("orders_product_id_fkey"), "from": c("orders.product_id"), "to": {Text: "products.id", Variant: "info"}},
			},
		},
	}
}

// Halaman Schema dengan data contoh: /dev/schema
func (d DevRoutes) schema(w http.ResponseWriter, r *http.Request) {
	d.R.Render(w, "schema.html", devSchema())
}
