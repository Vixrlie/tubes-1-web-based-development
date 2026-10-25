package web

import "html/template"

// Button = data satu tombol untuk komponen button (templates/components/button.html).
// Nama field sama dengan parameter komponen, jadi bisa langsung dilempar dari
// data halaman: {{template `button` .}} atau lewat daftar ({{range .Actions}}).
//
// Hanya Label yang wajib; field kosong = pakai default komponen. Untuk
// tombol yang sering dipakai, pakai konstruktor di bawah supaya tampilan
// seragam di semua halaman:
//
//	web.OutlineButton("Tambah Anggota", "/admin/groups/1/members/new").WithIcon("plus")
//	web.DangerButton("Hapus Grup")
type Button struct {
	Label     string
	Variant   string // "primary" (default) | "secondary" | "tertiary" | "outline" | "soft-danger"
	State     string // "default" | "active" | "disabled"
	IconLeft  string
	IconRight string
	Type      string // "button" (default) | "submit" | "reset"
	Href      string // kalau diisi, dirender sebagai <a>
	ID        string
	Name      string
	Value     string
	Title     string
	AriaLabel string
	Class     string
	// Attrs = atribut mentah (hx-post, form=..., dst). HANYA dari literal di
	// kode, jangan dari input user.
	Attrs template.HTMLAttr

	// Custom warna / padding (lihat button.html).
	Fill, Color, Stroke         string
	Padding, PaddingX, PaddingY string
}

// WithIcon mengembalikan salinan tombol dengan icon di kiri.
func (b Button) WithIcon(name string) Button {
	b.IconLeft = name
	return b
}

// OutlineButton = tombol outline biru (mis. "Tambah Anggota", "Kelola Group").
// Kalau href kosong tombol dirender sebagai <button type="button">.
func OutlineButton(label, href string) Button {
	return Button{Label: label, Variant: "outline", Href: href}
}

// DangerButton = tombol merah solid untuk aksi destruktif (mis. "Hapus Grup").
func DangerButton(label string) Button {
	return Button{
		Label: label,
		Fill:  "var(--color-danger)", Color: "var(--color-on-danger)", Stroke: "var(--color-danger)",
	}
}

// SoftDangerButton = tombol kecil merah muda (mis. "Keluarkan", "Cabut").
func SoftDangerButton(label string) Button {
	return Button{Label: label, Variant: "soft-danger"}
}
