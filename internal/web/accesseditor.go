package web

import "fmt"

// Level akses sebuah grup ke satu database.
const (
	AccessView = "view"
	AccessEdit = "edit"
)

// AccessLabel mengubah level akses jadi teks + varian label,
// mis. AccessEdit -> {"View + Edit", "info"}. Dipakai di semua tempat yang
// menampilkan level akses supaya teks & warnanya seragam.
func AccessLabel(level string) TableCell {
	if level == AccessEdit {
		return TableCell{Text: "View + Edit", Variant: "info"}
	}
	return TableCell{Text: "View Only", Variant: "warning"}
}

// Tipe data untuk komponen access-editor (templates/components/access-editor.html):
// kartu "Akses Database" mode ubah. Perubahan dikumpulkan di sisi klien
// (static/js/access-editor.js) lalu dikirim sekali lewat tombol "Simpan Perubahan"
// di action bar. Tombol "Tambah Database" membuka pop up (pilih database + level akses);
// "Tambahkan" di pop up hanya menambah baris baru di daftar, belum menyimpan.
//
// Field yang dikirim form (POST FormAction):
//
//	csrf_token         token csrf
//	access:<ID>        level baru ("view" | "edit") untuk database lama yang tidak dicabut
//	remove             ID database yang dicabut (boleh berulang)
//	add_db, add_access pasangan database baru + levelnya (berulang, urutannya sejajar)
//
// Handler bebas membandingkan dengan data lama; yang tidak berubah tetap dikirim.

// AccessRow = satu database yang sudah dimiliki grup.
type AccessRow struct {
	ID      string // dipakai di nama field: access:<ID> dan remove
	Name    string
	Level   string // AccessView | AccessEdit
	Removed bool   // true -> baris tampil tercoret (mis. saat render ulang setelah galat)
}

type AccessEditor struct {
	Title      string // default "Akses Database"
	FormAction string
	CSRFToken  string
	Rows       []AccessRow
	// Available = database yang masih bisa ditambahkan ("Tambah Database").
	// Kosong -> tombol Tambah Database dinonaktifkan.
	Available []DropdownOption
	// CancelHref = tujuan tombol "Batal" di action bar (biasanya halaman detail mode lihat).
	CancelHref string
	// MessageFormat = teks action bar, "{n}" diganti jumlah perubahan.
	// Default "{n} perubahan akses belum disimpan".
	MessageFormat string
}

// LevelOptions = pilihan dropdown level akses.
func (AccessEditor) LevelOptions() []DropdownOption {
	return []DropdownOption{
		{Value: AccessView, Label: "View Only"},
		{Value: AccessEdit, Label: "View + Edit"},
	}
}

// LevelChoices = pilihan level akses versi kartu (judul + keterangan) untuk pop up
// "Tambah Database". Nilainya sama dengan LevelOptions.
func (AccessEditor) LevelChoices() []RadioOption {
	return []RadioOption{
		{Value: AccessView, Label: "View Only", Description: "Lihat schema, data, dan riwayat migration"},
		{Value: AccessEdit, Label: "View + Edit", Description: "Plus buat, ubah, dan jalankan migration"},
	}
}

// Message = teks awal action bar (sebelum JS menghitung perubahan).
func (e AccessEditor) Message() string {
	f := e.MessageFormat
	if f == "" {
		f = "{n} perubahan akses belum disimpan"
	}
	return f
}

// AccessSummary membuat ringkasan satu baris: "shop_db (View + Edit), analytics_db (View Only)".
func AccessSummary(items []GroupAccess) string {
	if len(items) == 0 {
		return "-"
	}
	s := ""
	for i, a := range items {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%s (%s)", a.Database, AccessLabel(a.Level).Text)
	}
	return s
}
