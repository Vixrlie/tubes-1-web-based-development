package dbm

// Halaman server-rendered workflow migration. Memakai Workflow yang sama
// dengan JSON API (api.go), jadi aturan izin dan state identik.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// migrationPageRoutes dipasang dari Routes di dalam r.Route("/databases").
func (h *Handlers) migrationPageRoutes(r chi.Router) {
	r.Get("/{connID}/migrations", h.migrationsList)
	r.Post("/{connID}/migrations", h.migrationsCreate)
	r.Get("/{connID}/migrations/new", h.migrationEditor)
	r.Post("/{connID}/migrations/new", h.migrationEditor) // prefill dari Schema Diff
	r.Get("/{connID}/migrations/{mID}", h.migrationDetail)
	r.Get("/{connID}/migrations/{mID}/edit", h.migrationEdit)
	r.Post("/{connID}/migrations/{mID}", h.migrationUpdate)
	r.Post("/{connID}/migrations/{mID}/validate", h.migrationValidate)
	r.Post("/{connID}/migrations/{mID}/requests", h.migrationSubmit)
	r.Get("/{connID}/migrations/{mID}/preview", h.migrationPreview)
	r.Post("/{connID}/migrations/{mID}/explain", h.migrationExplain)
	r.Get("/{connID}/requests/{rID}", h.requestDetail)
	r.Post("/{connID}/requests/{rID}/approve", h.requestApprove)
	r.Post("/{connID}/requests/{rID}/reject", h.requestReject)
	r.Post("/{connID}/requests/{rID}/cancel", h.requestCancel)
	r.Get("/{connID}/executions/{eID}", h.executionDetail)
}

func migrationsHref(connID int64) string {
	return "/databases/" + strconv.FormatInt(connID, 10) + "/migrations"
}

func migrationHref(connID, mID int64) string {
	return migrationsHref(connID) + "/" + strconv.FormatInt(mID, 10)
}

func requestHref(connID, rID int64) string {
	return fmt.Sprintf("/databases/%d/requests/%d", connID, rID)
}

func executionHref(connID, eID int64) string {
	return fmt.Sprintf("/databases/%d/executions/%d", connID, eID)
}

// withNotice menambahkan pesan umpan balik (?notice= atau ?error=) ke URL.
func withNotice(href, key, msg string) string {
	sep := "?"
	if strings.Contains(href, "?") {
		sep = "&"
	}
	return href + sep + key + "=" + url.QueryEscape(msg)
}

// Feedback = pesan umpan balik satu kali dari redirect.
type Feedback struct {
	Notice string
	Error  string
}

func feedbackFrom(r *http.Request) Feedback {
	q := r.URL.Query()
	return Feedback{Notice: q.Get("notice"), Error: q.Get("error")}
}

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

func (h *Handlers) migrationShell(ctx context.Context, connID int64, name string) web.Shell {
	sh := h.databaseShell(ctx, connID, name, "")
	if sh.Sidebar != nil {
		sh.Sidebar.MigrationsActive = true
	}
	return sh
}

// pageFail menerjemahkan galat Workflow ke respons halaman.
func (h *Handlers) pageFail(w http.ResponseWriter, r *http.Request, err error) {
	status, _ := apiStatus(err)
	switch status {
	case http.StatusForbidden:
		http.Error(w, "forbidden", http.StatusForbidden)
	case http.StatusNotFound:
		http.NotFound(w, r)
	case http.StatusInternalServerError:
		h.serverError(w, r.URL.Path, err)
	default:
		http.Error(w, err.Error(), status)
	}
}

// redirectResult kembali ke back dengan pesan sukses, atau pesan galat bila
// galatnya galat domain (4xx). Galat server tetap 500.
func (h *Handlers) redirectResult(w http.ResponseWriter, r *http.Request, back, okMsg string, err error) {
	if err == nil {
		http.Redirect(w, r, withNotice(back, "notice", okMsg), http.StatusSeeOther)
		return
	}
	status, _ := apiStatus(err)
	if status == http.StatusForbidden || status == http.StatusNotFound || status == http.StatusInternalServerError {
		h.pageFail(w, r, err)
		return
	}
	http.Redirect(w, r, withNotice(back, "error", err.Error()), http.StatusSeeOther)
}

func pageUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
	}
	return user, ok
}

func pageID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || v <= 0 {
		http.NotFound(w, r)
		return 0, false
	}
	return v, true
}

func timeLabel(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("02 Jan 2006 15:04:05")
}

func execLabel(id int64) string { return "E" + strconv.FormatInt(id, 10) }

func actionLabel(e Execution) string {
	switch e.Action {
	case ActionRollback:
		if e.RevertsExecutionID != nil {
			return "ROLLBACK " + execLabel(*e.RevertsExecutionID)
		}
		return "ROLLBACK"
	case ActionForward:
		return "FORWARD"
	}
	return "UP"
}

