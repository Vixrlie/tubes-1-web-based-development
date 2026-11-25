package dbm

// JSON API workflow migration untuk SPA. Kontrak: docs/api-migration-workflow.md.

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

// APIRoutes mendaftarkan route /api/... workflow migration.
func (h *Handlers) APIRoutes(r chi.Router) {
	r.Route("/api", func(r chi.Router) {
		r.Get("/me", h.apiMe)
		r.Get("/approvals", h.apiInbox)
		h.databaseAPIRoutes(r)
		r.Route("/databases/{connID}", func(r chi.Router) {
			h.databaseScopedRoutes(r)
			r.Get("/migrations", h.apiListMigrations)
			r.Post("/migrations", h.apiCreateMigration)
			r.Get("/migrations/{mID}", h.apiGetMigration)
			r.Put("/migrations/{mID}", h.apiUpdateMigration)
			r.Post("/migrations/{mID}/validate", h.apiValidate)
			r.Post("/migrations/{mID}/requests", h.apiSubmit)
			r.Get("/requests", h.apiListRequests)
			r.Get("/history", h.apiHistory)
			r.Get("/workflow", h.apiBoard)
			r.Get("/executions/{eID}", h.apiGetExecution)
		})
		r.Route("/migration-requests/{rID}", func(r chi.Router) {
			r.Get("/", h.apiGetRequest)
			r.Post("/approve", h.apiApprove)
			r.Post("/reject", h.apiReject)
			r.Post("/cancel", h.apiCancel)
		})
	})
}

// apiMe memberi identitas user dan token CSRF untuk header X-CSRF-Token
// pada request POST/PUT dari SPA.
func (h *Handlers) apiMe(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "unauthenticated", "sesi tidak valid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         user.ID,
		"username":   user.Username,
		"is_admin":   user.IsPlatformAdmin,
		"csrf_token": session.CSRFToken(r.Context()),
	})
}

type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiError{Error: message, Code: code})
}

// apiStatus memetakan galat domain ke status HTTP dan kode stabil untuk FE.
func apiStatus(err error) (int, string) {
	switch {
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, ErrNotReviewer):
		return http.StatusForbidden, "not_reviewer"
	case errors.Is(err, ErrMigrationNotFound), errors.Is(err, ErrRequestNotFound),
		errors.Is(err, ErrExecutionNotFound), errors.Is(err, ErrConnNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, ErrEmptySQL), errors.Is(err, ErrMissingDown), errors.Is(err, ErrMissingReason),
		errors.Is(err, ErrIrreversibleDown), errors.Is(err, ErrDownSameAsUp), errors.Is(err, ErrNameTooLong),
		errors.Is(err, ErrRejectNoteRequired), errors.Is(err, ErrUnknownAction),
		errors.Is(err, ErrBatchConfig), errors.Is(err, ErrBatchPlaceholders), errors.Is(err, ErrBatchIdentifier):
		return http.StatusUnprocessableEntity, "invalid_input"
	case errors.Is(err, ErrPartialPending), errors.Is(err, ErrNotPartial):
		return http.StatusConflict, "partial_pending"
	case errors.Is(err, ErrDatabasePartial):
		return http.StatusConflict, "database_held"
	case errors.Is(err, ErrValidationRequired):
		return http.StatusConflict, "validation_required"
	case errors.Is(err, ErrNotReversible):
		return http.StatusConflict, "irreversible"
	case errors.Is(err, ErrNotActive), errors.Is(err, ErrNotLatestActive):
		return http.StatusConflict, "rollback_not_allowed"
	case errors.Is(err, ErrAlreadyActive), errors.Is(err, ErrForwardOrder):
		return http.StatusConflict, "forward_not_allowed"
	case errors.Is(err, ErrMigrationLocked):
		return http.StatusConflict, "immutable"
	case errors.Is(err, ErrMigrationInReview), errors.Is(err, ErrRequestExists):
		return http.StatusConflict, "request_open"
	case errors.Is(err, ErrRequestNotPending), errors.Is(err, ErrCannotCancel):
		return http.StatusConflict, "invalid_state"
	case errors.Is(err, ErrChecksumMismatch):
		return http.StatusConflict, "checksum_mismatch"
	case errors.Is(err, ErrVersionConflict):
		return http.StatusConflict, "version_conflict"
	case errors.Is(err, ErrNoQueue):
		return http.StatusServiceUnavailable, "queue_unavailable"
	}
	return http.StatusInternalServerError, "internal"
}

func (h *Handlers) apiFail(w http.ResponseWriter, r *http.Request, err error) {
	status, code := apiStatus(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		safeMethod := strings.NewReplacer("\r", "", "\n", "").Replace(r.Method)
		safePath := strings.NewReplacer("\r", "", "\n", "").Replace(r.URL.Path)
		log.Printf("dbm api %s %s: %v", safeMethod, safePath, err)
		msg = "terjadi kesalahan pada server"
	}
	writeAPIError(w, status, code, msg)
}

// apiActor mengambil user login dan ID path. false = respons sudah dikirim.
func apiActor(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "unauthenticated", "sesi tidak valid")
		return auth.User{}, false
	}
	return user, true
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || v <= 0 {
		writeAPIError(w, http.StatusNotFound, "not_found", "ID tidak valid")
		return 0, false
	}
	return v, true
}

