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

var migrationNotices = map[string]string{
	"saved":   "Draft migration disimpan. Database target belum diubah.",
	"updated": "Perubahan draft migration disimpan. Database target belum diubah.",
	"applied": "Migration berhasil dijalankan. Perubahan sudah dikomit di database target.",
	"failed":  "Migration gagal. Database target membatalkan seluruh perubahan.",
}

// needEdittrue menuntut akses View + Edit; selain itu cukup View Only.
type migrationAccess struct {
	ConnID  int64
	Link    web.DatabaseLink
	Shell   web.AppShell
	CanEdit bool
}

type migrationListPage struct {
	web.AppShell
	Connection web.DatabaseLink
	Migrations []Migration
	Filter     string
	CanEdit    bool
}

type migrationDetailPage struct {
	web.AppShell
	Connection web.DatabaseLink
	Migration  Migration
	CanEdit    bool
}

type migrationEditorPage struct {
	web.AppShell
	Connection web.DatabaseLink
	Migration  Migration
	IsNew      bool
	FormAction string
	SQL        string
	Name       string
	Error      string
}

type migrationPreviewPage struct {
	web.AppShell
	Connection web.DatabaseLink
	Migration  Migration
	FormAction string
	Error      string
}

func (h *Handlers) migrationRoutes(r chi.Router) {
	if h.cfg.Migrations == nil {
		return
	}
	r.Get("/databases/{id}/migrations", h.migrationList)
	r.Get("/databases/{id}/migrations/new", h.migrationNew)
	r.Post("/databases/{id}/migrations", h.migrationCreate)
	r.Get("/databases/{id}/migrations/{mid}", h.migrationDetail)
	r.Get("/databases/{id}/migrations/{mid}/edit", h.migrationEdit)
	r.Post("/databases/{id}/migrations/{mid}", h.migrationUpdate)
	r.Get("/databases/{id}/migrations/{mid}/preview", h.migrationPreview)
	r.Post("/databases/{id}/migrations/{mid}/execute", h.migrationExecute)
}

func (h *Handlers) migrationAccess(w http.ResponseWriter, r *http.Request, title string, needEdit bool) (migrationAccess, bool) {
	ctx := r.Context()
	connID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || connID <= 0 {
		http.NotFound(w, r)
		return migrationAccess{}, false
	}
	me, _ := auth.UserFromContext(ctx)

	shell := h.shell(ctx, r, connID, "migrations", title)
	link := h.findLink(connID, shell.Databases)
	if link == nil {
		http.NotFound(w, r)
		return migrationAccess{}, false
	}

	canView, err := h.cfg.Policy.Can(ctx, me.ID, connID, access.ActionView)
	if err != nil {
		h.serverError(w, "cek akses database", err)
		return migrationAccess{}, false
	}
	canEdit := false
	if canView {
		canEdit, _ = h.cfg.Policy.Can(ctx, me.ID, connID, access.ActionEdit)
	}
	if !canView || (needEdit && !canEdit) {
		h.render(w, http.StatusForbidden, "dbm/error_403.html", error403Page{
			AppShell:        shell,
			ConnectionName:  link.Name,
			EffectiveLevel:  h.levelBadge(canView, canEdit),
			RequestedDB:     connID,
			Action:          "Lihat atau ubah migration",
			RequestedAction: "Lihat atau ubah migration",
		})
		return migrationAccess{}, false
	}

	return migrationAccess{ConnID: connID, Link: *link, Shell: shell, CanEdit: canEdit}, true
}

func migrationCrumbs(acc migrationAccess, tail ...web.Crumb) []web.Crumb {
	crumbs := []web.Crumb{
		{Label: acc.Link.Name, URL: "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/schema"},
		{Label: "Migrasi", URL: "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/migrations"},
	}
	return append(crumbs, tail...)
}

func (h *Handlers) loadMigration(w http.ResponseWriter, r *http.Request, acc migrationAccess) (Migration, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return Migration{}, false
	}
	m, err := h.cfg.Migrations.Get(r.Context(), id)
	if errors.Is(err, ErrMigrationNotFound) || m.ConnectionID != acc.ConnID {
		http.NotFound(w, r)
		return Migration{}, false
	}
	if err != nil {
		h.serverError(w, "ambil migration", err)
		return Migration{}, false
	}
	return m, true
}

func (h *Handlers) migrationList(w http.ResponseWriter, r *http.Request) {
	acc, ok := h.migrationAccess(w, r, "Riwayat Migrasi", false)
	if !ok {
		return
	}
	rows, err := h.cfg.Migrations.List(r.Context(), acc.ConnID)
	if err != nil {
		h.serverError(w, "daftar migration", err)
		return
	}

	filter := r.URL.Query().Get("status")
	switch filter {
	case StatusDraft, StatusApplied, StatusFailed:
	default:
		filter = ""
	}
	filtered := make([]Migration, 0, len(rows))
	for _, m := range rows {
		if filter == "" || m.Status == filter {
			filtered = append(filtered, m)
		}
	}

	acc.Shell.Breadcrumbs = migrationCrumbs(acc, web.Crumb{Label: "Riwayat"})
	acc.Shell.Notice = migrationNotices[r.URL.Query().Get("status")]

	h.render(w, http.StatusOK, "dbm/migrations.html", migrationListPage{
		AppShell:   acc.Shell,
		Connection: acc.Link,
		Migrations: filtered,
		Filter:     filter,
		CanEdit:    acc.CanEdit,
	})
}

