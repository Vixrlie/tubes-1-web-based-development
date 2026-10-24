package web

import "fmt"

// ---------------------------------------------------------------------------
// Kelola Koneksi (templates/admin/connections.html)
//
//	view.Render(w, "admin/connections.html", web.ConnectionsPage{
//	    Shell:       web.Shell{Nav: "admin", SideActive: "connections", User: u},
//	    PageHeader:  web.ConnectionsHeader("/admin/connections/new"),
//	    Connections: web.NewConnectionsTable(summaries),
//	})
// ---------------------------------------------------------------------------

// Status koneksi sebuah database (hasil tes koneksi terakhir).
const (
	ConnConnected = "connected"
	ConnFailed    = "failed"
)

// ConnectionStatusLabel mengubah status koneksi jadi teks + varian label,
// mis. ConnConnected -> {"Terhubung", "success"}. Dipakai di semua tempat yang
// menampilkan status koneksi supaya teks & warnanya seragam.
func ConnectionStatusLabel(status string) TableCell {
	if status == ConnFailed {
		return TableCell{Text: "Gagal", Variant: "error"}
	}
	return TableCell{Text: "Terhubung", Variant: "success"}
}

type ConnectionsPage struct {
	Shell       // Nav "admin", SideActive "connections"
	PageHeader  // "Kelola Koneksi" + tombol "Tambah Koneksi"
	Connections Table
}

// ConnectionSummary = satu baris di tabel Daftar Koneksi.
type ConnectionSummary struct {
	Name   string
	Host   string
	Port   int
	Status string   // ConnConnected (default) | ConnFailed
	Groups []string // nama group yang punya akses; kosong -> "-"
	Href   string   // halaman detail koneksi
}

// ConnectionsHeader = judul, breadcrumb, dan tombol "Tambah Koneksi" untuk halaman Kelola Koneksi.
func ConnectionsHeader(newHref string) PageHeader {
	return PageHeader{
		Title:   "Kelola Koneksi",
		Crumbs:  []Crumb{{Label: "Admin", Href: "/admin/users"}, {Label: "Kelola Koneksi"}},
		Actions: []Button{{Label: "Tambah Koneksi", IconLeft: "plus", Href: newHref}},
	}
}

// joinNames menggabungkan nama dengan koma; kosong -> "-".
func joinNames(names []string) string {
	if len(names) == 0 {
		return "-"
	}
	s := ""
	for i, n := range names {
		if i > 0 {
			s += ", "
		}
		s += n
	}
	return s
}

// NewConnectionsTable membuat tabel "Daftar Koneksi" dari ringkasan koneksi.
func NewConnectionsTable(conns []ConnectionSummary) Table {
	rows := make([]TableRow, 0, len(conns))
	for _, c := range conns {
		manage := OutlineButton("Kelola Koneksi", c.Href)
		manage.PaddingY = "var(--space-1)" // lebih ramping supaya baris tabel tidak membesar
		rows = append(rows, TableRow{
			"name":   {Text: c.Name},
			"host":   {Text: fmt.Sprintf("%s:%d", c.Host, c.Port)},
			"status": ConnectionStatusLabel(c.Status),
			"groups": {Text: joinNames(c.Groups)},
			"manage": {Button: &manage},
		})
	}
	return Table{
		Title: "Daftar Koneksi",
		Columns: []TableColumn{
			{Key: "name", Label: "Nama", Bold: true},
			{Key: "host", Label: "Host"},
			{Key: "status", Label: "Status"},
			{Key: "groups", Label: "Group dengan akses"},
			{Key: "manage", Label: "", Align: "right"},
		},
		Rows:  rows,
		Empty: "Belum ada koneksi.",
	}
}

// ---------------------------------------------------------------------------
// Detail Koneksi (templates/admin/connection_detail.html)
//
// Dua kartu berdampingan: "Parameter Koneksi" (Params) dan "Group dengan Akses"
// (Groups, hanya tampil; akses diatur dari halaman Grup). Tiga tombol di kepala
// halaman: Tes Koneksi (POST TestAction), Edit Koneksi (link EditHref), Hapus
// (POST DeleteAction, ada konfirmasi).
// ---------------------------------------------------------------------------

