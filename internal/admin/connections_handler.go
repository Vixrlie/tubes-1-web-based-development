package admin

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

type AppRenderer interface {
	RenderApp(w http.ResponseWriter, name string, data any)
}

type PoolEvictor interface {
	Evict(connID int64)
}

type connectionHandlers struct {
	store   ConnectionStore
	view    AppRenderer
	evictor PoolEvictor
	shell   web.ShellStore
}

func WithConnectionAdmin(store ConnectionStore, view AppRenderer, evictor PoolEvictor, shell web.ShellStore) Option {
	return func(h *Handlers) {
		h.connections = &connectionHandlers{store: store, view: view, evictor: evictor, shell: shell}
	}
}

func (h *Handlers) connectionsRoutes(r chi.Router) {
	if h.connections == nil {
		return
	}
	c := h.connections
	r.Get("/connections", c.list)
	r.Get("/connections/new", c.newForm)
	r.Post("/connections", c.create)
	r.Get("/connections/{id}", c.editForm)
	r.Post("/connections/{id}", c.update)
	r.Post("/connections/{id}/test", c.test)
	r.Post("/connections/{id}/delete", c.remove)
}

// Pesan hasil aksi dipilih lewat kode di query string; teks bebas dari URL
// tidak pernah dirender.
type connectionNotice struct {
	Title string
	Desc  string
}

var connectionNotices = map[string]connectionNotice{
	"created":     {Title: "Koneksi database berhasil dibuat.", Desc: "Koneksi dapat segera digunakan untuk memberikan akses group."},
	"updated":     {Title: "Data koneksi diperbarui.", Desc: "Perubahan tersimpan dan tidak memengaruhi riwayat migrasi."},
	"deleted":     {Title: "Koneksi database dihapus.", Desc: "Grants dan riwayat migration terkait ikut dihapus dari platform."},
	"test_ok":     {Title: "Uji koneksi berhasil", Desc: "Koneksi dapat dihubungi. Tidak ada perubahan data yang dilakukan."},
	"test_failed": {Title: "Koneksi gagal", Desc: "Periksa host, port, dan password koneksi."},
}

type connectionsPage struct {
	web.AppShell
	Connections []ConnectionRow
	Selected    *ConnectionRow
	DeleteError string
}

type connectionFormPage struct {
	web.AppShell
	IsNew      bool
	FormAction string
	Values     ConnectionInput
	Errors     map[string]string
}

func (c *connectionHandlers) shellFor(ctx context.Context, title, activeNav string) web.AppShell {
	return web.BuildShell(ctx, c.shell, web.ShellOptions{
		Title:                title,
		ActiveNav:            activeNav,
		CanManageUsers:       true,
		CanManageConnections: true,
	})
}

func (c *connectionHandlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	c.view.RenderApp(w, name, data)
}

func (c *connectionHandlers) serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("admin connections: %s: %v", what, err)
	http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
}

func (c *connectionHandlers) load(w http.ResponseWriter, r *http.Request) (ConnectionRow, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return ConnectionRow{}, false
	}
	row, err := c.store.Get(r.Context(), id)
	if errors.Is(err, ErrConnectionNotFound) {
		http.NotFound(w, r)
		return ConnectionRow{}, false
	}
	if err != nil {
		c.serverError(w, "ambil koneksi", err)
		return ConnectionRow{}, false
	}
	return row, true
}

func readConnectionForm(r *http.Request) ConnectionInput {
	return ConnectionInput{
		Name:        strings.TrimSpace(r.PostFormValue("name")),
		Description: strings.TrimSpace(r.PostFormValue("description")),
		Host:        strings.TrimSpace(r.PostFormValue("host")),
		Port:        strings.TrimSpace(r.PostFormValue("port")),
		Database:    strings.TrimSpace(r.PostFormValue("database")),
		Username:    strings.TrimSpace(r.PostFormValue("username")),
		Password:    r.PostFormValue("password"),
	}
}

func (c *connectionHandlers) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := c.store.List(ctx)
	if err != nil {
		c.serverError(w, "daftar koneksi", err)
		return
	}

	shell := c.shellFor(ctx, "Koneksi database", "connections")
	if n, ok := connectionNotices[r.URL.Query().Get("status")]; ok {
		shell.Notice = n.Title
		shell.NoticeDesc = n.Desc
	}
	shell.Breadcrumbs = []web.Crumb{
		{Label: "Admin", Href: "/admin/users"},
		{Label: "Koneksi"},
		{Label: "Daftar"},
	}

	page := connectionsPage{AppShell: shell, Connections: rows}
	if rawID := r.URL.Query().Get("hapus"); rawID != "" {
		if row, ok := findConnection(rows, rawID); ok {
			page.Selected = &row
		}
	}
	c.render(w, http.StatusOK, "admin/connections.html", page)
}

func findConnection(rows []ConnectionRow, rawID string) (ConnectionRow, bool) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return ConnectionRow{}, false
	}
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return ConnectionRow{}, false
}

func (c *connectionHandlers) newForm(w http.ResponseWriter, r *http.Request) {
	shell := c.shellFor(r.Context(), "Tambah koneksi", "connections")
	shell.Breadcrumbs = []web.Crumb{
		{Label: "Admin", Href: "/admin/users"},
		{Label: "Koneksi", Href: "/admin/connections"},
		{Label: "Tambah"},
	}
	c.render(w, http.StatusOK, "admin/connection_form.html", connectionFormPage{
		AppShell:   shell,
		IsNew:      true,
		FormAction: "/admin/connections",
		Values:     ConnectionInput{Port: "5432"},
	})
}