var stateLabels = map[string]web.TableCell{
	StateDraft:      {Text: "Draft", Variant: "neutral"},
	StateActive:     {Text: "Aktif", Variant: "success"},
	StateRolledBack: {Text: "Di-rollback", Variant: "warning"},
	StateFailed:     {Text: "Gagal", Variant: "error"},
}

var requestLabels = map[string]web.TableCell{
	RequestPendingReview: {Text: "Pending Review", Variant: "info"},
	RequestRejected:      {Text: "Ditolak", Variant: "error"},
	RequestApproved:      {Text: "Approved", Variant: "info"},
	RequestQueued:        {Text: "Pending (Queue)", Variant: "warning"},
	RequestRunning:       {Text: "Running", Variant: "warning"},
	RequestSuccess:       {Text: "Sukses", Variant: "success"},
	RequestFailed:        {Text: "Gagal", Variant: "error"},
	RequestCancelled:     {Text: "Dibatalkan", Variant: "neutral"},
}

func execStatusCell(status string) web.TableCell {
	switch status {
	case ExecSuccess:
		return web.TableCell{Text: "Sukses", Variant: "success"}
	case ExecFailed:
		return web.TableCell{Text: "Gagal", Variant: "error"}
	}
	return web.TableCell{Text: "Running", Variant: "warning"}
}

func reviewCell(decision string) web.TableCell {
	switch decision {
	case ReviewApproved:
		return web.TableCell{Text: "Approved", Variant: "success"}
	case ReviewRejected:
		return web.TableCell{Text: "Rejected", Variant: "error"}
	}
	return web.TableCell{Text: "Pending", Variant: "info"}
}

func levelText(level string) string {
	if level == string(access.ActionEdit) {
		return "View + Edit"
	}
	return "View"
}

func actionText(action string) string {
	switch action {
	case ActionRollback:
		return "Rollback"
	case ActionForward:
		return "Forward"
	}
	return "Up"
}

// Riwayat & daftar

type migrationsPage struct {
	web.Shell
	Feedback
	Crumbs       []web.Crumb
	Actions      []web.Button
	HideRollback bool
	ToggleHref   string
	History      web.Table
	Pending      web.Table
	Migrations   web.Table
}

func (h *Handlers) migrationsList(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	name, ok := h.loadConnName(w, r, connID)
	if !ok {
		return
	}
	ctx := r.Context()
	hide := parseBool(r.URL.Query().Get("hide_rollback"))

	execs, err := h.workflow.History(ctx, user, connID, hide)
	if err != nil {
		h.pageFail(w, r, err)
		return
	}
	defs, err := h.workflow.ListMigrations(ctx, user, connID)
	if err != nil {
		h.pageFail(w, r, err)
		return
	}
	open, err := h.workflow.ListRequests(ctx, user, connID,
		[]string{RequestPendingReview, RequestApproved, RequestQueued, RequestRunning})
	if err != nil {
		h.pageFail(w, r, err)
		return
	}

	history := web.Table{
		Title: "Riwayat Eksekusi",
		Columns: []web.TableColumn{
			{Key: "id", Label: "Execution ID", Bold: true},
			{Key: "mig", Label: "Migration / Action"},
			{Key: "actor", Label: "Actor"},
			{Key: "at", Label: "Executed at"},
			{Key: "status", Label: "Status"},
		},
		Empty: "Belum ada eksekusi migration pada database ini.",
	}
	for _, e := range execs {
		actor := e.ExecutedByName
		if actor == "" {
			actor = "-"
		}
		history.Rows = append(history.Rows, web.TableRow{
			"id":     {Text: execLabel(e.ID), Href: executionHref(connID, e.ID)},
			"mig":    {Text: fmt.Sprintf("v%d · %s · %s", e.Version, e.MigrationName, actionLabel(e))},
			"actor":  {Text: actor},
			"at":     {Text: e.StartedAt.Format("2006-01-02 15:04:05")},
			"status": execStatusCell(e.Status),
		})
	}

	pending := web.Table{
		Title: "Pengajuan Berjalan",
		Columns: []web.TableColumn{
			{Key: "id", Label: "Pengajuan", Bold: true},
			{Key: "mig", Label: "Migration"},
			{Key: "action", Label: "Aksi"},
			{Key: "by", Label: "Diajukan oleh"},
			{Key: "status", Label: "Status"},
		},
		Empty: "Tidak ada pengajuan yang sedang berjalan.",
	}
	for _, rq := range open {
		pending.Rows = append(pending.Rows, web.TableRow{
			"id":     {Text: "#" + strconv.FormatInt(rq.ID, 10), Href: requestHref(connID, rq.ID)},
			"mig":    {Text: fmt.Sprintf("v%d · %s", rq.MigrationVersion, rq.MigrationName)},
			"action": {Text: actionText(rq.Action)},
			"by":     {Text: rq.RequestedByName},
			"status": requestLabels[rq.Status],
		})
	}

	migrations := web.Table{
		Title: "Daftar Migration",
		Columns: []web.TableColumn{
			{Key: "ver", Label: "Versi", Bold: true},
			{Key: "name", Label: "Nama"},
			{Key: "rev", Label: "Revisi"},
			{Key: "kind", Label: "Jenis"},
			{Key: "state", Label: "Posisi"},
		},
		Empty: "Belum ada migration untuk database ini.",
	}
	for _, m := range defs {
		kind := "Reversible"
		if !m.Reversible() {
			kind = "Irreversible"
		}
		migrations.Rows = append(migrations.Rows, web.TableRow{
			"ver":   {Text: strconv.FormatInt(m.Version, 10)},
			"name":  {Text: m.Name, Href: migrationHref(connID, m.ID)},
			"rev":   {Text: strconv.Itoa(m.Revision)},
			"kind":  {Text: kind},
			"state": stateLabels[m.State],
		})
	}

	base := migrationsHref(connID)
	actions := []web.Button{
		{Label: "Persetujuan", Variant: "outline", Href: "/approvals"},
		{Label: "Schema Diff", Variant: "outline", Href: base + "/diff"},
	}
	if h.can(ctx, user, connID, true) {
		actions = append([]web.Button{{Label: "Migrasi Baru", IconLeft: "plus", Href: base + "/new"}}, actions...)
	}
	toggle := base + "?hide_rollback=1"
	if hide {
		toggle = base
	}

	h.render(w, http.StatusOK, "migrations.html", migrationsPage{
		Shell:        h.migrationShell(ctx, connID, name),
		Feedback:     feedbackFrom(r),
		Crumbs:       []web.Crumb{{Label: name, Href: "/databases"}, {Label: "Riwayat Migrasi"}},
		Actions:      actions,
		HideRollback: hide,
		ToggleHref:   toggle,
		History:      history,
		Pending:      pending,
		Migrations:   migrations,
	})
}

