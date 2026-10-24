package admin

// Form koneksi database: tambah (/admin/connections/new) dan ubah
// (/admin/connections/{id}/edit). Password bersifat write-only — saat ubah,
// kolom kosong berarti mempertahankan password tersimpan.

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// connectionFormPage = data templates/admin/connection_form.html fe-components.
type connectionFormPage struct {
	web.Shell
	Title      string
	Crumbs     []web.Crumb
	FormAction string
	Error      string
	Fields     []map[string]any
	Actions    []web.Button
}

// connectionFields membangun daftar isian form (kontrak komponen `input`).
// row nil = form tambah; selain itu form ubah (password selalu kosong di
// layar, maknanya "pertahankan yang tersimpan").
func connectionFields(row *ConnectionRow) []map[string]any {
	var name, host, port, database, username string
	isNew := row == nil
	if row != nil {
		name, host, port = row.Name, row.Host, row.Port
		database, username = row.Database, row.Username
	}
	if port == "" {
		port = "5432"
	}
	passwordHelper := "Kosong = pakai password tersimpan."
	if isNew {
		passwordHelper = "Wajib diisi; disimpan terenkripsi."
	}
	return []map[string]any{
		{"Label": "Nama tampilan", "Name": "name", "Value": name,
			"Placeholder": "analitik-produksi", "Required": true},
		{"Label": "Host", "Name": "host", "Value": host,
			"Placeholder": "10.0.0.1", "Required": true},
		{"Label": "Port", "Name": "port", "Type": "number", "Value": port, "Required": true},
		{"Label": "Database", "Name": "database", "Value": database,
			"Placeholder": "appdb", "Required": true},
		{"Label": "Username", "Name": "username", "Value": username, "Required": true},
		{"Label": "Password", "Name": "password", "Type": "password", "Helper": passwordHelper},
	}
}

// connectionFormActions = tombol Simpan + Batal untuk halaman form.
func connectionFormActions(cancelHref string) []web.Button {
	return []web.Button{
		{Label: "Simpan", Type: "submit"},
		{Label: "Batal", Variant: "secondary", Href: cancelHref},
	}
}

// connectionInput membaca & memvalidasi isi form. requirePassword=false untuk
// ubah (password kosong = pertahankan). pesan kosong = valid.
func connectionInput(r *http.Request, requirePassword bool) (ConnectionInput, string) {
	in := ConnectionInput{
		Name:     strings.TrimSpace(r.PostFormValue("name")),
		Host:     strings.TrimSpace(r.PostFormValue("host")),
		Port:     strings.TrimSpace(r.PostFormValue("port")),
		Database: strings.TrimSpace(r.PostFormValue("database")),
		Username: strings.TrimSpace(r.PostFormValue("username")),
		Password: r.PostFormValue("password"),
	}
	if in.Name == "" {
		return in, "Nama tampilan wajib diisi."
	}
	if in.Host == "" {
		return in, "Host wajib diisi."
	}
	if in.Port == "" {
		in.Port = "5432"
	}
	if _, err := strconv.Atoi(in.Port); err != nil {
		return in, "Port harus berupa angka."
	}
	if in.Database == "" {
		return in, "Nama database wajib diisi."
	}
	if in.Username == "" {
		return in, "Username wajib diisi."
	}
	if requirePassword && in.Password == "" {
		return in, "Password wajib diisi."
	}
	return in, ""
}

// --- tambah -----------------------------------------------------------------

// newFormPage membangun halaman form "Koneksi Baru" (errMsg untuk galat validasi).
func (c *connectionHandlers) newFormPage(r *http.Request, errMsg string) connectionFormPage {
	return connectionFormPage{
		Shell: c.shell(r),
		Title: "Koneksi Baru",
		Crumbs: []web.Crumb{
			{Label: "Admin", Href: "/admin/users"},
			{Label: "Kelola Koneksi", Href: "/admin/connections"},
			{Label: "Baru"},
		},
		FormAction: "/admin/connections",
		Error:      errMsg,
		Fields:     connectionFields(nil),
		Actions:    connectionFormActions("/admin/connections"),
	}
}

func (c *connectionHandlers) createForm(w http.ResponseWriter, r *http.Request) {
	c.render(w, http.StatusOK, "admin/connection_form.html", c.newFormPage(r, ""))
}

func (c *connectionHandlers) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	in, errMsg := connectionInput(r, true)
	if errMsg != "" {
		c.render(w, http.StatusBadRequest, "admin/connection_form.html", c.newFormPage(r, errMsg))
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	row, err := c.store.Create(r.Context(), in, user.ID)
	if err != nil {
		if errors.Is(err, ErrConnectionNameTaken) {
			c.render(w, http.StatusConflict, "admin/connection_form.html",
				c.newFormPage(r, "Nama koneksi sudah dipakai."))
			return
		}
		log.Printf("admin connections: buat koneksi: %v", err)
		c.render(w, http.StatusInternalServerError, "admin/connection_form.html",
			c.newFormPage(r, "Gagal menyimpan koneksi."))
		return
	}
	http.Redirect(w, r, "/admin/connections/"+strconv.FormatInt(row.ID, 10), http.StatusSeeOther)
}

// --- ubah -------------------------------------------------------------------

// editFormPage membangun halaman form "Ubah Koneksi" (errMsg untuk galat validasi).
func (c *connectionHandlers) editFormPage(r *http.Request, row ConnectionRow, errMsg string) connectionFormPage {
	id := strconv.FormatInt(row.ID, 10)
	return connectionFormPage{
		Shell: c.shell(r),
		Title: "Ubah Koneksi: " + row.Name,
		Crumbs: []web.Crumb{
			{Label: "Admin", Href: "/admin/users"},
			{Label: "Kelola Koneksi", Href: "/admin/connections"},
			{Label: row.Name, Href: "/admin/connections/" + id},
			{Label: "Ubah"},
		},
		FormAction: "/admin/connections/" + id,
		Error:      errMsg,
		Fields:     connectionFields(&row),
		Actions:    connectionFormActions("/admin/connections/" + id),
	}
}

func (c *connectionHandlers) editForm(w http.ResponseWriter, r *http.Request) {
	row, ok := c.load(w, r)
	if !ok {
		return
	}
	c.render(w, http.StatusOK, "admin/connection_form.html", c.editFormPage(r, row, ""))
}

func (c *connectionHandlers) update(w http.ResponseWriter, r *http.Request) {
	row, ok := c.load(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	in, errMsg := connectionInput(r, false)
	if errMsg != "" {
		c.render(w, http.StatusBadRequest, "admin/connection_form.html", c.editFormPage(r, row, errMsg))
		return
	}
	// Form tidak punya field deskripsi; pertahankan nilai tersimpan.
	in.Description = row.Description
	if _, err := c.store.Update(r.Context(), row.ID, in); err != nil {
		if errors.Is(err, ErrConnectionNameTaken) {
			c.render(w, http.StatusConflict, "admin/connection_form.html",
				c.editFormPage(r, row, "Nama koneksi sudah dipakai."))
			return
		}
		c.serverError(w, "ubah koneksi", err)
		return
	}
	c.evict(row.ID)
	http.Redirect(w, r, "/admin/connections/"+strconv.FormatInt(row.ID, 10), http.StatusSeeOther)
}
