package web

import (
	"fmt"
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
	r.Get("/dev/table", d.tableDetail)
	r.Get("/dev/groups", d.groups)
	r.Get("/dev/group", d.groupDetail)
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

// devTableDetail membuat data contoh halaman detail tabel untuk satu tab
// (TabColumns | TabConstraints | TabData; lainnya dianggap TabColumns).
func devTableDetail(tab string) TableDetailPage {
	c := func(t string) TableCell { return TableCell{Text: t} }
	if tab != TabConstraints && tab != TabData {
		tab = TabColumns
	}

	var tables []Table
	switch tab {
	case TabColumns:
		tables = []Table{{
			Title: "Column",
			Columns: []TableColumn{
				{Key: "name", Label: "Nama"},
				{Key: "type", Label: "Tipe"},
				{Key: "nullable", Label: "Nullable"},
				{Key: "default", Label: "Default"},
			},
			Rows: []TableRow{
				{"name": c("id"), "type": c("integer"), "nullable": c("NO"), "default": c("nextval()")},
				{"name": c("email"), "type": c("varchar(255)"), "nullable": c("NO"), "default": c("-")},
				{"name": c("created_at"), "type": c("timestamp"), "nullable": c("YES"), "default": c("now()")},
			},
		}}
	case TabConstraints:
		tables = []Table{
			{
				Title: "Constraint",
				Columns: []TableColumn{
					{Key: "name", Label: "Nama"},
					{Key: "kind", Label: "Jenis"},
					{Key: "cols", Label: "Kolom"},
				},
				Rows: []TableRow{
					{"name": c("users_pkey"), "kind": {Text: "PRIMARY KEY", Variant: "info"}, "cols": c("id")},
					{"name": c("users_email_key"), "kind": {Text: "UNIQUE", Variant: "neutral"}, "cols": c("email")},
				},
			},
			{
				Title: "Index",
				Columns: []TableColumn{
					{Key: "name", Label: "Nama"},
					{Key: "cols", Label: "Kolom"},
					{Key: "unique", Label: "Unik"},
				},
				Rows: []TableRow{
					{"name": c("users_pkey"), "cols": c("id"), "unique": c("Ya")},
					{"name": c("idx_users_created"), "cols": c("created_at"), "unique": c("Tidak")},
				},
			},
		}
	case TabData:
		tables = []Table{{
			Title: "Isi table",
			Tag:   TableCell{Text: "View-only", Variant: "neutral"},
			Columns: []TableColumn{
				{Key: "id", Label: "id", Sort: "asc", SortHref: "?tab=data&sort=id&dir=desc"},
				{Key: "email", Label: "email"},
				{Key: "created_at", Label: "created_at"},
			},
			Rows: []TableRow{
				{"id": c("1"), "email": c("ani@mail.com"), "created_at": c("2026-09-01 09:12")},
				{"id": c("2"), "email": c("budi@mail.com"), "created_at": c("2026-09-02 14:30")},
				{"id": c("3"), "email": c("citra@mail.com"), "created_at": c("2026-09-05 08:45")},
				{"id": c("4"), "email": c("dedi@mail.com"), "created_at": c("2026-09-07 17:03")},
			},
			Pager: NewPager("Menampilkan 1–4 dari 1.250 baris", 1, 133,
				func(p int) string { return fmt.Sprintf("?tab=data&page=%d", p) }),
		}}
	}

	return TableDetailPage{
		Shell: devShell("database", "table:public.users", true),
		Title: "Tabel users",
		Crumbs: []Crumb{
			{Label: "shop_db", Href: "/dev/databases"},
			{Label: "public", Href: "/dev/schema"},
			{Label: "users"},
		},
		Tabs:   NewTableTabs(tab, func(id string) string { return "?tab=" + id }),
		Tables: tables,
	}
}

// Halaman detail tabel dengan data contoh: /dev/table?tab=columns|constraints|data
func (d DevRoutes) tableDetail(w http.ResponseWriter, r *http.Request) {
	d.R.Render(w, "table_detail.html", devTableDetail(r.URL.Query().Get("tab")))
}

// devGroups membuat data contoh halaman Kelola Grup (dipakai route dev dan test).
func devGroups() GroupsPage {
	return GroupsPage{
		Shell:      devShell("admin", "groups", true),
		PageHeader: GroupsHeader("#"),
		Groups: NewGroupsTable([]GroupSummary{
			{Name: "Tim Backend", Href: "/dev/group", MemberCount: 3, Access: []GroupAccess{
				{Database: "shop_db", Level: AccessEdit}, {Database: "analytics_db", Level: AccessView}}},
			{Name: "Tim Data", Href: "/dev/group", MemberCount: 2, Access: []GroupAccess{
				{Database: "analytics_db", Level: AccessEdit}}},
			{Name: "Analis", Href: "/dev/group", MemberCount: 1, Access: []GroupAccess{
				{Database: "analytics_db", Level: AccessView}}},
		}),
	}
}

// Halaman Kelola Grup dengan data contoh: /dev/groups
func (d DevRoutes) groups(w http.ResponseWriter, r *http.Request) {
	d.R.Render(w, "admin/groups.html", devGroups())
}

// devGroupDetail membuat data contoh halaman Detail Grup. edit=false -> mode
// lihat; edit=true -> mode ubah akses (access-editor + action bar).
func devGroupDetail(edit bool) GroupDetailPage {
	p := GroupDetailPage{
		Shell:        devShell("admin", "groups", true),
		PageHeader:   GroupDetailHeader("Tim Backend", "/dev/groups"),
		DeleteAction: "#",
		Members: NewMembersCard([]Member{
			{Username: "budi", FullName: "Budi Santoso"},
			{Username: "siti", FullName: "Siti Rahma"},
			{Username: "algi", FullName: "Algi Pratama"},
		}, "#", func(i int) string { return "#" }),
		AddMembers: &AddMembersModal{
			FormAction: "#",
			Candidates: []MemberCandidate{
				{ID: "10", Username: "rudi", FullName: "Rudi Hartono"},
				{ID: "11", Username: "maya", FullName: "Maya Putri"},
				{ID: "12", Username: "dewi", FullName: "Dewi Lestari", Inactive: true},
			},
		},
	}
	if edit {
		p.Editor = &AccessEditor{
			FormAction: "#",
			CancelHref: "/dev/group",
			Rows: []AccessRow{
				{ID: "1", Name: "shop_db", Level: AccessEdit},
				{ID: "2", Name: "hospital_db", Level: AccessView},
				{ID: "3", Name: "analytics_db", Level: AccessView, Removed: true},
			},
			Available: []DropdownOption{
				{Value: "4", Label: "inventory_db"},
				{Value: "5", Label: "logs_db"},
			},
		}
	} else {
		p.Access = NewAccessCard([]ListItem{
			{Title: "shop_db", Subtitle: "PostgreSQL · Terhubung", Tag: AccessLabel(AccessEdit)},
			{Title: "analytics_db", Subtitle: "PostgreSQL · Terhubung", Tag: AccessLabel(AccessView)},
		}, "/dev/group?mode=edit")
	}
	p.SetCSRF("dev-token")
	return p
}

// Halaman Detail Grup dengan data contoh: /dev/group[?mode=edit]
func (d DevRoutes) groupDetail(w http.ResponseWriter, r *http.Request) {
	d.R.Render(w, "admin/group_detail.html", devGroupDetail(r.URL.Query().Get("mode") == "edit"))
}
