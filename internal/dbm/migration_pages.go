package dbm

// Halaman migration (riwayat, editor draft, detail, preview + eksekusi) di atas
// MigrationStore; izin: baca cukup ActionView, buat draft & eksekusi wajib ActionEdit.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// Helpers bersama

// migrationsHref = URL daftar riwayat migration sebuah koneksi.
func migrationsHref(connID int64) string {
	return "/databases/" + strconv.FormatInt(connID, 10) + "/migrations"
}

// loadConnName mengambil nama koneksi untuk breadcrumb. false = respons sudah
// dikirim (404 bila tak ada, 500 bila gagal).
func (h *Handlers) loadConnName(w http.ResponseWriter, r *http.Request, connID int64) (string, bool) {
	name, err := h.connectionName(r.Context(), connID)
	if errors.Is(err, ErrConnNotFound) {
		http.NotFound(w, r)
		return "", false
	}
	if err != nil {
		h.serverError(w, "nama koneksi", err)
		return "", false
	}
	return name, true
}

// migrationShell membangun shell FE (sidebar database) dengan item "Migrasi"
// ditandai aktif.
func (h *Handlers) migrationShell(ctx context.Context, connID int64, name string) web.Shell {
	sh := h.databaseShell(ctx, connID, name, "")
	if sh.Sidebar != nil {
		sh.Sidebar.MigrationsActive = true
	}
	return sh
}

// loadMigration memuat migration {mID} dan memastikan milik {connID} di URL.
// false = respons sudah dikirim (404/500).
func (h *Handlers) loadMigration(w http.ResponseWriter, r *http.Request, connID int64) (Migration, bool) {
	mID, err := strconv.ParseInt(chi.URLParam(r, "mID"), 10, 64)
	if err != nil || mID <= 0 {
		http.NotFound(w, r)
		return Migration{}, false
	}
	m, err := h.migrations.Get(r.Context(), mID)
	if errors.Is(err, ErrMigrationNotFound) || (err == nil && m.ConnectionID != connID) {
		http.NotFound(w, r)
		return Migration{}, false
	}
	if err != nil {
		h.serverError(w, "ambil migration", err)
		return Migration{}, false
	}
	return m, true
}

// statusCell memetakan status migration ke sel tabel berlabel warna.
func statusCell(status string) web.TableCell {
	variant := "neutral"
	switch status {
	case StatusApplied:
		variant = "success"
	case StatusFailed:
		variant = "error"
	}
	return web.TableCell{Text: status, Variant: variant}
}

// shortChecksum memotong checksum tampilan (cukup 12 karakter di UI).
func shortChecksum(sum string) string {
	if len(sum) > 12 {
		return sum[:12] + "…"
	}
	return sum
}

// Riwayat (daftar)

// migrationsPage = data halaman templates/migrations.html fe-components.
type migrationsPage struct {
	web.Shell
	Crumbs  []web.Crumb
	Actions []web.Button
	Table   web.Table
}

func (h *Handlers) migrationsList(w http.ResponseWriter, r *http.Request) {
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	name, ok := h.loadConnName(w, r, connID)
	if !ok {
		return
	}
	items, err := h.migrations.List(r.Context(), connID)
	if err != nil {
		h.serverError(w, "riwayat migration", err)
		return
	}

	t := web.Table{
		Title: "Daftar Migration",
		Columns: []web.TableColumn{
			{Key: "ver", Label: "Versi", Bold: true},
			{Key: "name", Label: "Nama"},
			{Key: "status", Label: "Status"},
			{Key: "created", Label: "Dibuat"},
			{Key: "executed", Label: "Dijalankan"},
		},
		Empty: "Belum ada migration untuk database ini.",
	}
	base := migrationsHref(connID)
	for _, m := range items {
		creator := m.CreatedByName
		if creator == "" {
			creator = "-"
		}
		executed := "-"
		if m.ExecutedAt != nil {
			executed = m.ExecutedAtLabel()
			if m.ExecutedByName != "" {
				executed += " · " + m.ExecutedByName
			}
		}
		t.Rows = append(t.Rows, web.TableRow{
			"ver":      {Text: strconv.FormatInt(m.Version, 10)},
			"name":     {Text: m.Name(), Href: base + "/" + strconv.FormatInt(m.ID, 10)},
			"status":   statusCell(m.Status),
			"created":  {Text: m.CreatedAtLabel() + " · " + creator},
			"executed": {Text: executed},
		})
	}

	canEdit := false
	if user, ok := auth.UserFromContext(r.Context()); ok {
		allowed, err := h.policy.Can(r.Context(), user.ID, connID, access.ActionEdit)
		canEdit = err == nil && allowed
	}

	actions := []web.Button{
		{Label: "Schema Diff", Variant: "outline", Href: base + "/diff"},
	}
	if canEdit {
		actions = append([]web.Button{{Label: "Migration Baru", IconLeft: "plus", Href: base + "/new"}}, actions...)
	}

	h.render(w, http.StatusOK, "migrations.html", migrationsPage{
		Shell: h.migrationShell(r.Context(), connID, name),
		Crumbs: []web.Crumb{
			{Label: name, Href: "/databases"},
			{Label: "Migrasi"},
		},
		Actions: actions,
		Table:   t,
	})
}

