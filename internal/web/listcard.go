package web

// Tipe data untuk komponen list-card (templates/components/list-card.html):
// kartu berisi judul, tombol aksi di kepala, dan daftar baris (nama + keterangan
// kecil, label opsional, tombol aksi opsional). Dipakai mis. untuk "Anggota"
// dan "Akses Database" di detail grup, tapi netral: cocok untuk daftar mana pun.
//
//	card := web.ListCard{
//	    Title:   "Anggota (3)",
//	    Actions: []web.Button{web.OutlineButton("Tambah Anggota", "/x").WithIcon("plus")},
//	    Items: []web.ListItem{{
//	        Title: "Budi Santoso", Subtitle: "budi",
//	        Action: &web.ListAction{Label: "Keluarkan", FormAction: "/x/remove"},
//	    }},
//	}
//
// Di template: {{template "list-card" .Members}}

// ListAction = tombol kecil di ujung kanan baris. Dikirim sebagai form POST
// (beserta csrf_token milik kartu), jadi aman tanpa JavaScript.
type ListAction struct {
	Label      string
	FormAction string // URL tujuan POST
	Confirm    string // opsional; kalau diisi muncul konfirmasi browser sebelum submit
}

// ListItem = satu baris.
type ListItem struct {
	Title    string
	Subtitle string      // opsional, teks kecil di bawah Title
	Tag      TableCell   // opsional; kalau Text diisi tampil di kanan (label berwarna lewat Variant)
	Action   *ListAction // opsional
}

type ListCard struct {
	Title     string
	Note      string   // opsional; keterangan kecil di kanan judul, mis. "Diatur dari halaman Groups"
	Actions   []Button // tombol di kanan judul (opsional)
	Items     []ListItem
	Empty     string // pesan kalau Items kosong (default "Belum ada data.")
	Class     string
	CSRFToken string // dipakai form aksi baris
}