func (h *Handlers) migrationNew(w http.ResponseWriter, r *http.Request) {
	acc, ok := h.migrationAccess(w, r, "Migration baru", true)
	if !ok {
		return
	}
	acc.Shell.Breadcrumbs = migrationCrumbs(acc, web.Crumb{Label: "Migration baru"})

	h.render(w, http.StatusOK, "dbm/migration_editor.html", migrationEditorPage{
		AppShell:   acc.Shell,
		Connection: acc.Link,
		IsNew:      true,
		FormAction: "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/migrations",
	})
}

func (h *Handlers) migrationCreate(w http.ResponseWriter, r *http.Request) {
	acc, ok := h.migrationAccess(w, r, "Migration baru", true)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	sqlText := strings.TrimSpace(r.PostFormValue("sql"))
	if name == "" {
		h.renderEditor(w, r, acc, migrationEditorPage{SQL: sqlText}, true, "Nama migration belum diisi.")
		return
	}
	if sqlText == "" {
		h.renderEditor(w, r, acc, migrationEditorPage{Name: name}, true, "SQL migration belum diisi.")
		return
	}
	sqlText = InjectName(sqlText, name)

	me, _ := auth.UserFromContext(r.Context())
	m, err := h.cfg.Migrations.CreateDraft(r.Context(), acc.ConnID, sqlText, me.ID)
	if err != nil {
		h.serverError(w, "simpan draft migration", err)
		return
	}
	http.Redirect(w, r, "/databases/"+strconv.FormatInt(acc.ConnID, 10)+
		"/migrations/"+strconv.FormatInt(m.ID, 10)+"?status=saved", http.StatusSeeOther)
}

func (h *Handlers) migrationDetail(w http.ResponseWriter, r *http.Request) {
	acc, ok := h.migrationAccess(w, r, "Detail Migration", false)
	if !ok {
		return
	}
	m, ok := h.loadMigration(w, r, acc)
	if !ok {
		return
	}
	acc.Shell.Breadcrumbs = migrationCrumbs(acc, web.Crumb{Label: "v" + strconv.FormatInt(m.Version, 10)})
	acc.Shell.Notice = migrationNotices[r.URL.Query().Get("status")]

	h.render(w, http.StatusOK, "dbm/migration_detail.html", migrationDetailPage{
		AppShell:   acc.Shell,
		Connection: acc.Link,
		Migration:  m,
		CanEdit:    acc.CanEdit,
	})
}

func (h *Handlers) migrationEdit(w http.ResponseWriter, r *http.Request) {
	acc, ok := h.migrationAccess(w, r, "Ubah migration", true)
	if !ok {
		return
	}
	m, ok := h.loadMigration(w, r, acc)
	if !ok {
		return
	}
	if m.Status != StatusDraft {
		h.render(w, http.StatusBadRequest, "dbm/migration_detail.html", migrationDetailPage{
			AppShell:   acc.Shell,
			Connection: acc.Link,
			Migration:  m,
			CanEdit:    acc.CanEdit,
		})
		return
	}
	acc.Shell.Breadcrumbs = migrationCrumbs(acc,
		web.Crumb{Label: "v" + strconv.FormatInt(m.Version, 10), URL: "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/migrations/" + strconv.FormatInt(m.ID, 10)},
		web.Crumb{Label: "Ubah"})

	h.render(w, http.StatusOK, "dbm/migration_editor.html", migrationEditorPage{
		AppShell:   acc.Shell,
		Connection: acc.Link,
		Migration:  m,
		FormAction: "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/migrations/" + strconv.FormatInt(m.ID, 10),
		SQL:        m.SQL,
		Name:       DeriveName(m.SQL),
	})
}