// Editor

type migrationEditorPage struct {
	web.Shell
	Feedback
	Title      string
	Crumbs     []web.Crumb
	FormAction string
	Input      MigrationInput
	Revision   int
	Actions    []web.Button
}

func formInput(r *http.Request) MigrationInput {
	up := r.PostFormValue("up_sql")
	if up == "" {
		up = r.PostFormValue("sql") // dari Schema Diff
	}
	return MigrationInput{
		Name:               r.PostFormValue("name"),
		UpSQL:              up,
		DownSQL:            r.PostFormValue("down_sql"),
		Irreversible:       r.PostFormValue("irreversible") != "",
		IrreversibleReason: r.PostFormValue("irreversible_reason"),
	}
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
	var in MigrationInput
	if r.Method == http.MethodPost {
		in = formInput(r)
	}
	base := migrationsHref(connID)
	h.render(w, http.StatusOK, "migration_editor.html", migrationEditorPage{
		Shell:      h.migrationShell(r.Context(), connID, name),
		Feedback:   feedbackFrom(r),
		Title:      "Create Migration",
		Crumbs:     []web.Crumb{{Label: name, Href: "/databases"}, {Label: "Migrasi", Href: base}, {Label: "Baru"}},
		FormAction: base,
		Input:      in,
		Actions: []web.Button{
			{Label: "Batal", Variant: "secondary", Href: base},
			{Label: "Simpan Draft", Type: "submit"},
		},
	})
}

func (h *Handlers) migrationsCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := pageID(w, r, "connID")
	if !ok {
		return
	}
	in := formInput(r)
	m, err := h.workflow.CreateMigration(r.Context(), user, connID, in)
	if err != nil {
		status, _ := apiStatus(err)
		if status == http.StatusUnprocessableEntity {
			h.renderEditorError(w, r, connID, in, 0, err)
			return
		}
		h.pageFail(w, r, err)
		return
	}
	http.Redirect(w, r, withNotice(migrationHref(connID, m.ID), "notice",
		"Draft tersimpan. Jalankan validasi sebelum mengajukan review."), http.StatusSeeOther)
}

func (h *Handlers) renderEditorError(w http.ResponseWriter, r *http.Request, connID int64, in MigrationInput, mID int64, err error) {
	name, _ := h.connectionName(r.Context(), connID)
	base := migrationsHref(connID)
	page := migrationEditorPage{
		Shell:      h.migrationShell(r.Context(), connID, name),
		Feedback:   Feedback{Error: err.Error()},
		Title:      "Create Migration",
		Crumbs:     []web.Crumb{{Label: name, Href: "/databases"}, {Label: "Migrasi", Href: base}, {Label: "Baru"}},
		FormAction: base,
		Input:      in,
		Actions: []web.Button{
			{Label: "Batal", Variant: "secondary", Href: base},
			{Label: "Simpan Draft", Type: "submit"},
		},
	}
	if mID != 0 {
		page.Title = "Edit Migration"
		page.FormAction = migrationHref(connID, mID)
		page.Actions[0].Href = migrationHref(connID, mID)
		page.Actions[1].Label = "Simpan Revisi"
	}
	h.render(w, http.StatusUnprocessableEntity, "migration_editor.html", page)
}

