package dbm

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// migrationNotices adalah pesan sukses per kode di query string (?status=).
var migrationNotices = map[string]string{
	"draft_saved": "Draft migration tersimpan. Belum ada perubahan pada database.",
	"executed":    "Migration selesai dieksekusi. Lihat detail untuk hasil commit atau rollback.",
}

// migrationsPage adalah data halaman riwayat (dbm/migrations.html).
type migrationsPage struct {
	web.AppShell
	Connection web.DatabaseLink
	Migrations []Migration
	CanEdit    bool
	Filter     string
	Notice     string
}

// migrationList menampilkan riwayat migration (View Only boleh melihat).
func (h *Handlers) migrationList(w http.ResponseWriter, r *http.Request) {
	connID, ok := parseConnID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	conn, allowed := h.requireAccess(w, r, connID, access.ActionView)
	if !allowed {
		return
	}

	page := migrationsPage{
		AppShell:  h.pageShell(r, "Riwayat Migration", connID, "migrations"),
		Connection: conn,
		CanEdit:    conn.Level == "edit",
		Notice:     migrationNotices[r.URL.Query().Get("status")],
	}

	items, err := h.store.List(r.Context(), connID)
	if err != nil {
		h.serverError(w, "daftar migration", err)
		return
	}

	// Filter status lewat ?status=draft|applied|failed (nilai dari whitelist).
	filter := r.URL.Query().Get("status")
	if filter == StatusDraft || filter == StatusApplied || filter == StatusFailed {
		page.Filter = filter
		filtered := make([]Migration, 0, len(items))
		for _, m := range items {
			if m.Status == filter {
				filtered = append(filtered, m)
			}
		}
		items = filtered
	}
	page.Migrations = items

	h.render(w, http.StatusOK, "dbm/migrations.html", page)
}

// migrationEditorPage adalah data editor draft (dbm/migration_editor.html).
type migrationEditorPage struct {
	web.AppShell
	Connection web.DatabaseLink
	Migration  Migration
	IsNew      bool
	FormAction string
	Name       string
	SQL        string
	Error      string
}

// migrationNew menampilkan form draft baru.
func (h *Handlers) migrationNew(w http.ResponseWriter, r *http.Request) {
	connID, ok := parseConnID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	conn, allowed := h.requireAccess(w, r, connID, access.ActionEdit)
	if !allowed {
		return
	}
	h.render(w, http.StatusOK, "dbm/migration_editor.html", migrationEditorPage{
		AppShell:  h.pageShell(r, "Editor Migration", connID, "migrations"),
		Connection: conn,
		IsNew:      true,
		FormAction: "/databases/" + strconv.FormatInt(connID, 10) + "/migrations",
	})
}