func (c *connectionHandlers) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	in := readConnectionForm(r)
	if fieldErrors := ValidateConnection(in, true); len(fieldErrors) > 0 {
		c.renderForm(w, r, http.StatusBadRequest, true, "/admin/connections", in, fieldErrors)
		return
	}

	me, _ := auth.UserFromContext(r.Context())
	if _, err := c.store.Create(r.Context(), in, me.ID); err != nil {
		if errors.Is(err, ErrConnectionNameTaken) {
			fieldErrors := map[string]string{"name": ErrConnectionNameTaken.Error()}
			c.renderForm(w, r, http.StatusConflict, true, "/admin/connections", in, fieldErrors)
			return
		}
		c.serverError(w, "buat koneksi", err)
		return
	}
	http.Redirect(w, r, "/admin/connections?status=created", http.StatusSeeOther)
}

func (c *connectionHandlers) editForm(w http.ResponseWriter, r *http.Request) {
	row, ok := c.load(w, r)
	if !ok {
		return
	}
	shell := c.shellFor(r.Context(), "Ubah koneksi", "connections")
	shell.Breadcrumbs = []web.Crumb{
		{Label: "Admin", Href: "/admin/users"},
		{Label: "Koneksi", Href: "/admin/connections"},
		{Label: row.Name},
	}
	c.render(w, http.StatusOK, "admin/connection_form.html", connectionFormPage{
		AppShell:   shell,
		FormAction: "/admin/connections/" + strconv.FormatInt(row.ID, 10),
		Values: ConnectionInput{
			Name:        row.Name,
			Description: row.Description,
			Host:        row.Host,
			Port:        row.Port,
			Database:    row.Database,
			Username:    row.Username,
		},
	})
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
	in := readConnectionForm(r)
	if fieldErrors := ValidateConnection(in, false); len(fieldErrors) > 0 {
		c.renderForm(w, r, http.StatusBadRequest, false, "/admin/connections/"+strconv.FormatInt(row.ID, 10), in, fieldErrors)
		return
	}

	if _, err := c.store.Update(r.Context(), row.ID, in); err != nil {
		if errors.Is(err, ErrConnectionNameTaken) {
			fieldErrors := map[string]string{"name": ErrConnectionNameTaken.Error()}
			c.renderForm(w, r, http.StatusConflict, false, "/admin/connections/"+strconv.FormatInt(row.ID, 10), in, fieldErrors)
			return
		}
		c.serverError(w, "ubah koneksi", err)
		return
	}
	c.evict(row.ID)
	http.Redirect(w, r, "/admin/connections?status=updated", http.StatusSeeOther)
}

func (c *connectionHandlers) test(w http.ResponseWriter, r *http.Request) {
	row, ok := c.load(w, r)
	if !ok {
		return
	}
	code := "test_ok"
	if err := c.store.Test(r.Context(), row.ID); err != nil {
		log.Printf("admin connections: uji koneksi %d gagal: %v", row.ID, err)
		code = "test_failed"
	}
	http.Redirect(w, r, "/admin/connections?status="+code, http.StatusSeeOther)
}

func (c *connectionHandlers) remove(w http.ResponseWriter, r *http.Request) {
	row, ok := c.load(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}

	confirmName := strings.TrimSpace(r.PostFormValue("confirm_name"))
	confirmed := r.PostFormValue("confirm_delete") == "on"
	if confirmName != row.Name || !confirmed {
		c.renderDeleteError(w, r, row, "Konfirmasi tidak cocok. Ketik nama koneksi persis seperti pada daftar dan centang persetujuan.")
		return
	}

	if err := c.store.Delete(r.Context(), row.ID); err != nil {
		c.serverError(w, "hapus koneksi", err)
		return
	}
	c.evict(row.ID)
	http.Redirect(w, r, "/admin/connections?status=deleted", http.StatusSeeOther)
}

func (c *connectionHandlers) renderDeleteError(w http.ResponseWriter, r *http.Request, row ConnectionRow, message string) {
	rows, err := c.store.List(r.Context())
	if err != nil {
		c.serverError(w, "daftar koneksi", err)
		return
	}
	shell := c.shellFor(r.Context(), "Koneksi database", "connections")
	shell.Breadcrumbs = []web.Crumb{
		{Label: "Admin", Href: "/admin/users"},
		{Label: "Koneksi"},
		{Label: "Daftar"},
	}
	selected := row
	c.render(w, http.StatusBadRequest, "admin/connections.html", connectionsPage{
		AppShell:    shell,
		Connections: rows,
		Selected:    &selected,
		DeleteError: message,
	})
}

func (c *connectionHandlers) renderForm(w http.ResponseWriter, r *http.Request, status int, isNew bool, action string, in ConnectionInput, fieldErrors map[string]string) {
	shell := c.shellFor(r.Context(), "Koneksi database", "connections")
	shell.Breadcrumbs = []web.Crumb{
		{Label: "Admin", Href: "/admin/users"},
		{Label: "Koneksi", Href: "/admin/connections"},
		{Label: "Tambah"},
	}
	if !isNew {
		shell.Breadcrumbs[2].Label = "Ubah"
	}
	c.render(w, status, "admin/connection_form.html", connectionFormPage{
		AppShell:   shell,
		IsNew:      isNew,
		FormAction: action,
		Values:     in,
		Errors:     fieldErrors,
	})
}

func (c *connectionHandlers) evict(connID int64) {
	if c.evictor != nil {
		c.evictor.Evict(connID)
	}
}