func (h *Handlers) migrationEdit(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := h.authorizeAction(w, r, access.ActionEdit)
	if !ok {
		return
	}
	mID, ok := pageID(w, r, "mID")
	if !ok {
		return
	}
	m, err := h.workflow.GetMigration(r.Context(), user, connID, mID)
	if err != nil {
		h.pageFail(w, r, err)
		return
	}
	if !m.Editable() {
		reason := ErrMigrationInReview
		if m.Locked() {
			reason = ErrMigrationLocked
		}
		http.Redirect(w, r, withNotice(migrationHref(connID, mID), "error", reason.Error()), http.StatusSeeOther)
		return
	}
	name, _ := h.connectionName(r.Context(), connID)
	detail := migrationHref(connID, mID)
	h.render(w, http.StatusOK, "migration_editor.html", migrationEditorPage{
		Shell:  h.migrationShell(r.Context(), connID, name),
		Title:  "Edit Migration",
		Crumbs: []web.Crumb{{Label: name, Href: "/databases"}, {Label: "Migrasi", Href: migrationsHref(connID)}, {Label: m.Name, Href: detail}, {Label: "Edit"}},
		Input: MigrationInput{
			Name: m.Name, UpSQL: m.UpSQL, DownSQL: m.DownSQL,
			Irreversible: !m.Reversible(), IrreversibleReason: m.IrreversibleReason,
		},
		Revision:   m.Revision,
		FormAction: detail,
		Actions: []web.Button{
			{Label: "Batal", Variant: "secondary", Href: detail},
			{Label: "Simpan Revisi", Type: "submit"},
		},
	})
}

func (h *Handlers) migrationUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := pageID(w, r, "connID")
	if !ok {
		return
	}
	mID, ok := pageID(w, r, "mID")
	if !ok {
		return
	}
	in := formInput(r)
	m, err := h.workflow.UpdateMigration(r.Context(), user, connID, mID, in)
	if err != nil {
		if status, _ := apiStatus(err); status == http.StatusUnprocessableEntity {
			h.renderEditorError(w, r, connID, in, mID, err)
			return
		}
		h.redirectResult(w, r, migrationHref(connID, mID), "", err)
		return
	}
	h.redirectResult(w, r, migrationHref(connID, mID),
		fmt.Sprintf("Revisi %d tersimpan. Validasi ulang sebelum mengajukan.", m.Revision), nil)
}

// Detail migration

type actionForm struct {
	Action string
	Label  string
	Hint   string
	Danger bool
}

type migrationDetailPage struct {
	web.Shell
	Feedback
	Title         string
	Crumbs        []web.Crumb
	Actions       []web.Button
	Migration     Migration
	StateCell     web.TableCell
	Detail        web.DetailCard
	CanEdit       bool
	ValidateURL   string
	SubmitURL     string
	SubmitActions []actionForm
	OpenRequest   string
	Requests      web.Table
	Executions    web.Table
}