// migrationCreate menyimpan draft baru lalu mengarahkan ke daftar migration.
func (h *Handlers) migrationCreate(w http.ResponseWriter, r *http.Request) {
	connID, ok := parseConnID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	conn, allowed := h.requireAccess(w, r, connID, access.ActionEdit)
	if !allowed {
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	sqlText := strings.TrimSpace(r.PostFormValue("sql"))
	if name == "" || sqlText == "" {
		h.renderEditor(w, r, conn, migrationEditorPage{
			AppShell:  h.pageShell(r, "Editor Migration", connID, "migrations"),
			Connection: conn,
			IsNew:      true,
			FormAction: "/databases/" + strconv.FormatInt(connID, 10) + "/migrations",
			Name:       name,
			SQL:        sqlText,
			Error:      "Nama dan SQL wajib diisi.",
		}, http.StatusBadRequest)
		return
	}

	me, _ := auth.UserFromContext(r.Context())
	m, err := h.store.CreateDraft(r.Context(), connID, InjectName(sqlText, name), me.ID)
	if err != nil {
		h.serverError(w, "simpan draft", err)
		return
	}
	http.Redirect(w, r,
		"/databases/"+strconv.FormatInt(connID, 10)+"/migrations/"+strconv.FormatInt(m.ID, 10)+"?status=draft_saved",
		http.StatusSeeOther)
}

// migrationDetail menampilkan satu migration (draft/applied/failed).
func (h *Handlers) migrationDetail(w http.ResponseWriter, r *http.Request) {
	conn, m, ok := h.loadMigration(w, r, access.ActionView)
	if !ok {
		return
	}
	h.render(w, http.StatusOK, "dbm/migration_detail.html", migrationDetailPage{
		AppShell:  h.pageShell(r, "Detail Migration", conn.ID, "migrations"),
		Connection: conn,
		Migration:  m,
		Notice:     migrationNotices[r.URL.Query().Get("status")],
	})
}

// migrationEdit menampilkan form ubah draft (hanya status draft).
func (h *Handlers) migrationEdit(w http.ResponseWriter, r *http.Request) {
	conn, m, ok := h.loadMigration(w, r, access.ActionEdit)
	if !ok {
		return
	}
	if m.Status != StatusDraft {
		http.Error(w, "migration sudah dieksekusi dan tidak bisa diubah", http.StatusConflict)
		return
	}
	h.render(w, http.StatusOK, "dbm/migration_editor.html", migrationEditorPage{
		AppShell:  h.pageShell(r, "Ubah Draft", conn.ID, "migrations"),
		Connection: conn,
		Migration:  m,
		FormAction: "/databases/" + strconv.FormatInt(conn.ID, 10) + "/migrations/" + strconv.FormatInt(m.ID, 10),
		Name:       m.Name(),
		SQL:        m.SQL,
	})
}

// migrationUpdate menyimpan perubahan SQL draft.
func (h *Handlers) migrationUpdate(w http.ResponseWriter, r *http.Request) {
	conn, m, ok := h.loadMigration(w, r, access.ActionEdit)
	if !ok {
		return
	}
	if m.Status != StatusDraft {
		http.Error(w, "migration sudah dieksekusi dan tidak bisa diubah", http.StatusConflict)
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	sqlText := strings.TrimSpace(r.PostFormValue("sql"))
	if name == "" || sqlText == "" {
		h.renderEditor(w, r, conn, migrationEditorPage{
			AppShell:  h.pageShell(r, "Ubah Draft", conn.ID, "migrations"),
			Connection: conn,
			Migration:  m,
			FormAction: "/databases/" + strconv.FormatInt(conn.ID, 10) + "/migrations/" + strconv.FormatInt(m.ID, 10),
			Name:       name,
			SQL:        sqlText,
			Error:      "Nama dan SQL wajib diisi.",
		}, http.StatusBadRequest)
		return
	}

	if err := h.store.UpdateDraft(r.Context(), m.ID, InjectName(sqlText, name)); err != nil {
		if errors.Is(err, ErrMigrationNotDraft) {
			http.Error(w, "migration sudah dieksekusi dan tidak bisa diubah", http.StatusConflict)
			return
		}
		h.serverError(w, "ubah draft", err)
		return
	}
	http.Redirect(w, r,
		"/databases/"+strconv.FormatInt(conn.ID, 10)+"/migrations/"+strconv.FormatInt(m.ID, 10)+"?status=draft_saved",
		http.StatusSeeOther)
}

// migrationPreviewPage adalah data pratinjau (dbm/migration_preview.html).
type migrationPreviewPage struct {
	web.AppShell
	Connection web.DatabaseLink
	Migration  Migration
	FormAction string
	Error      string
}

// migrationPreview menampilkan ringkasan draft sebelum eksekusi (hanya draft).
func (h *Handlers) migrationPreview(w http.ResponseWriter, r *http.Request) {
	conn, m, ok := h.loadMigration(w, r, access.ActionEdit)
	if !ok {
		return
	}
	if m.Status != StatusDraft {
		http.Redirect(w, r,
			"/databases/"+strconv.FormatInt(conn.ID, 10)+"/migrations/"+strconv.FormatInt(m.ID, 10),
			http.StatusSeeOther)
		return
	}
	h.render(w, http.StatusOK, "dbm/migration_preview.html", migrationPreviewPage{
		AppShell:  h.pageShell(r, "Pratinjau Migration", conn.ID, "migrations"),
		Connection: conn,
		Migration:  m,
		FormAction: "/databases/" + strconv.FormatInt(conn.ID, 10) + "/migrations/" + strconv.FormatInt(m.ID, 10) + "/execute",
	})
}

// migrationExecute menjalankan SQL draft dalam satu transaksi pada database
// target, lalu mencatat hasilnya (applied/failed) di database platform.
func (h *Handlers) migrationExecute(w http.ResponseWriter, r *http.Request) {
	conn, m, ok := h.loadMigration(w, r, access.ActionEdit)
	if !ok {
		return
	}
	detailURL := "/databases/" + strconv.FormatInt(conn.ID, 10) + "/migrations/" + strconv.FormatInt(m.ID, 10)

	// Konfirmasi ketik-nama + checkbox, sama dengan pola hapus koneksi.
	confirmName := strings.TrimSpace(r.PostFormValue("confirm_name"))
	confirmed := r.PostFormValue("confirm_execute") == "on"
	if confirmName != conn.Name || !confirmed {
		h.render(w, http.StatusBadRequest, "dbm/migration_preview.html", migrationPreviewPage{
			AppShell:  h.pageShell(r, "Pratinjau Migration", conn.ID, "migrations"),
			Connection: conn,
			Migration:  m,
			FormAction: detailURL + "/execute",
			Error:      "Konfirmasi tidak cocok. Ketik nama database persis dan centang persetujuan.",
		})
		return
	}

	me, _ := auth.UserFromContext(r.Context())
	status, err := h.runner.Execute(r.Context(), conn.ID, m.ID, me.ID)
	if err != nil {
		switch {
		case errors.Is(err, ErrMigrationNotFound):
			http.NotFound(w, r)
		case errors.Is(err, ErrMigrationNotDraft):
			http.Redirect(w, r, detailURL, http.StatusSeeOther)
		case errors.Is(err, ErrChecksumMismatch):
			http.Error(w, "SQL berubah sejak terakhir ditinjau; buka kembali detail migration", http.StatusConflict)
		default:
			// Koneksi target gagal total (registry) — halaman koneksi gagal.
			h.renderDBError(w, r, conn, err)
		}
		return
	}

	if status == StatusFailed {
		http.Redirect(w, r, detailURL+"?status=executed", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, detailURL+"?status=executed", http.StatusSeeOther)
}

// migrationDetailPage adalah data halaman detail (dbm/migration_detail.html).
type migrationDetailPage struct {
	web.AppShell
	Connection web.DatabaseLink
	Migration  Migration
	Notice     string
}

// loadMigration memeriksa izin, memuat migration, dan memastikan migration
// memang milik koneksi di URL. Merender respons sendiri bila gagal.
func (h *Handlers) loadMigration(w http.ResponseWriter, r *http.Request, action access.Action) (web.DatabaseLink, Migration, bool) {
	connID, ok := parseConnID(r)
	if !ok {
		http.NotFound(w, r)
		return web.DatabaseLink{}, Migration{}, false
	}
	mid, err := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	if err != nil || mid <= 0 {
		http.NotFound(w, r)
		return web.DatabaseLink{}, Migration{}, false
	}

	conn, allowed := h.requireAccess(w, r, connID, action)
	if !allowed {
		return web.DatabaseLink{}, Migration{}, false
	}

	m, err := h.store.Get(r.Context(), mid)
	if errors.Is(err, ErrMigrationNotFound) || m.ConnectionID != connID {
		http.NotFound(w, r)
		return web.DatabaseLink{}, Migration{}, false
	}
	if err != nil {
		h.serverError(w, "ambil migration", err)
		return web.DatabaseLink{}, Migration{}, false
	}
	return conn, m, true
}

// renderEditor merender ulang halaman editor (untuk validasi gagal).
func (h *Handlers) renderEditor(w http.ResponseWriter, r *http.Request, conn web.DatabaseLink, page migrationEditorPage, status int) {
	page.AppShell = h.pageShell(r, "Editor Migration", conn.ID, "migrations")
	h.render(w, status, "dbm/migration_editor.html", page)
}
