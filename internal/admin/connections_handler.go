package admin

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// PoolEvictor menutup pool target saat koneksi diubah/dihapus (dipakai dbm).
type PoolEvictor interface {
	Evict(connID int64)
}

type connectionView interface {
	auth.Renderer
	RenderConnectionTest(w http.ResponseWriter, p web.ConnectionTestPanel)
}

type connectionHandlers struct {
	store   ConnectionStore
	view    connectionView
	evictor PoolEvictor
	revoker session.Revoker
}

// WithConnectionAdmin mengaktifkan halaman kelola koneksi (/admin/connections).
func WithConnectionAdmin(store ConnectionStore, view connectionView, evictor PoolEvictor, revoker session.Revoker) Option {
	return func(h *Handlers) {
		h.connections = &connectionHandlers{store: store, view: view, evictor: evictor, revoker: revoker}
	}
}

func (h *Handlers) connectionsRoutes(r chi.Router) {
	if h.connections == nil {
		return
	}
	c := h.connections
	r.Get("/connections", c.list)
	// Form "Tambah Koneksi"/"Edit Koneksi": tampilan (GET) + simpan (POST).
	r.Get("/connections/new", c.createForm)
	r.Post("/connections", c.create)
	r.Post("/connections/test", c.testForm)
	r.Get("/connections/{id}", c.detail)
	r.Post("/connections/{id}", c.update)
	r.Get("/connections/{id}/edit", c.editForm)
	r.Post("/connections/{id}/test", c.test)
	r.Post("/connections/{id}/delete", c.remove)
}

// shell membangun data shell FE (layout.html: header + sidebar admin).
func (c *connectionHandlers) shell(r *http.Request) web.Shell {
	me, _ := auth.UserFromContext(r.Context())
	return web.Shell{
		Nav:        "admin",
		SideActive: "connections",
		User: &web.ShellUser{
			Username:        me.Username,
			IsPlatformAdmin: me.IsPlatformAdmin,
			CSRFToken:       session.CSRFToken(r.Context()),
		},
	}
}

func (c *connectionHandlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	c.view.Render(w, name, data)
}

func (c *connectionHandlers) serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("admin connections: %s: %v", what, err) // jangan log kredensial
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

func (c *connectionHandlers) evict(connID int64) {
	if c.evictor != nil {
		c.evictor.Evict(connID)
	}
}

// list menampilkan tabel kelola koneksi format fe-components.
func (c *connectionHandlers) list(w http.ResponseWriter, r *http.Request) {
	rows, err := c.store.List(r.Context())
	if err != nil {
		c.serverError(w, "daftar koneksi", err)
		return
	}
	groups, err := c.store.ConnectionGroups(r.Context())
	if err != nil {
		c.serverError(w, "grup per koneksi", err)
		return
	}
	summaries := make([]web.ConnectionSummary, 0, len(rows))
	for _, row := range rows {
		port, _ := strconv.Atoi(row.Port)
		if port == 0 {
			port = 5432
		}
		names := make([]string, 0, 2)
		for _, g := range groups[row.ID] {
			names = append(names, g.Name)
		}
		summaries = append(summaries, web.ConnectionSummary{
			Name:   row.Name,
			Host:   row.Host,
			Port:   port,
			Status: web.ConnConnected,
			Groups: names,
			Href:   "/admin/connections/" + strconv.FormatInt(row.ID, 10),
		})
	}
	c.render(w, http.StatusOK, "admin/connections.html", web.ConnectionsPage{
		Shell:       c.shell(r),
		PageHeader:  web.ConnectionsHeader("/admin/connections/new"),
		Connections: web.NewConnectionsTable(summaries),
	})
}

