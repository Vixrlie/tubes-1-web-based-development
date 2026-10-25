package web

import "html/template"

// InputField = data satu input untuk komponen input (templates/components/input.html).
// Nama field sama dengan parameter komponen, jadi bisa langsung dilempar:
//
//	{{range .Fields}}{{template `input` .}}{{end}}
//
// Hanya Name yang hampir selalu perlu; field kosong = pakai default komponen.
type InputField struct {
	Label       string
	Name        string
	ID          string // default = Name
	Type        string // "text" (default) | "email" | "password" | "number" | ...
	Value       string
	Placeholder string
	Helper      string
	Error       string // diisi -> field merah + pesan ini menggantikan Helper
	State       string // "default" | "disabled"
	Required    bool
	IconLeft    string
	IconRight   string
	// PasswordToggle = tombol tampil / sembunyikan password di ujung kanan.
	PasswordToggle bool
	Class          string // class tambahan; "is-error" = merah tanpa pesan
	// Attrs = atribut mentah (autocomplete, min, max, dst). HANYA dari literal di
	// kode, jangan dari input user.
	Attrs template.HTMLAttr
}