func (h *Handlers) migrationUpdate(w http.ResponseWriter, r *http.Request) {
	acc, ok := h.migrationAccess(w, r, "Ubah migration", true)
	if !ok {
		return
	}
	m, ok := h.loadMigration(w, r, acc)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	sqlText := strings.TrimSpace(r.PostFormValue("sql"))
	if name == "" {
		h.renderEditor(w, r, acc, migrationEditorPage{Migration: m, SQL: m.SQL, Name: DeriveName(m.SQL)}, false, "Nama migration belum diisi.")
		return
	}
	if sqlText == "" {
		h.renderEditor(w, r, acc, migrationEditorPage{Migration: m, SQL: m.SQL, Name: name}, false, "SQL migration belum diisi.")
		return
	}
	sqlText = InjectName(sqlText, name)
	if err := h.cfg.Migrations.UpdateDraft(r.Context(), m.ID, sqlText); err != nil {
		if errors.Is(err, ErrMigrationNotDraft) {
			h.serverError(w, "ubah draft migration", err)
			return
		}
		h.serverError(w, "ubah draft migration", err)
		return
	}
	http.Redirect(w, r, "/databases/"+strconv.FormatInt(acc.ConnID, 10)+
		"/migrations/"+strconv.FormatInt(m.ID, 10)+"?status=updated", http.StatusSeeOther)
}

func (h *Handlers) renderEditor(w http.ResponseWriter, r *http.Request, acc migrationAccess, page migrationEditorPage, isNew bool, message string) {
	if isNew {
		acc.Shell.Breadcrumbs = migrationCrumbs(acc, web.Crumb{Label: "Migration baru"})
	} else {
		acc.Shell.Breadcrumbs = migrationCrumbs(acc,
			web.Crumb{Label: "v" + strconv.FormatInt(page.Migration.Version, 10), URL: "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/migrations/" + strconv.FormatInt(page.Migration.ID, 10)},
			web.Crumb{Label: "Ubah"})
	}
	page.AppShell = acc.Shell
	page.Connection = acc.Link
	page.IsNew = isNew
	page.Error = message
	if page.Name == "" && page.Migration.ID != 0 {
		page.Name = DeriveName(page.Migration.SQL)
	}
	if page.FormAction == "" {
		if isNew {
			page.FormAction = "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/migrations"
		} else {
			page.FormAction = "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/migrations/" + strconv.FormatInt(page.Migration.ID, 10)
		}
	}
	h.render(w, http.StatusBadRequest, "dbm/migration_editor.html", page)
}

func (h *Handlers) migrationPreview(w http.ResponseWriter, r *http.Request) {
	acc, ok := h.migrationAccess(w, r, "Pratinjau migration", true)
	if !ok {
		return
	}
	m, ok := h.loadMigration(w, r, acc)
	if !ok {
		return
	}
	if m.Status != StatusDraft {
		h.render(w, http.StatusBadRequest, "dbm/migration_detail.html", migrationDetailPage{
			AppShell:   acc.Shell,
			Connection: acc.Link,
			Migration:  m,
			CanEdit:    acc.CanEdit,
		})
		return
	}
	acc.Shell.Breadcrumbs = migrationCrumbs(acc,
		web.Crumb{Label: "v" + strconv.FormatInt(m.Version, 10), URL: "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/migrations/" + strconv.FormatInt(m.ID, 10)},
		web.Crumb{Label: "Pratinjau"})

	h.render(w, http.StatusOK, "dbm/migration_preview.html", migrationPreviewPage{
		AppShell:   acc.Shell,
		Connection: acc.Link,
		Migration:  m,
		FormAction: "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/migrations/" + strconv.FormatInt(m.ID, 10) + "/execute",
	})
}

func (h *Handlers) migrationExecute(w http.ResponseWriter, r *http.Request) {
	acc, ok := h.migrationAccess(w, r, "Jalankan migration", true)
	if !ok {
		return
	}
	m, ok := h.loadMigration(w, r, acc)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}

	confirmName := strings.TrimSpace(r.PostFormValue("confirm_name"))
	if confirmName != acc.Link.Name || r.PostFormValue("confirm_execute") != "on" {
		acc.Shell.Breadcrumbs = migrationCrumbs(acc,
			web.Crumb{Label: "v" + strconv.FormatInt(m.Version, 10), URL: "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/migrations/" + strconv.FormatInt(m.ID, 10)},
			web.Crumb{Label: "Pratinjau"})
		h.render(w, http.StatusBadRequest, "dbm/migration_preview.html", migrationPreviewPage{
			AppShell:   acc.Shell,
			Connection: acc.Link,
			Migration:  m,
			FormAction: "/databases/" + strconv.FormatInt(acc.ConnID, 10) + "/migrations/" + strconv.FormatInt(m.ID, 10) + "/execute",
			Error:      "Konfirmasi belum lengkap. Ketik nama database persis dan centang persetujuan.",
		})
		return
	}

	if m.Status != StatusDraft {
		h.serverError(w, "jalankan migration", ErrMigrationNotDraft)
		return
	}

	me, _ := auth.UserFromContext(r.Context())
	outcome, err := h.runner.Execute(r.Context(), acc.ConnID, m.ID, me.ID)
	if err != nil {
		h.serverError(w, "jalankan migration", err)
		return
	}
	http.Redirect(w, r, "/databases/"+strconv.FormatInt(acc.ConnID, 10)+
		"/migrations/"+strconv.FormatInt(m.ID, 10)+"?status="+outcome, http.StatusSeeOther)
}