type ConnectionDetailPage struct {
	Shell      // Nav "admin", SideActive "connections"
	PageHeader // nama koneksi + tombol Tes / Edit / Hapus
	CSRFToken  string
	// TestAction = URL POST tes koneksi; DeleteAction = URL POST hapus koneksi.
	// Kosong -> form-nya tidak dirender (pasangan tombol: lihat ConnectionDetailHeader).
	TestAction   string
	DeleteAction string
	Params       DetailCard
	Groups       ListCard
}

// ID form tersembunyi; dirujuk tombol di kepala halaman lewat atribut form=.
const (
	testConnectionFormID   = "test-connection-form"
	deleteConnectionFormID = "delete-connection-form"
)

// DangerOutlineButton = tombol outline merah dengan latar putih (mis. "Hapus"),
// aksi destruktif yang tidak sebesar DangerButton.
func DangerOutlineButton(label string) Button {
	return Button{
		Label: label,
		Fill:  "var(--color-surface-raised)", Color: "var(--color-danger)", Stroke: "var(--color-danger)",
	}
}

// ConnectionDetailHeader = judul (nama koneksi), breadcrumb "Admin > Kelola Koneksi > <nama>",
// dan tombol Tes Koneksi / Edit Koneksi / Hapus. Tes dan Hapus men-submit form tersembunyi
// milik halaman (TestAction, DeleteAction).
func ConnectionDetailHeader(name, listHref, editHref string) PageHeader {
	// Tes & Edit: latar putih; saat hover latar mengikuti warna tulisan, tulisan jadi putih
	// (lihat .btn--hover-fill di button.css)
	test := Button{Label: "Tes Koneksi", Variant: "secondary", Type: "submit", Class: "btn--hover-fill",
		Attrs: `form="` + testConnectionFormID + `"`}
	edit := OutlineButton("Edit Koneksi", editHref).WithIcon("edit")
	edit.Class = "btn--hover-fill"
	del := DangerOutlineButton("Hapus").WithIcon("trash")
	del.Type = "submit"
	del.Attrs = `form="` + deleteConnectionFormID + `"`
	return PageHeader{
		Title: name,
		Crumbs: []Crumb{
			{Label: "Admin", Href: "/admin/users"},
			{Label: "Kelola Koneksi", Href: listHref},
			{Label: name},
		},
		Actions: []Button{test, edit, del},
	}
}

// ConnectionParams = parameter koneksi yang ditampilkan. Password TIDAK ada di sini:
// kartu selalu menampilkan titik-titik dan keterangan "tidak ditampilkan ulang".
type ConnectionParams struct {
	DisplayName string // nama tampilan
	Database    string // nama database di server
	Host        string
	Port        int
	Username    string
}

// NewConnectionParamsCard membuat kartu "Parameter Koneksi".
func NewConnectionParamsCard(p ConnectionParams) DetailCard {
	return DetailCard{
		Title: "Parameter Koneksi",
		Fields: []DetailField{
			{Label: "Nama tampilan", Value: p.DisplayName},
			{Label: "Nama database", Value: p.Database},
			{Label: "Host", Value: p.Host},
			{Label: "Port", Value: fmt.Sprint(p.Port)},
			{Label: "Username", Value: p.Username},
			{Label: "Password", Value: "••••••••••", Note: "tidak ditampilkan ulang"},
		},
	}
}

// ConnectionGroup = group yang punya akses ke koneksi ini (Level: AccessView | AccessEdit).
type ConnectionGroup struct {
	Name        string
	MemberCount int
	Level       string
}

// NewConnectionGroupsCard membuat kartu "Group dengan Akses" (hanya tampil).
func NewConnectionGroupsCard(groups []ConnectionGroup) ListCard {
	items := make([]ListItem, 0, len(groups))
	for _, g := range groups {
		items = append(items, ListItem{
			Title:    g.Name,
			Subtitle: fmt.Sprintf("%d anggota", g.MemberCount),
			Tag:      AccessLabel(g.Level),
		})
	}
	return ListCard{
		Title: "Group dengan Akses",
		Note:  "Diatur dari halaman Groups",
		Items: items,
		Empty: "Belum ada group yang punya akses.",
	}
}

// SetCSRF mengisi token csrf ke semua form di halaman sekaligus.
func (p *ConnectionDetailPage) SetCSRF(token string) {
	p.CSRFToken = token
	p.Groups.CSRFToken = token
}