func (h *Handlers) migrationDetail(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	mID, ok := pageID(w, r, "mID")
	if !ok {
		return
	}
	d, err := h.loadMigrationDetail(r, user, connID, mID)
	if err != nil {
		h.pageFail(w, r, err)
		return
	}
	name, ok := h.loadConnName(w, r, connID)
	if !ok {
		return
	}
	m := d.Migration
	detailURL := migrationHref(connID, m.ID)

	actions := []web.Button{
		{Label: "Kembali ke Riwayat", Variant: "secondary", Href: migrationsHref(connID)},
		{Label: "EXPLAIN", Variant: "outline", Href: detailURL + "/preview"},
	}
	if d.CanEdit && m.Editable() {
		actions = append(actions, web.Button{Label: "Edit SQL", Variant: "outline", Href: detailURL + "/edit"})
	}

	var forms []actionForm
	for _, a := range d.AllowedActions {
		switch a {
		case ActionUp:
			forms = append(forms, actionForm{Action: a, Label: "Ajukan Review (Up)", Hint: "Menjalankan SQL up setelah seluruh leader group menyetujui."})
		case ActionForward:
			forms = append(forms, actionForm{Action: a, Label: "Ajukan Forward", Hint: "Menjalankan kembali SQL up asli revisi ini."})
		case ActionRollback:
			forms = append(forms, actionForm{Action: a, Label: "Ajukan Rollback", Hint: fmt.Sprintf("Menjalankan SQL down untuk membatalkan %s.", execLabel(m.ActiveExecutionID)), Danger: true})
		}
	}

	validation := "Belum divalidasi"
	if m.Validation.Checked {
		switch {
		case m.Validation.OK && m.Validation.Current:
			validation = "Lolos"
		case m.Validation.OK:
			validation = "Lolos (revisi lama)"
		default:
			validation = "Gagal"
		}
	}
	kind := "Reversible"
	if !m.Reversible() {
		kind = "Irreversible: " + m.IrreversibleReason
	}
	locked := "Belum dieksekusi (masih bisa diubah)"
	if m.Locked() {
		locked = "Immutable sejak " + timeLabel(m.LockedAt)
	}

	page := migrationDetailPage{
		Shell:     h.migrationShell(r.Context(), connID, name),
		Feedback:  feedbackFrom(r),
		Title:     fmt.Sprintf("v%d · %s", m.Version, m.Name),
		Crumbs:    []web.Crumb{{Label: name, Href: "/databases"}, {Label: "Migrasi", Href: migrationsHref(connID)}, {Label: m.Name}},
		Actions:   actions,
		Migration: m,
		StateCell: stateLabels[m.State],
		Detail: web.DetailCard{
			Title: "Informasi Migration",
			Fields: []web.DetailField{
				{Label: "Versi", Value: strconv.FormatInt(m.Version, 10)},
				{Label: "Revisi", Value: strconv.Itoa(m.Revision), Note: shortChecksum(m.Checksum)},
				{Label: "Jenis", Value: kind},
				{Label: "Definisi", Value: locked},
				{Label: "Validasi", Value: validation, Note: m.Validation.Message},
				{Label: "Dibuat", Value: m.CreatedAtLabel(), Note: m.CreatedByName},
			},
		},
		CanEdit:       d.CanEdit,
		ValidateURL:   detailURL + "/validate",
		SubmitURL:     detailURL + "/requests",
		SubmitActions: forms,
	}
	if m.OpenRequest != nil {
		page.OpenRequest = requestHref(connID, m.OpenRequest.ID)
	}

	page.Requests = web.Table{
		Title: "Pengajuan",
		Columns: []web.TableColumn{
			{Key: "id", Label: "Pengajuan", Bold: true},
			{Key: "action", Label: "Aksi"},
			{Key: "by", Label: "Diajukan oleh"},
			{Key: "at", Label: "Waktu"},
			{Key: "status", Label: "Status"},
		},
		Empty: "Belum ada pengajuan.",
	}
	for _, rq := range d.Requests {
		page.Requests.Rows = append(page.Requests.Rows, web.TableRow{
			"id":     {Text: "#" + strconv.FormatInt(rq.ID, 10), Href: requestHref(connID, rq.ID)},
			"action": {Text: actionText(rq.Action)},
			"by":     {Text: rq.RequestedByName},
			"at":     {Text: rq.CreatedAt.Format("2006-01-02 15:04")},
			"status": requestLabels[rq.Status],
		})
	}
	page.Executions = executionTable("Riwayat Eksekusi Migration Ini", connID, d.Executions)
	h.render(w, http.StatusOK, "migration_detail.html", page)
}

func executionTable(title string, connID int64, execs []Execution) web.Table {
	t := web.Table{
		Title: title,
		Columns: []web.TableColumn{
			{Key: "id", Label: "Execution ID", Bold: true},
			{Key: "action", Label: "Aksi"},
			{Key: "attempt", Label: "Attempt"},
			{Key: "at", Label: "Mulai"},
			{Key: "dur", Label: "Durasi"},
			{Key: "status", Label: "Status"},
		},
		Empty: "Belum ada eksekusi.",
	}
	for _, e := range execs {
		dur := "-"
		if e.DurationMS != nil {
			dur = strconv.FormatInt(*e.DurationMS, 10) + " ms"
		}
		t.Rows = append(t.Rows, web.TableRow{
			"id":      {Text: execLabel(e.ID), Href: executionHref(connID, e.ID)},
			"action":  {Text: actionLabel(e)},
			"attempt": {Text: strconv.Itoa(e.Attempt)},
			"at":      {Text: e.StartedAt.Format("2006-01-02 15:04:05")},
			"dur":     {Text: dur},
			"status":  execStatusCell(e.Status),
		})
	}
	return t
}

func shortChecksum(sum string) string {
	if len(sum) > 12 {
		return sum[:12] + "…"
	}
	return sum
}

func (h *Handlers) migrationValidate(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := pageID(w, r, "connID")
	if !ok {
		return
	}
	mID, ok := pageID(w, r, "mID")
	if !ok {
		return
	}
	res, err := h.workflow.Validate(r.Context(), user, connID, mID)
	back := migrationHref(connID, mID)
	if err != nil {
		h.redirectResult(w, r, back, "", err)
		return
	}
	key := "notice"
	if !res.OK {
		key = "error"
	}
	http.Redirect(w, r, withNotice(back, key, "Validasi: "+res.Message), http.StatusSeeOther)
}

