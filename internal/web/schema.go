package web

type Crumb struct {
	Label string
	Href  string
}

type SchemaPage struct {
	Shell            // Nav "database" -> sidebar database + menu Database aktif
	Title     string // mis. "Schema public"
	Crumbs    []Crumb
	Tables    Table // kartu "Daftar Tabel"
	Relations Table // kartu "Relasi (Foreign Key)"
}
