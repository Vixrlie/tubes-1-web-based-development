package web

import (
	"fmt"
	"html/template"
	"strings"
)

// PageHeader = data kepala halaman (judul, breadcrumb, tombol aksi). Embed di
// struct halaman, lalu di template cukup: {{template `page-header` .PageHeader}}
type PageHeader struct {
	Title   string
	Crumbs  []Crumb
	Actions []Button
	// Tag = label kecil di kanan kepala halaman (opsional), mis. {Text: "Mode edit", Variant: "warning"}
	Tag *TableCell
}

// ---------------------------------------------------------------------------
// Kelola Grup (templates/admin/groups.html)
//
//	view.Render(w, "admin/groups.html", web.GroupsPage{
//	    Shell:      web.Shell{Nav: "admin", SideActive: "groups", User: u},
//	    PageHeader: web.GroupsHeader("/admin/groups/new"),
//	    Groups:     web.NewGroupsTable(summaries),
//	})
// ---------------------------------------------------------------------------

type GroupsPage struct {
	Shell      // Nav "admin", SideActive "groups"
	PageHeader // "Kelola Grup" + tombol "Grup Baru"
	Groups     Table
}

// GroupAccess = satu akses grup ke database (Level: AccessView | AccessEdit).
type GroupAccess struct {
	Database string
	Level    string
}

// GroupSummary = satu baris di tabel Daftar Grup.
type GroupSummary struct {
	Name        string
	Href        string // halaman detail grup
	MemberCount int
	Access      []GroupAccess
}

// GroupsHeader = judul, breadcrumb, dan tombol "Grup Baru" untuk halaman Kelola Grup.
func GroupsHeader(newHref string) PageHeader {
	return PageHeader{
		Title:   "Kelola Grup",
		Crumbs:  []Crumb{{Label: "Admin", Href: "/admin/users"}, {Label: "Kelola Grup"}},
		Actions: []Button{{Label: "Grup Baru", IconLeft: "plus", Href: newHref}},
	}
}

// NewGroupsTable membuat tabel "Daftar Grup" dari ringkasan grup.
func NewGroupsTable(groups []GroupSummary) Table {
	rows := make([]TableRow, 0, len(groups))
	for _, g := range groups {
		manage := OutlineButton("Kelola Group", g.Href)
		manage.PaddingY = "var(--space-1)" // lebih ramping supaya baris tabel tidak membesar
		rows = append(rows, TableRow{
			"name":    {Text: g.Name},
			"members": {Text: fmt.Sprintf("%d anggota", g.MemberCount)},
			"access":  {Text: AccessSummary(g.Access)},
			"manage":  {Button: &manage},
		})
	}
	return Table{
		Title: "Daftar Grup",
		Columns: []TableColumn{
			{Key: "name", Label: "Group", Bold: true},
			{Key: "members", Label: "Anggota"},
			{Key: "access", Label: "Akses database"},
			{Key: "manage", Label: "", Align: "right"},
		},
		Rows:  rows,
		Empty: "Belum ada grup.",
	}
}

// ---------------------------------------------------------------------------
// Detail Grup (templates/admin/group_detail.html)
//
// Satu halaman, dua mode:
//   - lihat : Editor == nil -> kartu "Akses Database" (Access) + tombol "Kelola Database"
//   - ubah  : Editor != nil -> kartu diganti access-editor + action bar "Simpan Perubahan"
//
// Dua pop up: "Tambah Anggota" (AddMembers, kedua mode) dan "Tambah Database" (bagian dari
// access-editor, hanya mode ubah).
// ---------------------------------------------------------------------------

type GroupDetailPage struct {
	Shell      // Nav "admin", SideActive "groups"
	PageHeader // "Grup <nama>" + tombol "Hapus Grup"
	CSRFToken  string
	// DeleteAction = URL POST hapus grup. Kosong -> form hapus tidak dirender
	// (pasangan tombol: DeleteGroupButton()).
	DeleteAction string
	Members      ListCard
	Access       ListCard
	Editor       *AccessEditor
	// AddMembers = data pop up "Tambah Anggota" (tombol pembukanya dibuat NewMembersCard).
	// nil -> pop up tidak dirender dan tombol kembali menjadi link biasa ke addHref.
	AddMembers *AddMembersModal
}

// ID form tersembunyi untuk hapus grup; dirujuk tombol lewat atribut form=.
const deleteGroupFormID = "delete-group-form"

// DeleteGroupButton = tombol "Hapus Grup" untuk PageHeader.Actions; men-submit
// form hapus (DeleteAction) milik halaman.
func DeleteGroupButton() Button {
	b := DangerButton("Hapus Grup")
	b.Type = "submit"
	b.Attrs = `form="` + deleteGroupFormID + `"`
	return b
}

