package web

// Tipe data untuk komponen detail-card (templates/components/detail-card.html):
// kartu berisi judul dan kisi 2 kolom pasangan "label + nilai" (hanya tampil, bukan
// form). Dipakai mis. untuk "Parameter Koneksi" di detail koneksi, tapi netral.
//
//	card := web.DetailCard{
//	    Title: "Parameter Koneksi",
//	    Fields: []web.DetailField{
//	        {Label: "Host", Value: "db-shop.internal"},
//	        {Label: "Password", Value: "••••••••••", Note: "tidak ditampilkan ulang"},
//	    },
//	}
//
// Di template: {{template "detail-card" .Params}}

// DetailField = satu pasangan label + nilai.
type DetailField struct {
	Label string
	Value string
	Note  string // opsional; keterangan kecil di sebelah nilai
}

type DetailCard struct {
	Title   string
	Actions []Button // tombol di kanan judul (opsional)
	Fields  []DetailField
	Class   string
}
