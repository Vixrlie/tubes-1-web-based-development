package web

import "html/template"

// Data untuk halaman "Daftar Database" (templates/databases.html).
//
// Handler (BE) mengisi DatabasesPage lalu memanggil:
//
//	view.Render(w, "databases.html", web.DatabasesPage{
//	    Shell: web.Shell{Nav: "databases", User: shellUser},
//	    Cards: cards,
//	})

// DBCard = data satu kartu database. Nama field sama dengan parameter
// komponen db-card, jadi bisa langsung dilempar: {{template "db-card" .}}.
type DBCard struct {
	Name       string
	Meta       string            // mis. "PostgreSQL · 1 schema · 7 tabel"
	Status     string            // "connected" (default) | "failed"
	Access     string            // "view" (default) | "edit"
	Href       string            // tujuan tombol "Buka"
	Error      string            // hanya dipakai saat Status == "failed"
	RetryHref  string            // tujuan tombol "Coba lagi" (opsional)
	RetryAttrs template.HTMLAttr // atribut mentah "Coba lagi", mis. hx-post (opsional)
}

// DatabasesPage = seluruh data halaman.
type DatabasesPage struct {
	// Nav "databases" -> header menyorot menu Database, tanpa sidebar.
	Shell
	Cards []DBCard
	Empty string // pesan kalau Cards kosong (default ada di template)
}