// Editor (draft baru)

// migrationEditorPage = data templates/migration_editor.html fe-components.
type migrationEditorPage struct {
	web.Shell
	Title      string
	Crumbs     []web.Crumb
	FormAction string
	SQL        string
	Note       string
	Actions    []web.Button
}

func (h *Handlers) migrationEditor(w http.ResponseWriter, r *http.Request) {
	connID, ok := h.authorizeAction(w, r, access.ActionEdit)
	if !ok {
		return
	}
	name, ok := h.loadConnName(w, r, connID)
	if !ok {
		return
	}
	base := migrationsHref(connID)
	h.render(w, http.StatusOK, "migration_editor.html", migrationEditorPage{
		Shell: h.migrationShell(r.Context(), connID, name),
		Title: "Migration Baru",
		Crumbs: []web.Crumb{
			{Label: name, Href: "/databases"},
			{Label: "Migrasi", Href: base},
			{Label: "Baru"},
		},
		FormAction: base,
		Note:       "Draft belum menyentuh database target; simpan dulu, lalu periksa lewat Preview.",
		Actions: []web.Button{
			{Label: "Simpan Draft", Type: "submit"},
			{Label: "Batal", Variant: "secondary", Href: base},
		},
	})
}

// migrationsCreate menyimpan SQL sebagai draft lalu membuka halamannya.
func (h *Handlers) migrationsCreate(w http.ResponseWriter, r *http.Request) {
	connID, ok := h.authorizeAction(w, r, access.ActionEdit)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	sqlText := strings.TrimSpace(r.PostFormValue("sql"))
	if sqlText == "" {
		http.Error(w, "SQL migration kosong", http.StatusBadRequest)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	m, err := h.migrations.CreateDraft(r.Context(), connID, sqlText, user.ID)
	if err != nil {
		if errors.Is(err, ErrEmptySQL) {
			http.Error(w, "SQL migration kosong", http.StatusBadRequest)
			return
		}
		h.serverError(w, "simpan draft", err)
		return
	}
	http.Redirect(w, r, migrationsHref(connID)+"/"+strconv.FormatInt(m.ID, 10), http.StatusSeeOther)
}

// Detail

// migrationDetailPage = data templates/migration_detail.html fe-components.
type migrationDetailPage struct {
	web.Shell
	Title      string
	Crumbs     []web.Crumb
	Actions    []web.Button
	StatusText string
	StatusNote string
	SQL        string
	Detail     web.DetailCard
}

func (h *Handlers) migrationDetail(w http.ResponseWriter, r *http.Request) {
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	m, ok := h.loadMigration(w, r, connID)
	if !ok {
		return
	}
	name, ok := h.loadConnName(w, r, connID)
	if !ok {
		return
	}
	base := migrationsHref(connID)

	var statusText, statusNote string
	switch m.Status {
	case StatusDraft:
		statusText = "Draft: belum dieksekusi"
		statusNote = "Database target belum tersentuh. Periksa SQL lewat Preview sebelum dieksekusi."
	case StatusApplied:
		statusText = "Berhasil dieksekusi"
		statusNote = fmt.Sprintf("Transaksi di-commit pada %s", m.ExecutedAtLabel())
		if m.ExecutedByName != "" {
			statusNote += " oleh " + m.ExecutedByName
		}
		statusNote += "."
	case StatusFailed:
		statusText = "Gagal: di-rollback"
		statusNote = "Seluruh perubahan dibatalkan; database tidak berubah."
		if m.Error != "" {
			statusNote += " Galat: " + m.Error
		}
	default:
		statusText = m.Status
		statusNote = "Status tidak dikenal."
	}

	actions := []web.Button{
		{Label: "Kembali ke Riwayat", Variant: "secondary", Href: base},
	}
	if m.Status == StatusDraft {
		actions = append(actions, web.Button{
			Label:     "Preview & Eksekusi",
			IconRight: "arrow-right",
			Href:      base + "/" + strconv.FormatInt(m.ID, 10) + "/preview",
		})
	}

	h.render(w, http.StatusOK, "migration_detail.html", migrationDetailPage{
		Shell: h.migrationShell(r.Context(), connID, name),
		Title: m.Name(),
		Crumbs: []web.Crumb{
			{Label: name, Href: "/databases"},
			{Label: "Migrasi", Href: base},
		},
		Actions:    actions,
		StatusText: statusText,
		StatusNote: statusNote,
		SQL:        m.SQL,
		Detail: web.DetailCard{
			Title: "Pelaksana",
			Fields: []web.DetailField{
				{Label: "Versi", Value: strconv.FormatInt(m.Version, 10)},
				{Label: "Checksum", Value: shortChecksum(m.Checksum)},
				{Label: "Dibuat", Value: m.CreatedAtLabel(), Note: m.CreatedByName},
				{Label: "Dijalankan", Value: m.ExecutedAtLabel(), Note: m.ExecutedByName},
			},
		},
	})
}

// Preview & eksekusi

// migrationPreviewPage = data templates/migration_preview.html fe-components.
type migrationPreviewPage struct {
	web.Shell
	Title         string
	Crumbs        []web.Crumb
	SQL           string
	Warning       string
	Detail        web.DetailCard
	ExecuteAction string
	CanExecute    bool
	Actions       []web.Button
	ConnName      string // nama database tujuan, untuk konfirmasi ketik-nama

	// Analisa rencana eksekusi.
	ExplainAction string
	ExplainErr    string            // kegagalan global (mis. target tak terjangkau)
	ExplainTable  web.Table         // ringkasan per statement; kosong = belum dianalisa
	Plans         []ExplainPlanCard // teks plan per statement
	Analyze       bool              // nilai checkbox terakhir
	CanAnalyze    bool              // EXPLAIN ANALYZE butuh izin edit
}

// ExplainPlanCard = satu blok teks plan statement di halaman preview.
type ExplainPlanCard struct {
	Title string
	Tree  string
}

// previewData membangun data halaman preview (dipakai GET preview & hasil EXPLAIN).
func (h *Handlers) previewData(r *http.Request, connID int64, m Migration, name string) migrationPreviewPage {
	base := migrationsHref(connID)
	detailHref := base + "/" + strconv.FormatInt(m.ID, 10)

	canEdit := false
	if user, ok := auth.UserFromContext(r.Context()); ok {
		allowed, err := h.policy.Can(r.Context(), user.ID, connID, access.ActionEdit)
		canEdit = err == nil && allowed
	}

	var warning string
	canExecute := m.Status == StatusDraft && h.runner != nil && canEdit
	switch m.Status {
	case StatusDraft:
		warning = "Draft ini belum pernah dieksekusi. Eksekusi berjalan dalam satu transaksi; gagal berarti rollback penuh."
	case StatusApplied:
		warning = "Migration ini sudah dieksekusi pada " + m.ExecutedAtLabel() + "; tidak bisa dijalankan ulang."
	case StatusFailed:
		warning = "Percobaan sebelumnya gagal dan sudah di-rollback. Perbaiki SQL sebelum mencoba lagi."
	}

	// Tombol eksekusi di kartu "Konfirmasi eksekusi"
	actions := []web.Button{
		{Label: "Batal", Variant: "secondary", Href: detailHref},
	}

	return migrationPreviewPage{
		Shell: h.migrationShell(r.Context(), connID, name),
		Title: "Preview Migration",
		Crumbs: []web.Crumb{
			{Label: name, Href: "/databases"},
			{Label: "Migrasi", Href: base},
			{Label: "Preview"},
		},
		SQL:     m.SQL,
		Warning: warning,
		Detail: web.DetailCard{
			Title: "Migration",
			Fields: []web.DetailField{
				{Label: "Status", Value: m.Status},
				{Label: "Versi", Value: strconv.FormatInt(m.Version, 10)},
				{Label: "Dibuat", Value: m.CreatedAtLabel(), Note: m.CreatedByName},
			},
		},
		ExecuteAction: detailHref + "/execute",
		ExplainAction: detailHref + "/explain",
		CanExecute:    canExecute,
		CanAnalyze:    canEdit,
		Actions:       actions,
		ConnName:      name,
	}
}

func (h *Handlers) migrationPreview(w http.ResponseWriter, r *http.Request) {
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	m, ok := h.loadMigration(w, r, connID)
	if !ok {
		return
	}
	name, ok := h.loadConnName(w, r, connID)
	if !ok {
		return
	}
	h.render(w, http.StatusOK, "migration_preview.html", h.previewData(r, connID, m, name))
}

// migrationExplain jalanin EXPLAIN di seluruh statement migration & render ulang preview
func (h *Handlers) migrationExplain(w http.ResponseWriter, r *http.Request) {
	analyze := r.PostFormValue("analyze") != ""
	action := access.ActionView
	if analyze {
		action = access.ActionEdit
	}
	connID, ok := h.authorizeAction(w, r, action)
	if !ok {
		return
	}
	m, ok := h.loadMigration(w, r, connID)
	if !ok {
		return
	}
	name, _ := h.connectionName(r.Context(), connID)

	page := h.previewData(r, connID, m, name)
	page.Analyze = analyze

	pool, err := h.registry.Get(r.Context(), connID)
	if err != nil || pool == nil {
		page.ExplainErr = "Database tidak dapat dihubungi; analisa tidak dijalankan."
		h.render(w, http.StatusOK, "migration_preview.html", page)
		return
	}

	items := ExplainAll(r.Context(), pool, m.SQL, analyze)
	mode := "tanpa ANALYZE"
	if analyze {
		mode = "dengan ANALYZE (di-rollback)"
	}
	page.ExplainTable = web.Table{
		Title: "Ringkasan rencana eksekusi",
		Tag:   web.TableCell{Text: mode, Variant: "neutral"},
		Columns: []web.TableColumn{
			{Key: "no", Label: "#"},
			{Key: "stmt", Label: "Statement"},
			{Key: "plan", Label: "Scan / Join"},
			{Key: "idx", Label: "Index"},
			{Key: "cost", Label: "Est. Cost"},
			{Key: "time", Label: "Aktual (ms)"},
			{Key: "rows", Label: "Rows"},
			{Key: "ket", Label: "Keterangan"},
		},
		Empty: "Tidak ada statement untuk dianalisa.",
	}
	for _, it := range items {
		scanJoin := it.Summary.Scan
		if it.Summary.Join != "-" {
			if scanJoin == "-" {
				scanJoin = it.Summary.Join
			} else {
				scanJoin += " · " + it.Summary.Join
			}
		}
		cost, timeStr, rows, ket := "-", "-", "-", it.Note
		if it.Tree != "" {
			cost = it.Summary.Cost
			rows = it.Summary.Rows
			if it.Analyze {
				timeStr = it.Summary.ActualTime
			}
			page.Plans = append(page.Plans, ExplainPlanCard{
				Title: fmt.Sprintf("Statement %d: %s", it.N, it.Stmt),
				Tree:  it.Tree,
			})
		}
		page.ExplainTable.Rows = append(page.ExplainTable.Rows, web.TableRow{
			"no":   {Text: strconv.Itoa(it.N)},
			"stmt": {Text: it.Stmt},
			"plan": {Text: scanJoin},
			"idx":  {Text: it.Summary.Index},
			"cost": {Text: cost},
			"time": {Text: timeStr},
			"rows": {Text: rows},
			"ket":  {Text: ket},
		})
	}
	h.render(w, http.StatusOK, "migration_preview.html", page)
}

// migrationExecute menjalankan draft via MigrationRunner (transaksional).
// Galat SQL dicatat store sebagai status failed; kegagalan operasional
// di-log server dan user kembali ke detail.
func (h *Handlers) migrationExecute(w http.ResponseWriter, r *http.Request) {
	connID, ok := h.authorizeAction(w, r, access.ActionEdit)
	if !ok {
		return
	}
	m, ok := h.loadMigration(w, r, connID)
	if !ok {
		return
	}
	detailHref := migrationsHref(connID) + "/" + strconv.FormatInt(m.ID, 10)
	if m.Status != StatusDraft || h.runner == nil {
		http.Redirect(w, r, detailHref, http.StatusSeeOther)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if _, err := h.runner.Execute(r.Context(), connID, m.ID, user.ID); err != nil {
		log.Printf("dbm: eksekusi migration %d (conn %d): %v", m.ID, connID, err)
	}
	http.Redirect(w, r, detailHref, http.StatusSeeOther)
}
