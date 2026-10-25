package web

import (
	"html/template"
	"strconv"
)

// ---------------------------------------------------------------------------
// Edit Koneksi (templates/admin/connection_form.html)
//
//	page := web.ConnectionFormPage{
//	    Shell:      web.Shell{Nav: "admin", SideActive: "connections", User: u},
//	    PageHeader: web.ConnectionEditHeader(c.Name, "/admin/connections"),
//	    FormAction: "/admin/connections/" + id,                // POST simpan
//	    CancelHref: "/admin/connections/" + id,                // tombol Batal
//	    Values:     web.ConnectionFormValues{DisplayName: ..., Host: ..., Port: strconv.Itoa(c.Port)},
//	    Test:       web.ConnectionTestPanel{TestAction: "/admin/connections/" + id + "/test"},
//	}
//	page.SetCSRF(token)
//
// Field yang dikirim form simpan (POST FormAction):
//
//	csrf_token, display_name, database, host, port, username, password
//
// Password kosong = tidak diubah (password lama tidak pernah dikirim ke halaman).
//
// Tombol "Test Koneksi" (htmx) mem-POST isi form ke Test.TestAction. Handler-nya menjawab
// dengan potongan HTML dari Renderer.RenderConnectionTest(w, panel); potongan itu
// menggantikan tombol + hasil tes di halaman.
// ---------------------------------------------------------------------------

// ID elemen yang dirujuk template / JS (static/js/connection-form.js).
const connectionTestID = "connection-test"

// ConnectionFormValues = isi awal form (teks apa adanya, termasuk Port).
type ConnectionFormValues struct {
	DisplayName string
	Database    string
	Host        string
	Port        string
	Username    string
}

// ConnectionTestResult = hasil satu kali tes koneksi.
type ConnectionTestResult struct {
	OK     bool
	Title  string // kalimat tebal, mis. "Koneksi berhasil diuji."
	Detail string // lanjutan, mis. "Server membalas dalam 38 ms."
	// InvalidField = name input yang disorot merah saat tes gagal (mis. "host"); boleh kosong.
	InvalidField string
}

// ConnectionTestPanel = tombol tes + hasil tesnya (komponen connection-test).
// Result nil = belum pernah dites.
type ConnectionTestPanel struct {
	TestAction string // URL POST tes koneksi
	Result     *ConnectionTestResult
}

// Button = tombol tes: "Test Koneksi", atau "Test Ulang" kalau tes terakhir gagal.
func (p ConnectionTestPanel) Button() Button {
	label := "Test Koneksi"
	if p.Result != nil && !p.Result.OK {
		label = "Test Ulang"
	}
	b := OutlineButton(label, "")
	b.Attrs = template.HTMLAttr(`hx-post="` + template.HTMLEscapeString(p.TestAction) + `" hx-target="#` +
		connectionTestID + `" hx-swap="outerHTML" hx-disabled-elt="this"`)
	return b
}

type ConnectionFormPage struct {
	Shell      // Nav "admin", SideActive "connections"
	PageHeader // nama koneksi + label "Mode edit"
	CSRFToken  string
	FormAction string // URL POST simpan
	CancelHref string // tujuan tombol "Batal"
	Values     ConnectionFormValues
	// Errors = galat validasi dari server, kunci = name field ("host", "port", ...).
	Errors map[string]string
	Test   ConnectionTestPanel
}

// ConnectionEditHeader = judul (nama koneksi), breadcrumb "Admin > Kelola Koneksi > <nama>",
// dan label "Mode edit" di kanan.
func ConnectionEditHeader(name, listHref string) PageHeader {
	return PageHeader{
		Title: name,
		Crumbs: []Crumb{
			{Label: "Admin", Href: "/admin/users"},
			{Label: "Kelola Koneksi", Href: listHref},
			{Label: name},
		},
		Tag: &TableCell{Text: "Mode edit", Variant: "warning"},
	}
}

// Fields = isian form berurutan (2 kolom: kiri-kanan, atas-bawah).
func (p ConnectionFormPage) Fields() []InputField {
	f := func(in InputField) InputField {
		in.Error = p.Errors[in.Name]
		if r := p.Test.Result; r != nil && !r.OK && r.InvalidField == in.Name && in.Error == "" {
			in.Class, in.Attrs = "is-error", in.Attrs+` aria-invalid="true"`
		}
		return in
	}
	v := p.Values
	return []InputField{
		f(InputField{Label: "Nama tampilan", Name: "display_name", Value: v.DisplayName, Required: true}),
		f(InputField{Label: "Nama database", Name: "database", Value: v.Database, Required: true}),
		f(InputField{Label: "Host", Name: "host", Value: v.Host, Required: true, Attrs: `autocomplete="off"`}),
		f(InputField{Label: "Port", Name: "port", Type: "number", Value: v.Port, Required: true,
			Attrs: `inputmode="numeric" min="1" max="65535"`}),
		f(InputField{Label: "Username", Name: "username", Value: v.Username, Required: true, Attrs: `autocomplete="off"`}),
		f(InputField{Label: "Password", Name: "password", Type: "password",
			Placeholder: "Kosongkan jika tidak diubah", Attrs: `autocomplete="new-password"`}),
	}
}

// SetCSRF mengisi token csrf ke form.
func (p *ConnectionFormPage) SetCSRF(token string) { p.CSRFToken = token }

// FormValuesFrom membuat nilai awal form dari data koneksi.
func FormValuesFrom(displayName, database, host string, port int, username string) ConnectionFormValues {
	return ConnectionFormValues{DisplayName: displayName, Database: database, Host: host,
		Port: strconv.Itoa(port), Username: username}
}
