package async

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/dbm"
)

const maxJSONBodyBytes = 1 << 20 // 1 MiB

// HandlerConfig mengumpulkan dependency untuk HTTP API asynchronous migration.
type HandlerConfig struct {
	Store      Store
	Migrations dbm.MigrationStore
	Policy     access.Policy
	Approval   ApprovalChecker
	Tracer     TracePropagator
	Metrics    MetricsObserver
}

// Handlers melayani endpoint JSON untuk enqueue, resume, cancel, dan monitoring job.
type Handlers struct {
	store      Store
	migrations dbm.MigrationStore
	policy     access.Policy
	approval   ApprovalChecker
	tracer     TracePropagator
	metrics    MetricsObserver
}

func NewHandlers(cfg HandlerConfig) *Handlers {
	policy := cfg.Policy
	if policy == nil {
		policy = access.DenyAll{}
	}
	approval := cfg.Approval
	if approval == nil {
		approval = AllowAllApproval{}
	}
	tracer := cfg.Tracer
	if tracer == nil {
		tracer = NoopTracePropagator{}
	}
	metrics := cfg.Metrics
	if metrics == nil {
		metrics = NoopMetricsObserver{}
	}
	return &Handlers{
		store:      cfg.Store,
		migrations: cfg.Migrations,
		policy:     policy,
		approval:   approval,
		tracer:     tracer,
		metrics:    metrics,
	}
}

// Routes mendaftarkan endpoint /api/* untuk asynchronous migration.
func (h *Handlers) Routes(r chi.Router) {
	r.Route("/api", func(r chi.Router) {
		r.Post("/databases/{connID}/migrations/{mID}/execute", h.executeMigration)
		r.Get("/databases/{connID}/jobs", h.listDatabaseJobs)
		r.Get("/jobs/{jobID}", h.getJob)
		r.Post("/jobs/{jobID}/resume", h.resumeJob)
		r.Post("/jobs/{jobID}/cancel", h.cancelJob)
	})
}

type executeRequest struct {
	Action         Action `json:"action"`
	Mode           Mode   `json:"mode"`
	SQLDown        string `json:"sql_down"`
	Irreversible   bool   `json:"irreversible"`
	BatchTable     string `json:"batch_table"`
	BatchColumn    string `json:"batch_column"`
	BatchSize      int    `json:"batch_size"`
	MaxRetries     *int   `json:"max_retries"`
	IdempotencyKey string `json:"idempotency_key"`
}

type actionRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type jobResponse struct {
	JobID            int64        `json:"job_id"`
	MigrationID      int64        `json:"migration_id"`
	ExecutionID      *int64       `json:"execution_id,omitempty"`
	ConnectionID     int64        `json:"connection_id"`
	Action           Action       `json:"action"`
	Mode             Mode         `json:"mode"`
	Status           Status       `json:"status"`
	BatchTable       string       `json:"batch_table,omitempty"`
	BatchColumn      string       `json:"batch_column,omitempty"`
	BatchSize        int          `json:"batch_size,omitempty"`
	CursorMin        string       `json:"cursor_min,omitempty"`
	CursorMax        string       `json:"cursor_max,omitempty"`
	LastCursor       string       `json:"last_cursor,omitempty"`
	CommittedBatches int          `json:"committed_batches"`
	CommittedRows    int64        `json:"committed_rows"`
	Attempt          int          `json:"attempt"`
	MaxRetries       int          `json:"max_retries"`
	ParentJobID      *int64       `json:"parent_job_id,omitempty"`
	Error            string       `json:"error,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
	StartedAt        *time.Time   `json:"started_at,omitempty"`
	FinishedAt       *time.Time   `json:"finished_at,omitempty"`
	Checkpoints      []Checkpoint `json:"checkpoints,omitempty"`
}

func toJobResponse(j Job, cps []Checkpoint) jobResponse {
	return jobResponse{
		JobID:            j.ID,
		MigrationID:      j.MigrationID,
		ExecutionID:      j.ExecutionID,
		ConnectionID:     j.ConnectionID,
		Action:           j.Action,
		Mode:             j.Mode,
		Status:           j.Status,
		BatchTable:       j.BatchTable,
		BatchColumn:      j.BatchColumn,
		BatchSize:        j.BatchSize,
		CursorMin:        j.CursorMin,
		CursorMax:        j.CursorMax,
		LastCursor:       j.LastCursor,
		CommittedBatches: j.CommittedBatches,
		CommittedRows:    j.CommittedRows,
		Attempt:          j.Attempt,
		MaxRetries:       j.MaxRetries,
		ParentJobID:      j.ParentJobID,
		Error:            j.Error,
		CreatedAt:        j.CreatedAt,
		StartedAt:        j.StartedAt,
		FinishedAt:       j.FinishedAt,
		Checkpoints:      cps,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

func (h *Handlers) authorize(w http.ResponseWriter, r *http.Request, connID int64, action access.Action) (auth.User, bool) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusForbidden, "akses ditolak")
		return auth.User{}, false
	}
	allowed, err := h.policy.Can(r.Context(), user.ID, connID, action)
	if err != nil || !allowed {
		writeJSONError(w, http.StatusForbidden, "akses ditolak")
		return auth.User{}, false
	}
	return user, true
}

func parsePositiveID(r *http.Request, param string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, param), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (h *Handlers) executeMigration(w http.ResponseWriter, r *http.Request) {
	connID, ok := parsePositiveID(r, "connID")
	if !ok {
		writeJSONError(w, http.StatusNotFound, "koneksi tidak ditemukan")
		return
	}
	mID, ok := parsePositiveID(r, "mID")
	if !ok {
		writeJSONError(w, http.StatusNotFound, "migration tidak ditemukan")
		return
	}

	user, ok := h.authorize(w, r, connID, access.ActionEdit)
	if !ok {
		return
	}

	var req executeRequest
	if err := decodeOptionalJSON(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "format JSON tidak valid")
		return
	}

	if req.Action == "" {
		req.Action = ActionUp
	}
	switch req.Action {
	case ActionUp, ActionDown, ActionForward:
	default:
		writeJSONError(w, http.StatusBadRequest, "action tidak valid")
		return
	}

	if req.Mode == "" {
		req.Mode = ModeNormal
	}
	if req.Mode != ModeNormal && req.Mode != ModeBatch {
		writeJSONError(w, http.StatusBadRequest, "mode eksekusi harus normal atau batch")
		return
	}

	if h.migrations == nil || h.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "layanan async belum siap")
		return
	}

	m, err := h.migrations.Get(r.Context(), mID)
	if errors.Is(err, dbm.ErrMigrationNotFound) || (err == nil && m.ConnectionID != connID) {
		writeJSONError(w, http.StatusNotFound, "migration tidak ditemukan")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "gagal memuat migration")
		return
	}

	if err := h.approval.EnsureApproved(r.Context(), connID, mID, req.Action); err != nil {
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	}

	// Tolak enqueue baru jika database masih memiliki migration partial / partially_reverted.
	blocked, err := h.store.HasBlockingJob(r.Context(), connID, 0)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "gagal memeriksa status database")
		return
	}
	if blocked {
		writeJSONError(w, http.StatusConflict, ErrDatabaseBlocked.Error())
		return
	}

	if req.Mode == ModeBatch {
		if _, _, err := validateBatchIdentifiers(req.BatchTable, req.BatchColumn); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.BatchSize <= 0 {
			writeJSONError(w, http.StatusBadRequest, "batch_size harus lebih besar dari 0")
			return
		}
		if err := validateBatchSQL(m.SQL); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	maxRetries := 3
	if req.MaxRetries != nil && *req.MaxRetries >= 0 {
		maxRetries = *req.MaxRetries
	}

	userID := user.ID
	job, err := h.store.Enqueue(r.Context(), EnqueueInput{
		MigrationID:    m.ID,
		ConnectionID:   connID,
		Action:         req.Action,
		Mode:           req.Mode,
		SQLUp:          m.SQL,
		SQLDown:        req.SQLDown,
		Irreversible:   req.Irreversible,
		BatchTable:     req.BatchTable,
		BatchColumn:    req.BatchColumn,
		BatchSize:      req.BatchSize,
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
		MaxRetries:     maxRetries,
		TraceContext:   h.tracer.Inject(r.Context()),
		CreatedBy:      &userID,
	})
	if errors.Is(err, ErrDuplicateJob) {
		// Idempotent: kembalikan job yang sudah ada tanpa membuat duplikat.
		writeJSON(w, http.StatusAccepted, toJobResponse(job, nil))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "gagal memasukkan job ke queue")
		return
	}

	h.metrics.OnJobEnqueued(job.Mode, job.Action)
	writeJSON(w, http.StatusAccepted, toJobResponse(job, nil))
}

func (h *Handlers) resumeJob(w http.ResponseWriter, r *http.Request) {
	jobID, ok := parsePositiveID(r, "jobID")
	if !ok {
		writeJSONError(w, http.StatusNotFound, "job tidak ditemukan")
		return
	}
	if h.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "layanan async belum siap")
		return
	}

	orig, err := h.store.GetJob(r.Context(), jobID)
	if errors.Is(err, ErrJobNotFound) {
		writeJSONError(w, http.StatusNotFound, ErrJobNotFound.Error())
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "gagal memuat job")
		return
	}

	user, ok := h.authorize(w, r, orig.ConnectionID, access.ActionEdit)
	if !ok {
		return
	}

	if orig.Mode != ModeBatch || (orig.Status != StatusPartial && orig.Status != StatusPartiallyReverted) {
		writeJSONError(w, http.StatusConflict, ErrInvalidJobState.Error())
		return
	}

	var req actionRequest
	if err := decodeOptionalJSON(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "format JSON tidak valid")
		return
	}

	nextAction := ActionResume
	if orig.Status == StatusPartiallyReverted {
		nextAction = ActionCancelBatch
	}

	if err := h.approval.EnsureApproved(r.Context(), orig.ConnectionID, orig.MigrationID, nextAction); err != nil {
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	}

	parentID := orig.ID
	if orig.ParentJobID != nil {
		parentID = *orig.ParentJobID
	}
	userID := user.ID

	newJob, err := h.store.Enqueue(r.Context(), EnqueueInput{
		MigrationID:    orig.MigrationID,
		ExecutionID:    orig.ExecutionID,
		ConnectionID:   orig.ConnectionID,
		Action:         nextAction,
		Mode:           ModeBatch,
		SQLUp:          orig.SQLUp,
		SQLDown:        orig.SQLDown,
		Irreversible:   orig.Irreversible,
		BatchTable:     orig.BatchTable,
		BatchColumn:    orig.BatchColumn,
		BatchSize:      orig.BatchSize,
		CursorMin:      orig.CursorMin,
		CursorMax:      orig.CursorMax,
		LastCursor:     orig.LastCursor,
		ParentJobID:    &parentID,
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
		MaxRetries:     orig.MaxRetries,
		TraceContext:   h.tracer.Inject(r.Context()),
		CreatedBy:      &userID,
	})
	if errors.Is(err, ErrDuplicateJob) {
		writeJSON(w, http.StatusAccepted, toJobResponse(newJob, nil))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "gagal menjadwalkan resume job")
		return
	}

	h.metrics.OnJobEnqueued(newJob.Mode, newJob.Action)
	writeJSON(w, http.StatusAccepted, toJobResponse(newJob, nil))
}

func (h *Handlers) cancelJob(w http.ResponseWriter, r *http.Request) {
	jobID, ok := parsePositiveID(r, "jobID")
	if !ok {
		writeJSONError(w, http.StatusNotFound, "job tidak ditemukan")
		return
	}
	if h.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "layanan async belum siap")
		return
	}

	orig, err := h.store.GetJob(r.Context(), jobID)
	if errors.Is(err, ErrJobNotFound) {
		writeJSONError(w, http.StatusNotFound, ErrJobNotFound.Error())
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "gagal memuat job")
		return
	}

	user, ok := h.authorize(w, r, orig.ConnectionID, access.ActionEdit)
	if !ok {
		return
	}

	if orig.Mode != ModeBatch || (orig.Status != StatusPartial && orig.Status != StatusPartiallyReverted && orig.Status != StatusSuccess) {
		writeJSONError(w, http.StatusConflict, ErrInvalidJobState.Error())
		return
	}
	if orig.Irreversible || strings.TrimSpace(orig.SQLDown) == "" {
		writeJSONError(w, http.StatusConflict, ErrUnsafeCancel.Error())
		return
	}
	if err := validateBatchSQL(orig.SQLDown); err != nil {
		writeJSONError(w, http.StatusConflict, ErrUnsafeCancel.Error())
		return
	}

	var req actionRequest
	if err := decodeOptionalJSON(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "format JSON tidak valid")
		return
	}

	if err := h.approval.EnsureApproved(r.Context(), orig.ConnectionID, orig.MigrationID, ActionCancelBatch); err != nil {
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	}

	parentID := orig.ID
	if orig.ParentJobID != nil {
		parentID = *orig.ParentJobID
	}
	userID := user.ID

	newJob, err := h.store.Enqueue(r.Context(), EnqueueInput{
		MigrationID:    orig.MigrationID,
		ExecutionID:    orig.ExecutionID,
		ConnectionID:   orig.ConnectionID,
		Action:         ActionCancelBatch,
		Mode:           ModeBatch,
		SQLUp:          orig.SQLUp,
		SQLDown:        orig.SQLDown,
		Irreversible:   orig.Irreversible,
		BatchTable:     orig.BatchTable,
		BatchColumn:    orig.BatchColumn,
		BatchSize:      orig.BatchSize,
		CursorMin:      orig.CursorMin,
		CursorMax:      orig.CursorMax,
		LastCursor:     orig.LastCursor,
		ParentJobID:    &parentID,
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
		MaxRetries:     orig.MaxRetries,
		TraceContext:   h.tracer.Inject(r.Context()),
		CreatedBy:      &userID,
	})
	if errors.Is(err, ErrDuplicateJob) {
		writeJSON(w, http.StatusAccepted, toJobResponse(newJob, nil))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "gagal menjadwalkan pembatalan batch")
		return
	}

	h.metrics.OnJobEnqueued(newJob.Mode, newJob.Action)
	writeJSON(w, http.StatusAccepted, toJobResponse(newJob, nil))
}

func (h *Handlers) getJob(w http.ResponseWriter, r *http.Request) {
	jobID, ok := parsePositiveID(r, "jobID")
	if !ok {
		writeJSONError(w, http.StatusNotFound, "job tidak ditemukan")
		return
	}
	if h.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "layanan async belum siap")
		return
	}

	job, err := h.store.GetJob(r.Context(), jobID)
	if errors.Is(err, ErrJobNotFound) {
		writeJSONError(w, http.StatusNotFound, ErrJobNotFound.Error())
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "gagal memuat job")
		return
	}

	if _, ok := h.authorize(w, r, job.ConnectionID, access.ActionView); !ok {
		return
	}

	rootID := job.ID
	if job.ParentJobID != nil {
		rootID = *job.ParentJobID
	}
	var cps []Checkpoint
	if job.Mode == ModeBatch {
		cps, _ = h.store.ListCheckpoints(r.Context(), rootID)
	}

	writeJSON(w, http.StatusOK, toJobResponse(job, cps))
}

func (h *Handlers) listDatabaseJobs(w http.ResponseWriter, r *http.Request) {
	connID, ok := parsePositiveID(r, "connID")
	if !ok {
		writeJSONError(w, http.StatusNotFound, "koneksi tidak ditemukan")
		return
	}
	if _, ok := h.authorize(w, r, connID, access.ActionView); !ok {
		return
	}
	if h.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "layanan async belum siap")
		return
	}

	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	jobs, err := h.store.ListJobsByConnection(r.Context(), connID, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "gagal memuat daftar job")
		return
	}

	out := make([]jobResponse, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, toJobResponse(j, nil))
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": out})
}