func (h *Handlers) migrationSubmit(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := pageID(w, r, "connID")
	if !ok {
		return
	}
	mID, ok := pageID(w, r, "mID")
	if !ok {
		return
	}
	req, err := h.workflow.Submit(r.Context(), user, connID, mID, r.PostFormValue("action"), r.PostFormValue("note"))
	if err != nil {
		h.redirectResult(w, r, migrationHref(connID, mID), "", err)
		return
	}
	h.redirectResult(w, r, requestHref(connID, req.ID),
		"Pengajuan "+actionText(req.Action)+" dikirim dan menunggu review leader setiap group.", nil)
}

// Pengajuan & approval

type requestDetailPage struct {
	web.Shell
	Feedback
	Title       string
	Crumbs      []web.Crumb
	Actions     []web.Button
	Request     Request
	StatusCell  web.TableCell
	Detail      web.DetailCard
	Reviewers   web.Table
	Executions  web.Table
	ApproveURL  string
	RejectURL   string
	CancelURL   string
	AutoRefresh bool
}

func (h *Handlers) requestDetail(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	rID, ok := pageID(w, r, "rID")
	if !ok {
		return
	}
	req, err := h.workflow.GetRequest(r.Context(), user, rID)
	if err == nil && req.ConnectionID != connID {
		err = ErrRequestNotFound
	}
	if err != nil {
		h.pageFail(w, r, err)
		return
	}
	name, ok := h.loadConnName(w, r, connID)
	if !ok {
		return
	}

	reviewers := web.Table{
		Title: "Approval per Group",
		Columns: []web.TableColumn{
			{Key: "group", Label: "Group", Bold: true},
			{Key: "leader", Label: "Leader"},
			{Key: "level", Label: "Akses"},
			{Key: "note", Label: "Catatan"},
			{Key: "decision", Label: "Keputusan"},
		},
		Empty: "Tidak ada group yang memiliki akses ke database ini.",
	}
	for _, g := range req.Reviewers {
		leader := g.LeaderName
		if leader == "" {
			leader = "belum ditetapkan"
		}
		reviewers.Rows = append(reviewers.Rows, web.TableRow{
			"group":    {Text: g.GroupName},
			"leader":   {Text: leader},
			"level":    {Text: levelText(g.Level)},
			"note":     {Text: g.Note},
			"decision": reviewCell(g.Decision),
		})
	}

	target := "-"
	if req.TargetExecutionID != nil {
		target = execLabel(*req.TargetExecutionID)
	}
	job := "-"
	if req.JobID != nil {
		job = fmt.Sprintf("#%d (%s, attempt %d)", *req.JobID, req.JobStatus, req.JobAttempts)
	}
	base := requestHref(connID, rID)
	h.render(w, http.StatusOK, "request_detail.html", requestDetailPage{
		Shell:    h.migrationShell(r.Context(), connID, name),
		Feedback: feedbackFrom(r),
		Title:    fmt.Sprintf("Review %s v%d · %s", actionText(req.Action), req.MigrationVersion, req.MigrationName),
		Crumbs: []web.Crumb{
			{Label: name, Href: "/databases"},
			{Label: "Migrasi", Href: migrationsHref(connID)},
			{Label: req.MigrationName, Href: migrationHref(connID, req.MigrationID)},
			{Label: "Pengajuan #" + strconv.FormatInt(req.ID, 10)},
		},
		Actions: []web.Button{
			{Label: "Migration", Variant: "secondary", Href: migrationHref(connID, req.MigrationID)},
			{Label: "Riwayat", Variant: "outline", Href: migrationsHref(connID)},
		},
		Request:    req,
		StatusCell: requestLabels[req.Status],
		Detail: web.DetailCard{
			Title: "Informasi Pengajuan",
			Fields: []web.DetailField{
				{Label: "Aksi", Value: actionText(req.Action), Note: "target " + target},
				{Label: "Revisi", Value: strconv.Itoa(req.Revision), Note: shortChecksum(req.Checksum)},
				{Label: "Diajukan", Value: req.CreatedAt.Format("02 Jan 2006 15:04"), Note: req.RequestedByName},
				{Label: "Diputuskan", Value: timeLabel(req.DecidedAt)},
				{Label: "Masuk antrian", Value: timeLabel(req.QueuedAt), Note: "job " + job},
				{Label: "Selesai", Value: timeLabel(req.FinishedAt)},
			},
		},
		Reviewers:   reviewers,
		Executions:  executionTable("Attempt Eksekusi", connID, req.Executions),
		ApproveURL:  base + "/approve",
		RejectURL:   base + "/reject",
		CancelURL:   base + "/cancel",
		AutoRefresh: req.Status == RequestQueued || req.Status == RequestRunning || req.Status == RequestApproved,
	})
}