// decodeBody membaca JSON maksimal 1 MiB; body kosong dianggap {}.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "bad_json", "body JSON tidak valid: "+err.Error())
		return false
	}
	return true
}

func (h *Handlers) apiListMigrations(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return
	}
	items, err := h.workflow.ListMigrations(r.Context(), user, connID)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	// Daftar hanya ringkasan; isi SQL diambil lewat GET /migrations/{id}.
	for i := range items {
		items[i].UpSQL, items[i].DownSQL = "", ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) apiCreateMigration(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return
	}
	var in MigrationInput
	if !decodeBody(w, r, &in) {
		return
	}
	m, err := h.workflow.CreateMigration(r.Context(), user, connID, in)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// migrationDetail = respons GET satu migration.
type migrationDetail struct {
	Migration  Migration   `json:"migration"`
	Requests   []Request   `json:"requests"`
	Executions []Execution `json:"executions"`
	// Aksi yang saat ini sah (mengikuti aturan linear); FE menampilkan tombolnya.
	AllowedActions []string `json:"allowed_actions"`
	CanEdit        bool     `json:"can_edit"`
}

func (h *Handlers) apiGetMigration(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return
	}
	mID, ok := pathID(w, r, "mID")
	if !ok {
		return
	}
	detail, err := h.loadMigrationDetail(r, user, connID, mID)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handlers) loadMigrationDetail(r *http.Request, user auth.User, connID, mID int64) (migrationDetail, error) {
	ctx := r.Context()
	m, err := h.workflow.GetMigration(ctx, user, connID, mID)
	if err != nil {
		return migrationDetail{}, err
	}
	requests, err := h.workflow.MigrationRequests(ctx, user, connID, mID)
	if err != nil {
		return migrationDetail{}, err
	}
	execs, err := h.workflow.MigrationExecutions(ctx, user, connID, mID)
	if err != nil {
		return migrationDetail{}, err
	}
	canEdit := h.can(ctx, user, connID, true)
	actions := []string{}
	if canEdit && m.OpenRequest == nil {
		actions = h.workflow.AllowedActions(ctx, connID, m)
	}
	return migrationDetail{Migration: m, Requests: requests, Executions: execs, AllowedActions: actions, CanEdit: canEdit}, nil
}

func (h *Handlers) apiUpdateMigration(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return
	}
	mID, ok := pathID(w, r, "mID")
	if !ok {
		return
	}
	var in MigrationInput
	if !decodeBody(w, r, &in) {
		return
	}
	m, err := h.workflow.UpdateMigration(r.Context(), user, connID, mID, in)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handlers) apiValidate(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return
	}
	mID, ok := pathID(w, r, "mID")
	if !ok {
		return
	}
	res, err := h.workflow.Validate(r.Context(), user, connID, mID)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type submitBody struct {
	Action string `json:"action"`
	Note   string `json:"note"`
}

func (h *Handlers) apiSubmit(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return
	}
	mID, ok := pathID(w, r, "mID")
	if !ok {
		return
	}
	var body submitBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Action == "" {
		body.Action = ActionUp
	}
	req, err := h.workflow.Submit(r.Context(), user, connID, mID, body.Action, body.Note)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func splitCSV(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (h *Handlers) apiListRequests(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return
	}
	items, err := h.workflow.ListRequests(r.Context(), user, connID, splitCSV(r.URL.Query().Get("status")))
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) apiHistory(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return
	}
	hide := parseBool(r.URL.Query().Get("hide_rollback"))
	items, err := h.workflow.History(r.Context(), user, connID, hide)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	// SQL yang dijalankan tersedia di GET /executions/{id}.
	for i := range items {
		items[i].SQL = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "hide_rollback": hide})
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func (h *Handlers) apiGetExecution(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return
	}
	eID, ok := pathID(w, r, "eID")
	if !ok {
		return
	}
	e, err := h.workflow.GetExecution(r.Context(), user, connID, eID)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *Handlers) apiGetRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	rID, ok := pathID(w, r, "rID")
	if !ok {
		return
	}
	req, err := h.workflow.GetRequest(r.Context(), user, rID)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

type noteBody struct {
	Note string `json:"note"`
}

func (h *Handlers) apiApprove(w http.ResponseWriter, r *http.Request) {
	h.apiDecide(w, r, true)
}

func (h *Handlers) apiReject(w http.ResponseWriter, r *http.Request) {
	h.apiDecide(w, r, false)
}

func (h *Handlers) apiDecide(w http.ResponseWriter, r *http.Request, approve bool) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	rID, ok := pathID(w, r, "rID")
	if !ok {
		return
	}
	var body noteBody
	if !decodeBody(w, r, &body) {
		return
	}
	var res Decision
	var err error
	if approve {
		res, err = h.workflow.Approve(r.Context(), user, rID, body.Note)
	} else {
		res, err = h.workflow.Reject(r.Context(), user, rID, body.Note)
	}
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	status := http.StatusOK
	if res.JobID != 0 {
		// Eksekusi dipublikasikan ke queue; hasilnya dipantau lewat GET request.
		status = http.StatusAccepted
	}
	writeJSON(w, status, res)
}

func (h *Handlers) apiCancel(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	rID, ok := pathID(w, r, "rID")
	if !ok {
		return
	}
	req, err := h.workflow.Cancel(r.Context(), user, rID)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (h *Handlers) apiInbox(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	items, err := h.workflow.Inbox(r.Context(), user)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
