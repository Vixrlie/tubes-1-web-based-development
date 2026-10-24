package web

// Data untuk halaman detail tabel (templates/table_detail.html):
// tab Column | Constraint & Index | Data. Satu halaman, satu template;
// handler memilih isi `Tables` sesuai tab aktif:
//
//	Column             -> 1 tabel  (kolom penuh)
//	Constraint & Index -> 2 tabel  (berdampingan)
//	Data               -> 1 tabel  (dengan Tag, header sortable, Pager)
//
// Jumlah tabel menentukan tata letak (grid .split), jadi tab baru tidak
// perlu mengubah template.
//
//	view.Render(w, "table_detail.html", web.TableDetailPage{
//	    Shell:  web.Shell{Nav: "database", User: u, Sidebar: sb},
//	    Title:  "Tabel users",
//	    Crumbs: crumbs,
//	    Tabs:   web.NewTableTabs("columns", func(id string) string { return "?tab=" + id }),
//	    Tables: []web.Table{cols},
//	})
type TableDetailPage struct {
	Shell         // Nav "database" -> sidebar database + menu Database aktif
	Title  string // mis. "Tabel users"
	Crumbs []Crumb
	Tabs   []TabItem
	Tables []Table
}

// ID tab untuk NewTableTabs.
const (
	TabColumns     = "columns"
	TabConstraints = "constraints"
	TabData        = "data"
)

var tableTabDefs = []struct{ ID, Label string }{
	{TabColumns, "Column"},
	{TabConstraints, "Constraint & Index"},
	{TabData, "Data"},
}

// NewTableTabs membuat daftar tab; href(id) menentukan tujuan tiap tab.
func NewTableTabs(active string, href func(id string) string) []TabItem {
	tabs := make([]TabItem, 0, len(tableTabDefs))
	for _, d := range tableTabDefs {
		tabs = append(tabs, TabItem{Label: d.Label, Href: href(d.ID), Active: d.ID == active})
	}
	return tabs
}