func (h *Handlers) requestApprove(w http.ResponseWriter, r *http.Request) {
	h.requestDecide(w, r, true)
}

func (h *Handlers) requestReject(w http.ResponseWriter, r *http.Request) {
	h.requestDecide(w, r, false)
}

func (h *Handlers) requestDecide(w http.ResponseWriter, r *http.Request, approve bool) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := pageID(w, r, "connID")
	if !ok {
		return
	}
	rID, ok := pageID(w, r, "rID")
	if !ok {
		return
	}
	back := requestHref(connID, rID)
	note := r.PostFormValue("note")
	var res Decision
	var err error
	if approve {
		res, err = h.workflow.Approve(r.Context(), user, rID, note)
	} else {
		res, err = h.workflow.Reject(r.Context(), user, rID, note)
	}
	if err != nil {
		h.redirectResult(w, r, back, "", err)
		return
	}
	msg := "Penolakan tercatat."
	if approve {
		msg = "Persetujuan tercatat; menunggu group lain."
		if res.JobID != 0 {
			msg = fmt.Sprintf("Seluruh group menyetujui. Job #%d masuk antrian dan dijalankan worker.", res.JobID)
		}
	}
	h.redirectResult(w, r, back, msg, nil)
}

func (h *Handlers) requestCancel(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := pageID(w, r, "connID")
	if !ok {
		return
	}
	rID, ok := pageID(w, r, "rID")
	if !ok {
		return
	}
	_, err := h.workflow.Cancel(r.Context(), user, rID)
	h.redirectResult(w, r, requestHref(connID, rID), "Pengajuan dibatalkan.", err)
}

// Detail eksekusi

type executionDetailPage struct {
	web.Shell
	Title      string
	Crumbs     []web.Crumb
	Actions    []web.Button
	Execution  Execution
	StatusCell web.TableCell
	Detail     web.DetailCard
}

func (h *Handlers) executionDetail(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	eID, ok := pageID(w, r, "eID")
	if !ok {
		return
	}
	e, err := h.workflow.GetExecution(r.Context(), user, connID, eID)
	if err != nil {
		h.pageFail(w, r, err)
		return
	}
	name, ok := h.loadConnName(w, r, connID)
	if !ok {
		return
	}
	dur, rows := "-", "-"
	if e.DurationMS != nil {
		dur = strconv.FormatInt(*e.DurationMS, 10) + " ms"
	}
	if e.RowsAffected != nil {
		rows = formatRows(*e.RowsAffected)
	}
	reverted := "-"
	if e.RevertedBy != nil {
		reverted = "di-rollback oleh " + execLabel(*e.RevertedBy)
	}
	errText := "-"
	if e.Error != "" {
		errText = e.Error
	}
	h.render(w, http.StatusOK, "execution_detail.html", executionDetailPage{
		Shell: h.migrationShell(r.Context(), connID, name),
		Title: fmt.Sprintf("Eksekusi %s · %s", execLabel(e.ID), actionLabel(e)),
		Crumbs: []web.Crumb{
			{Label: name, Href: "/databases"},
			{Label: "Migrasi", Href: migrationsHref(connID)},
			{Label: execLabel(e.ID)},
		},
		Actions: []web.Button{
			{Label: "Pengajuan", Variant: "secondary", Href: requestHref(connID, e.RequestID)},
			{Label: "Migration", Variant: "outline", Href: migrationHref(connID, e.MigrationID)},
		},
		Execution:  e,
		StatusCell: execStatusCell(e.Status),
		Detail: web.DetailCard{
			Title: "Detail Eksekusi",
			Fields: []web.DetailField{
				{Label: "Migration", Value: fmt.Sprintf("v%d · %s", e.Version, e.MigrationName), Note: "revisi " + strconv.Itoa(e.Revision)},
				{Label: "Execution ID", Value: execLabel(e.ID), Note: "attempt " + strconv.Itoa(e.Attempt)},
				{Label: "Pengguna", Value: e.ExecutedByName},
				{Label: "Mulai", Value: e.StartedAt.Format("02 Jan 2006 15:04:05")},
				{Label: "Selesai", Value: timeLabel(e.FinishedAt), Note: dur},
				{Label: "Rows", Value: rows},
				{Label: "Status rollback", Value: reverted},
				{Label: "Galat", Value: errText, Note: strings.TrimSpace(e.ErrorStage + " " + e.SQLState)},
			},
		},
	})
}

// Inbox approval

type approvalsPage struct {
	web.Shell
	Feedback
	Table web.Table
}

// ApprovalRoutes mendaftarkan halaman inbox approval lintas database.
func (h *Handlers) ApprovalRoutes(r chi.Router) {
	r.Get("/approvals", h.approvalsInbox)
}

