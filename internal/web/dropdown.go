package web

// Tipe data untuk komponen dropdown (templates/components/dropdown.html).
//
//	opts := []web.DropdownOption{
//	    {Value: "public", Label: "public"},
//	    {Value: "analytics", Label: "analytics"},
//	}
//
// Di template:
//
//	{{template "dropdown" (dict "Label" "Schema" "Name" "schema" "Options" .Options "Value" .Selected)}}

// DropdownOption adalah satu pilihan di dropdown.
type DropdownOption struct {
	Value    string // nilai yang dikirim form
	Label    string // teks yang ditampilkan
	Disabled bool   // pilihan tidak bisa dipilih
}