// GroupDetailHeader = judul + breadcrumb "Admin > Kelola Grup > <nama>".
func GroupDetailHeader(name, groupsHref string) PageHeader {
	return PageHeader{
		Title: "Grup " + name,
		Crumbs: []Crumb{
			{Label: "Admin", Href: "/admin/users"},
			{Label: "Kelola Grup", Href: groupsHref},
			{Label: name},
		},
		Actions: []Button{DeleteGroupButton()},
	}
}

// ID <dialog> pop up "Tambah Anggota"; dirujuk tombol lewat atribut data-modal-open.
const addMembersModalID = "add-members-modal"

// MemberCandidate = pengguna yang bisa dipilih di pop up "Tambah Anggota".
// Inactive -> tampil redup bertanda "(nonaktif)" dan tidak bisa dicentang.
type MemberCandidate struct {
	ID       string // nilai field user_id
	Username string
	FullName string
	Inactive bool
}

func (c MemberCandidate) DisplayName() string {
	if c.FullName != "" {
		return c.FullName
	}
	return c.Username
}

// SearchKey = teks huruf kecil (nama + username) yang dicocokkan kotak cari di pop up.
func (c MemberCandidate) SearchKey() string {
	return strings.ToLower(c.FullName + " " + c.Username)
}

// AddMembersModal = data pop up "Tambah Anggota" (templates/components/add-members-modal.html).
//
//	page.AddMembers = &web.AddMembersModal{
//	    FormAction: "/admin/groups/1/members",
//	    Candidates: candidates, // pengguna yang BELUM jadi anggota grup ini
//	}
//	page.SetCSRF(token)
//
// Field yang dikirim form (POST FormAction):
//
//	csrf_token  token csrf
//	user_id     ID pengguna yang dipilih (berulang, satu per anggota)
//
// Handler wajib memvalidasi ulang di server (pengguna ada, aktif, belum jadi anggota);
// kotak centang yang dinonaktifkan di UI bukan jaminan keamanan.
type AddMembersModal struct {
	FormAction string
	CSRFToken  string
	Candidates []MemberCandidate
}

// NewMembersCard membuat kartu "Anggota (n)". removeAction(i) = URL POST untuk
// mengeluarkan anggota ke-i. Tombol "Tambah Anggota" membuka pop up AddMembers
// (data-modal-open); addHref hanya cadangan kalau JS mati / pop up tidak dirender.
func NewMembersCard(members []Member, addHref string, removeAction func(i int) string) ListCard {
	items := make([]ListItem, 0, len(members))
	for i, m := range members {
		item := ListItem{
			Title: m.DisplayName(),
			Action: &ListAction{Label: "Keluarkan", FormAction: removeAction(i),
				Confirm: "Keluarkan " + m.DisplayName() + " dari grup ini?"},
		}
		if m.FullName != "" {
			item.Subtitle = m.Username // tanpa nama lengkap, username sudah jadi judul
		}
		items = append(items, item)
	}
	add := OutlineButton("Tambah Anggota", addHref).WithIcon("plus")
	add.Attrs = template.HTMLAttr(`data-modal-open="` + addMembersModalID + `"`)
	return ListCard{
		Title:   fmt.Sprintf("Anggota (%d)", len(members)),
		Actions: []Button{add},
		Items:   items,
		Empty:   "Belum ada anggota.",
	}
}

// NewAccessCard membuat kartu "Akses Database" (mode lihat). Status koneksi
// dan engine ditulis handler di Subtitle, mis. "PostgreSQL · Terhubung".
func NewAccessCard(items []ListItem, manageHref string) ListCard {
	return ListCard{
		Title:   "Akses Database",
		Actions: []Button{OutlineButton("Kelola Database", manageHref)},
		Items:   items,
		Empty:   "Grup ini belum punya akses database.",
	}
}

// Member = anggota grup yang ditampilkan.
type Member struct {
	Username string
	FullName string
}

func (m Member) DisplayName() string {
	if m.FullName != "" {
		return m.FullName
	}
	return m.Username
}

// SetCSRF mengisi token csrf ke semua form di halaman sekaligus.
func (p *GroupDetailPage) SetCSRF(token string) {
	p.CSRFToken = token
	p.Members.CSRFToken = token
	p.Access.CSRFToken = token
	if p.Editor != nil {
		p.Editor.CSRFToken = token
	}
	if p.AddMembers != nil {
		p.AddMembers.CSRFToken = token
	}
}
