package web

// Tipe data untuk komponen tabel (templates/components/table.html).
// Backend cukup mengisi struct ini, template yang menggambar.
//
//	tbl := web.Table{
//	    Title: "Daftar Tabel",
//	    Columns: []web.TableColumn{
//	        {Key: "name", Label: "Nama", Bold: true},
//	        {Key: "status", Label: "Koneksi"},
//	    },
//	    Rows: []web.TableRow{
//	        {"name": {Text: "hr_db"}, "status": {Text: "Terhubung", Variant: "success"}},
//	    },
//	}
//
// Di template: {{template "table" .Table}}

// TableColumn mendefinisikan satu kolom.
type TableColumn struct {
	Key   string // kunci untuk mengambil nilai dari TableRow
	Label string // teks header
	Align string // "" (kiri, default) | "center" | "right"
	Bold  bool   // isi kolom ditebalkan (mis. kolom nama)

	// Sort opsional. Header jadi link pengurutan kalau SortHref diisi.
	//   Sort      arah urutan saat ini: "asc" | "desc" | "" (tidak diurutkan)
	//   SortHref  tujuan klik header (handler yang menentukan arah berikutnya)
	Sort     string
	SortHref string
}

// TableCell adalah isi satu sel.
//
//	Text     teks yang ditampilkan
//	Variant  kalau diisi, sel dirender sebagai label berwarna:
//	         "success" | "error" | "warning" | "info" | "neutral"
//	Href     kalau diisi (dan Variant kosong), teks jadi link
type TableCell struct {
	Text    string
	Variant string
	Href    string
	Button  *Button
}

// TableRow memetakan Column.Key -> sel. Key yang tidak ada = sel kosong.
type TableRow map[string]TableCell

// Table adalah seluruh data untuk satu tabel.
type Table struct {
	Title   string    // opsional; kalau kosong tidak ada judul
	Tag     TableCell // opsional; label kecil di kanan judul, mis. {Text: "View-only"}
	Columns []TableColumn
	Rows    []TableRow
	Empty   string // pesan kalau Rows kosong (default "Belum ada data.")
	Class   string // class tambahan untuk pembungkus
	Pager   *Pager // opsional; paginasi di bawah tabel (lihat pager.go)
}