// detail menampilkan halaman detail koneksi (kartu parameter + kartu group).
func (c *connectionHandlers) detail(w http.ResponseWriter, r *http.Request) {
	row, ok := c.load(w, r)
	if !ok {
		return
	}
	all, err := c.store.ConnectionGroups(r.Context())
	if err != nil {
		c.serverError(w, "grup per koneksi", err)
		return
	}
	port, _ := strconv.Atoi(row.Port)
	if port == 0 {
		port = 5432
	}
	id := strconv.FormatInt(row.ID, 10)
	groupNames := make([]string, 0, len(all[row.ID]))
	for _, g := range all[row.ID] {
		groupNames = append(groupNames, g.Name)
	}
	page := web.ConnectionDetailPage{
		Shell:        c.shell(r),
		PageHeader:   web.ConnectionDetailHeader(row.Name, "/admin/connections", "/admin/connections/"+id+"/edit"),
		TestAction:   "/admin/connections/" + id + "/test",
		DeleteAction: "/admin/connections/" + id + "/delete",
		DeleteTitle:  "Hapus koneksi " + row.Database,
		DeleteHost:   fmt.Sprintf("%s:%d", row.Host, port),
		DeleteGroups: groupNames,
		Params: web.NewConnectionParamsCard(web.ConnectionParams{
			DisplayName: row.Name,
			Database:    row.Database,
			Host:        row.Host,
			Port:        port,
			Username:    row.Username,
		}),
		Groups: web.NewConnectionGroupsCard(all[row.ID]),
	}
	page.SetCSRF(session.CSRFToken(r.Context()))
	c.render(w, http.StatusOK, "admin/connection_detail.html", page)
}

// test menjalankan uji koneksi (SELECT 1) ke database target.
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
	// Hasil uji kembali ke halaman detail (FE belum punya slot flash terpisah).
	http.Redirect(w, r, "/admin/connections/"+strconv.FormatInt(row.ID, 10)+"?status="+code, http.StatusSeeOther)
}

// testForm menguji isi form koneksi tanpa menyimpannya. Jawaban berupa
// potongan HTML tombol tes + hasil untuk diganti htmx di halaman form.
func (c *connectionHandlers) testForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	id, _ := strconv.ParseInt(r.PostFormValue("connection_id"), 10, 64)
	in, errMsg := connectionInput(r, id == 0)
	panel := web.ConnectionTestPanel{Action: "/admin/connections/test"}
	if errMsg != "" {
		panel.Result = &web.ConnectionTestResult{
			Title:  "Koneksi belum bisa diuji.",
			Detail: errMsg,
		}
		c.view.RenderConnectionTest(w, panel)
		return
	}

	start := time.Now()
	if err := c.store.TestInput(r.Context(), id, in); err != nil {
		log.Printf("admin connections: uji koneksi dari form: %v", err)
		result := &web.ConnectionTestResult{
			Title: "Koneksi gagal diuji.",
			Detail: "Tidak dapat terhubung ke " + net.JoinHostPort(in.Host, in.Port) +
				". Periksa host, port, dan kredensial.",
		}
		if errors.Is(err, ErrConnectionNotFound) {
			result.Title = "Koneksi tidak ditemukan."
			result.Detail = "Muat ulang halaman lalu coba lagi."
		}
		panel.Result = result
	} else {
		panel.Result = &web.ConnectionTestResult{
			OK:     true,
			Title:  "Koneksi berhasil diuji.",
			Detail: fmt.Sprintf("Server membalas dalam %d ms.", time.Since(start).Milliseconds()),
		}
	}
	c.view.RenderConnectionTest(w, panel)
}

// remove menghapus koneksi. Konfirmasi memakai dialog browser (onsubmit di
// template fe-components), sehingga server menerima POST langsung.
func (c *connectionHandlers) remove(w http.ResponseWriter, r *http.Request) {
	row, ok := c.load(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	affected, err := c.store.UsersWithAccess(r.Context(), row.ID)
	if err != nil {
		c.serverError(w, "ambil user dengan akses", err)
		return
	}
	if err := c.store.Delete(r.Context(), row.ID); err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			http.NotFound(w, r)
			return
		}
		c.serverError(w, "hapus koneksi", err)
		return
	}
	c.evict(row.ID)
	if err := c.revokeUsers(r.Context(), affected); err != nil {
		c.serverError(w, "cabut session setelah hapus koneksi", err)
		return
	}
	http.Redirect(w, r, "/admin/connections", http.StatusSeeOther)
}

// revokeUsers cabut session affected users, kecuali current admin
func (c *connectionHandlers) revokeUsers(ctx context.Context, ids []int64) error {
	me, _ := auth.UserFromContext(ctx)
	for _, id := range ids {
		if me.ID != 0 && id == me.ID {
			continue
		}
		if err := c.revoker.RevokeSessionsByUser(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