func (h *Handlers) approvalsInbox(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	items, err := h.workflow.Inbox(r.Context(), user)
	if err != nil {
		h.pageFail(w, r, err)
		return
	}
	t := web.Table{
		Title: "Menunggu Persetujuan Anda",
		Columns: []web.TableColumn{
			{Key: "id", Label: "Pengajuan", Bold: true},
			{Key: "db", Label: "Database"},
			{Key: "mig", Label: "Migration"},
			{Key: "action", Label: "Aksi"},
			{Key: "by", Label: "Diajukan oleh"},
			{Key: "at", Label: "Waktu"},
		},
		Empty: "Tidak ada pengajuan yang menunggu keputusan Anda sebagai leader group.",
	}
	for _, rq := range items {
		t.Rows = append(t.Rows, web.TableRow{
			"id":     {Text: "#" + strconv.FormatInt(rq.ID, 10), Href: requestHref(rq.ConnectionID, rq.ID)},
			"db":     {Text: rq.ConnectionName},
			"mig":    {Text: fmt.Sprintf("v%d · %s", rq.MigrationVersion, rq.MigrationName)},
			"action": {Text: actionText(rq.Action)},
			"by":     {Text: rq.RequestedByName},
			"at":     {Text: rq.CreatedAt.Format("2006-01-02 15:04")},
		})
	}
	h.render(w, http.StatusOK, "approvals.html", approvalsPage{
		Shell:    web.Shell{Nav: "databases", User: shellUser(r.Context())},
		Feedback: feedbackFrom(r),
		Table:    t,
	})
}

// Preview & EXPLAIN (bonus M1) untuk SQL up

type migrationPreviewPage struct {
	web.Shell
	Title  string
	Crumbs []web.Crumb
	SQL    string
	Detail web.DetailCard
	// Analisa rencana eksekusi.
	ExplainAction string
	ExplainErr    string
	ExplainTable  web.Table
	Plans         []ExplainPlanCard
	Analyze       bool
	CanAnalyze    bool
	Actions       []web.Button
}

// ExplainPlanCard = satu blok teks plan statement di halaman preview.
type ExplainPlanCard struct {
	Title string
	Tree  string
}

func (h *Handlers) previewData(r *http.Request, user auth.User, connID int64, m Migration, name string) migrationPreviewPage {
	detailHref := migrationHref(connID, m.ID)
	return migrationPreviewPage{
		Shell: h.migrationShell(r.Context(), connID, name),
		Title: "EXPLAIN Migration",
		Crumbs: []web.Crumb{
			{Label: name, Href: "/databases"},
			{Label: "Migrasi", Href: migrationsHref(connID)},
			{Label: m.Name, Href: detailHref},
			{Label: "EXPLAIN"},
		},
		SQL: m.UpSQL,
		Detail: web.DetailCard{
			Title: "Migration",
			Fields: []web.DetailField{
				{Label: "Posisi", Value: stateLabels[m.State].Text},
				{Label: "Versi", Value: strconv.FormatInt(m.Version, 10)},
				{Label: "Revisi", Value: strconv.Itoa(m.Revision)},
			},
		},
		ExplainAction: detailHref + "/explain",
		CanAnalyze:    h.can(r.Context(), user, connID, true),
		Actions:       []web.Button{{Label: "Kembali", Variant: "secondary", Href: detailHref}},
	}
}

func (h *Handlers) migrationPreview(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	mID, ok := pageID(w, r, "mID")
	if !ok {
		return
	}
	m, err := h.workflow.GetMigration(r.Context(), user, connID, mID)
	if err != nil {
		h.pageFail(w, r, err)
		return
	}
	name, ok := h.loadConnName(w, r, connID)
	if !ok {
		return
	}
	h.render(w, http.StatusOK, "migration_preview.html", h.previewData(r, user, connID, m, name))
}

// migrationExplain menjalankan EXPLAIN untuk seluruh statement SQL up.
func (h *Handlers) migrationExplain(w http.ResponseWriter, r *http.Request) {
	user, ok := pageUser(w, r)
	if !ok {
		return
	}
	analyze := r.PostFormValue("analyze") != ""
	action := access.ActionView
	if analyze {
		action = access.ActionEdit
	}
	connID, ok := h.authorizeAction(w, r, action)
	if !ok {
		return
	}
	mID, ok := pageID(w, r, "mID")
	if !ok {
		return
	}
	m, err := h.workflow.GetMigration(r.Context(), user, connID, mID)
	if err != nil {
		h.pageFail(w, r, err)
		return
	}
	name, _ := h.connectionName(r.Context(), connID)

	page := h.previewData(r, user, connID, m, name)
	page.Analyze = analyze

	pool, err := h.registry.Get(r.Context(), connID)
	if err != nil || pool == nil {
		page.ExplainErr = "Database tidak dapat dihubungi; analisa tidak dijalankan."
		h.render(w, http.StatusOK, "migration_preview.html", page)
		return
	}

	items := ExplainAll(r.Context(), pool, m.UpSQL, analyze)
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
