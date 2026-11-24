package dbm

// Workflow migration: Draft → Pending Review → Approved → Queued → Running →
// Success/Failed, sama untuk up, rollback, dan forward.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

var (
	ErrForbidden          = errors.New("akses ditolak")
	ErrNotReviewer        = errors.New("anda bukan leader dari group yang masih menunggu keputusan untuk pengajuan ini")
	ErrRequestNotPending  = errors.New("pengajuan tidak lagi menunggu review")
	ErrRequestExists      = errors.New("migration ini masih memiliki pengajuan yang berjalan")
	ErrRejectNoteRequired = errors.New("alasan penolakan wajib diisi")
	ErrCannotCancel       = errors.New("pengajuan hanya dapat dibatalkan sebelum masuk antrian")
	ErrNoQueue            = errors.New("queue eksekusi belum dikonfigurasi")
	ErrTargetBusy         = errors.New("database target sedang menjalankan migration; coba validasi lagi nanti")
)

// Enqueuer menyisipkan job di tx yang sama dengan approval, jadi keduanya
// tidak pernah terpisah.
type Enqueuer interface {
	Enqueue(ctx context.Context, tx pgx.Tx, requestID, connID int64) (jobID int64, err error)
}

// workflowLockKey = namespace advisory lock platform untuk menserialkan
// operasi workflow (submit, approve, revalidasi worker) per database target.
const workflowLockKey = 7101

func lockConnection(ctx context.Context, tx pgx.Tx, connID int64) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1::int, $2::int)`, workflowLockKey, connID)
	return err
}

// Workflow menjalankan operasi tulis migration dengan pemeriksaan izin.
type Workflow struct {
	pool     *pgxpool.Pool
	policy   access.Policy
	store    MigrationStore
	registry Registry
	queue    Enqueuer
	logger   *activity.Logger
	// ValidationTimeout membatasi dry-run validasi di database target.
	ValidationTimeout time.Duration
}

type WorkflowConfig struct {
	Pool     *pgxpool.Pool
	Policy   access.Policy
	Store    MigrationStore
	Registry Registry
	Queue    Enqueuer
	Logger   *activity.Logger
}

func NewWorkflow(cfg WorkflowConfig) *Workflow {
	store := cfg.Store
	if store == nil {
		store = NewPGMigrationStore(cfg.Pool)
	}
	policy := cfg.Policy
	if policy == nil {
		policy = access.DenyAll{}
	}
	registry := cfg.Registry
	if registry == nil {
		registry = StubRegistry{}
	}
	return &Workflow{
		pool:              cfg.Pool,
		policy:            policy,
		store:             store,
		registry:          registry,
		queue:             cfg.Queue,
		logger:            cfg.Logger,
		ValidationTimeout: 15 * time.Second,
	}
}

func (w *Workflow) can(ctx context.Context, actor auth.User, connID int64, action access.Action) error {
	ok, err := w.policy.Can(ctx, actor.ID, connID, action)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

func (w *Workflow) log(ctx context.Context, actor auth.User, action, status string, detail map[string]string, err error) {
	w.logger.Log(ctx, activity.Event{
		UserID: strconv.FormatInt(actor.ID, 10),
		Action: action,
		Status: status,
		Detail: detail,
		Err:    err,
	})
}

func idStr(n int64) string { return strconv.FormatInt(n, 10) }

// Definisi

func (w *Workflow) ListMigrations(ctx context.Context, actor auth.User, connID int64) ([]Migration, error) {
	if err := w.can(ctx, actor, connID, access.ActionView); err != nil {
		return nil, err
	}
	return w.store.List(ctx, connID)
}

// GetMigration memuat definisi dan memastikan milik connID.
func (w *Workflow) GetMigration(ctx context.Context, actor auth.User, connID, migID int64) (Migration, error) {
	if err := w.can(ctx, actor, connID, access.ActionView); err != nil {
		return Migration{}, err
	}
	m, err := w.store.Get(ctx, migID)
	if err != nil {
		return Migration{}, err
	}
	if m.ConnectionID != connID {
		return Migration{}, ErrMigrationNotFound
	}
	return m, nil
}

func (w *Workflow) CreateMigration(ctx context.Context, actor auth.User, connID int64, in MigrationInput) (Migration, error) {
	if err := w.can(ctx, actor, connID, access.ActionEdit); err != nil {
		return Migration{}, err
	}
	m, err := w.store.Create(ctx, connID, in, actor.ID)
	status := activity.StatusSuccess
	if err != nil {
		status = activity.StatusFailed
	}
	w.log(ctx, actor, "migration.create", status, map[string]string{
		"database_id": idStr(connID), "migration_id": idStr(m.ID), "revision": strconv.Itoa(m.Revision),
	}, err)
	return m, err
}

func (w *Workflow) UpdateMigration(ctx context.Context, actor auth.User, connID, migID int64, in MigrationInput) (Migration, error) {
	if _, err := w.GetMigration(ctx, actor, connID, migID); err != nil {
		return Migration{}, err
	}
	if err := w.can(ctx, actor, connID, access.ActionEdit); err != nil {
		return Migration{}, err
	}
	m, err := w.store.Update(ctx, migID, in)
	status := activity.StatusSuccess
	if err != nil {
		status = activity.StatusFailed
	}
	w.log(ctx, actor, "migration.update", status, map[string]string{
		"database_id": idStr(connID), "migration_id": idStr(migID), "revision": strconv.Itoa(m.Revision),
	}, err)
	return m, err
}

// ValidationResult = hasil validasi statis dan dry-run.
type ValidationResult struct {
	OK         bool   `json:"ok"`
	Message    string `json:"message"`
	Phase      string `json:"phase,omitempty"` // static | up | down
	SQLState   string `json:"sqlstate,omitempty"`
	DryRun     bool   `json:"dry_run"`
	DurationMS int64  `json:"duration_ms"`
}

// Validate menjalankan dry-run dalam transaksi yang selalu di-ROLLBACK.
// MySQL dilewati karena DDL-nya tidak transaksional.
func (w *Workflow) Validate(ctx context.Context, actor auth.User, connID, migID int64) (ValidationResult, error) {
	m, err := w.GetMigration(ctx, actor, connID, migID)
	if err != nil {
		return ValidationResult{}, err
	}
	if err := w.can(ctx, actor, connID, access.ActionEdit); err != nil {
		return ValidationResult{}, err
	}
	ctx, span := activity.StartSpan(ctx, "migration.validate")
	defer span.End()

	start := time.Now()
	res := w.dryRun(ctx, connID, m)
	res.DurationMS = time.Since(start).Milliseconds()
	if errors.Is(ctx.Err(), context.Canceled) {
		return res, ctx.Err()
	}
	if err := w.store.SaveValidation(ctx, m.ID, m.Checksum, res.OK, res.Message); err != nil {
		return res, err
	}
	status := activity.StatusSuccess
	if !res.OK {
		status = activity.StatusWarning
	}
	w.log(ctx, actor, "migration.validate", status, map[string]string{
		"database_id": idStr(connID), "migration_id": idStr(m.ID), "revision": strconv.Itoa(m.Revision),
		"phase": res.Phase, "sqlstate": res.SQLState, "dry_run": strconv.FormatBool(res.DryRun),
		"duration_ms": strconv.FormatInt(res.DurationMS, 10),
	}, nil)
	return res, nil
}

// validationLockWait = batas tunggu lock database sebelum dry-run dinyatakan sibuk.
const validationLockWait = 5 * time.Second

func (w *Workflow) dryRun(ctx context.Context, connID int64, m Migration) ValidationResult {
	if _, err := m.Input().Normalize(); err != nil {
		return ValidationResult{Message: err.Error(), Phase: "static"}
	}

	target, err := w.registry.Get(ctx, connID)
	if err != nil || target == nil {
		return ValidationResult{Message: "database target tidak dapat dihubungi; dry-run tidak dijalankan", Phase: "static"}
	}
	if target.Dialect.Name() != "postgres" {
		return ValidationResult{
			OK:      true,
			Message: "Validasi statis lolos. Dry-run dilewati karena " + target.Dialect.Name() + " tidak mendukung DDL transaksional.",
			Phase:   "static",
		}
	}

	type step struct{ phase, sql string }
	steps := []step{{"up", m.UpSQL}}
	if m.Reversible() {
		steps = append(steps, step{"down", m.DownSQL})
	}
	if m.State == StateActive {
		// Perubahan sudah ada di target: uji pembatalannya lalu penerapan ulang.
		steps = []step{{"up", m.UpSQL}}
		if m.Reversible() {
			steps = []step{{"down", m.DownSQL}, {"up", m.UpSQL}}
		}
	}

	timeout := w.ValidationTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()

	conn, err := target.DB.Conn(runCtx)
	if err != nil {
		return ValidationResult{Message: "gagal membuka koneksi target: " + sanitizePGError(err), Phase: "static"}
	}
	defer conn.Close()
	tx, err := conn.BeginTx(runCtx, nil)
	if err != nil {
		return ValidationResult{Message: "gagal memulai transaksi dry-run: " + sanitizePGError(err), Phase: "static"}
	}
	defer tx.Rollback() //nolint:errcheck // dry-run selalu dibatalkan

	// Dry-run diserialisasi dengan eksekusi dan dry-run lain pada database yang
	// sama; tunggu sebentar alih-alih langsung menolak saat lock sedang dipakai.
	if _, err := tx.ExecContext(runCtx, fmt.Sprintf("SET LOCAL lock_timeout = %d", validationLockWait.Milliseconds())); err != nil {
		return ValidationResult{Message: sanitizePGError(err), Phase: "static"}
	}
	if _, err := tx.ExecContext(runCtx, `SELECT pg_advisory_xact_lock($1)`, connID); err != nil {
		if sqlState(err) == "55P03" {
			return ValidationResult{Message: ErrTargetBusy.Error(), Phase: "static"}
		}
		return ValidationResult{Message: sanitizePGError(err), Phase: "static"}
	}
	if _, err := tx.ExecContext(runCtx, "SET LOCAL lock_timeout TO DEFAULT"); err != nil {
		return ValidationResult{Message: sanitizePGError(err), Phase: "static"}
	}
	if _, err := tx.ExecContext(runCtx, fmt.Sprintf("SET LOCAL statement_timeout = %d", timeout.Milliseconds())); err != nil {
		return ValidationResult{Message: sanitizePGError(err), Phase: "static"}
	}
	// Mode batch: dry-run memakai rentang batch pertama sebagai nilai $1/$2.
	var args []any
	if m.Batch.Enabled() {
		run := BatchRun{Dialect: target.Dialect, Config: *m.Batch}
		lo, hi, n, err := run.FirstWindow(runCtx, tx)
		if err != nil {
			return ValidationResult{Phase: "static", SQLState: sqlState(err),
				Message: "tabel atau kolom cursor batch tidak dapat dibaca: " + sanitizePGError(err)}
		}
		if n == 0 {
			return ValidationResult{OK: true, Phase: "static",
				Message: "Tabel batch kosong; tidak ada batch untuk diuji. Validasi statis lolos."}
		}
		args = []any{lo, hi}
	}
	for _, s := range steps {
		if _, err := tx.ExecContext(runCtx, s.sql, args...); err != nil {
			res := ValidationResult{
				Phase:    s.phase,
				DryRun:   true,
				SQLState: sqlState(err),
				Message:  "SQL " + s.phase + " gagal pada dry-run: " + sanitizePGError(err),
			}
			if res.SQLState == "57014" {
				res.Message = fmt.Sprintf("SQL %s melebihi batas waktu validasi (%s); pertimbangkan mode per batch.", s.phase, timeout)
			}
			return res
		}
	}
	phases := make([]string, len(steps))
	for i, s := range steps {
		phases[i] = "SQL " + s.phase
	}
	message := "Dry-run berhasil (" + strings.Join(phases, " → ") + ") dan seluruh perubahan sudah di-rollback."
	if m.Batch.Enabled() {
		message = fmt.Sprintf("Dry-run batch pertama (cursor %v sampai %v) berhasil (%s) dan sudah di-rollback.",
			args[0], args[1], strings.Join(phases, " → "))
	}
	return ValidationResult{
		OK:      true,
		DryRun:  true,
		Phase:   steps[len(steps)-1].phase,
		Message: message,
	}
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// Pengajuan

// Submit membuat pengajuan pending_review. Up pertama wajib lolos validasi
// untuk revisi terbaru.
func (w *Workflow) Submit(ctx context.Context, actor auth.User, connID, migID int64, action, note string) (Request, error) {
	if err := w.can(ctx, actor, connID, access.ActionEdit); err != nil {
		return Request{}, err
	}
	ctx, span := activity.StartSpan(ctx, "migration.request.submit")
	defer span.End()

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := lockConnection(ctx, tx, connID); err != nil {
		return Request{}, err
	}
	m, err := scanMigration(tx.QueryRow(ctx, `SELECT `+migrationColumns+migrationJoins+` WHERE m.id = $1`, migID))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && m.ConnectionID != connID) {
		return Request{}, ErrMigrationNotFound
	}
	if err != nil {
		return Request{}, err
	}
	if m.OpenRequest != nil {
		return Request{}, ErrRequestExists
	}
	states, err := loadStates(ctx, tx, connID)
	if err != nil {
		return Request{}, err
	}
	plan, err := PlanAction(states, migID, strings.ToLower(strings.TrimSpace(action)))
	if err != nil {
		w.log(ctx, actor, "migration.request.submit", activity.StatusWarning, map[string]string{
			"database_id": idStr(connID), "migration_id": idStr(migID), "action": action, "reason": err.Error(),
		}, nil)
		return Request{}, err
	}
	if plan.Action == ActionUp && !m.ValidatedForCurrent() {
		return Request{}, ErrValidationRequired
	}

	var target any
	if plan.TargetExecution != 0 {
		target = plan.TargetExecution
	}
	var reqID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO migration_requests (migration_id, connection_id, action, target_execution_id,
		                                 migration_revision, checksum, status, note, requested_by)
		 VALUES ($1, $2, $3, $4, $5, $6, 'pending_review', $7, $8)
		 RETURNING id`,
		m.ID, connID, plan.Action, target, m.Revision, m.Checksum, nullable(strings.TrimSpace(note)), actor.ID).Scan(&reqID)
	if isUniqueViolation(err) {
		return Request{}, ErrRequestExists
	}
	if err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	w.log(ctx, actor, "migration.request.submit", activity.StatusSuccess, map[string]string{
		"database_id": idStr(connID), "migration_id": idStr(migID), "request_id": idStr(reqID),
		"action": plan.Action, "revision": strconv.Itoa(m.Revision),
	}, nil)
	return w.GetRequest(ctx, actor, reqID)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Decision = hasil approve/reject.
type Decision struct {
	Request Request `json:"request"`
	JobID   int64   `json:"job_id,omitempty"`
}

// Approve berlaku untuk semua group yang dipimpin actor. Approval terakhir
// langsung mengantrikan job; SQL tidak ditunggu.
func (w *Workflow) Approve(ctx context.Context, actor auth.User, reqID int64, note string) (Decision, error) {
	return w.decide(ctx, actor, reqID, ReviewApproved, note)
}

// Reject mencatat penolakan; satu penolakan cukup untuk menolak pengajuan.
func (w *Workflow) Reject(ctx context.Context, actor auth.User, reqID int64, note string) (Decision, error) {
	if strings.TrimSpace(note) == "" {
		return Decision{}, ErrRejectNoteRequired
	}
	return w.decide(ctx, actor, reqID, ReviewRejected, note)
}

func (w *Workflow) decide(ctx context.Context, actor auth.User, reqID int64, decision, note string) (Decision, error) {
	ctx, span := activity.StartSpan(ctx, "migration.request."+decision)
	defer span.End()

	connID, err := w.requestConnection(ctx, reqID)
	if err != nil {
		return Decision{}, err
	}
	if err := w.can(ctx, actor, connID, access.ActionView); err != nil {
		return Decision{}, err
	}
	if decision == ReviewApproved && w.queue == nil {
		return Decision{}, ErrNoQueue
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return Decision{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := lockConnection(ctx, tx, connID); err != nil {
		return Decision{}, err
	}

	var status, reqChecksum, currentChecksum string
	var migID int64
	err = tx.QueryRow(ctx,
		`SELECT r.status, r.checksum, m.checksum, m.id
		   FROM migration_requests r JOIN migrations m ON m.id = r.migration_id
		  WHERE r.id = $1 FOR UPDATE OF r`, reqID).Scan(&status, &reqChecksum, &currentChecksum, &migID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Decision{}, ErrRequestNotFound
	}
	if err != nil {
		return Decision{}, err
	}
	if status != RequestPendingReview {
		return Decision{}, ErrRequestNotPending
	}
	if reqChecksum != currentChecksum {
		return Decision{}, ErrChecksumMismatch
	}

	reviewers, err := loadReviewers(ctx, tx, connID, reqID)
	if err != nil {
		return Decision{}, err
	}
	mine := PendingGroupsLedBy(reviewers, actor.ID)
	if len(mine) == 0 {
		return Decision{}, ErrNotReviewer
	}
	groupNames := make([]string, 0, len(mine))
	for _, g := range mine {
		if _, err := tx.Exec(ctx,
			`INSERT INTO migration_request_reviews (request_id, group_id, decision, note, decided_by)
			 VALUES ($1, $2, $3, $4, $5)`,
			reqID, g.GroupID, decision, nullable(strings.TrimSpace(note)), actor.ID); err != nil {
			return Decision{}, err
		}
		groupNames = append(groupNames, g.GroupName)
	}

	reviewers, err = loadReviewers(ctx, tx, connID, reqID)
	if err != nil {
		return Decision{}, err
	}
	var jobID int64
	outcome := ReviewOutcome(reviewers)
	switch outcome {
	case ReviewRejected:
		if _, err := tx.Exec(ctx,
			`UPDATE migration_requests SET status = 'rejected', decided_at = now(), updated_at = now()
			  WHERE id = $1`, reqID); err != nil {
			return Decision{}, err
		}
	case ReviewApproved:
		if _, err := tx.Exec(ctx,
			`UPDATE migration_requests SET status = 'approved', decided_at = now(), updated_at = now()
			  WHERE id = $1`, reqID); err != nil {
			return Decision{}, err
		}
		jobID, err = w.queue.Enqueue(ctx, tx, reqID, connID)
		if err != nil {
			return Decision{}, fmt.Errorf("publikasi job: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE migration_requests SET status = 'queued', queued_at = now(), updated_at = now()
			  WHERE id = $1`, reqID); err != nil {
			return Decision{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Decision{}, err
	}

	w.log(ctx, actor, "migration.request."+decision, activity.StatusSuccess, map[string]string{
		"database_id": idStr(connID), "migration_id": idStr(migID), "request_id": idStr(reqID),
		"groups": strings.Join(groupNames, ","), "outcome": outcome, "job_id": idStr(jobID),
	}, nil)
	req, err := w.GetRequest(ctx, actor, reqID)
	return Decision{Request: req, JobID: jobID}, err
}

// Cancel membatalkan pengajuan yang belum masuk queue. Hanya pengaju atau
// user dengan akses edit.
func (w *Workflow) Cancel(ctx context.Context, actor auth.User, reqID int64) (Request, error) {
	connID, err := w.requestConnection(ctx, reqID)
	if err != nil {
		return Request{}, err
	}
	if err := w.can(ctx, actor, connID, access.ActionEdit); err != nil {
		return Request{}, err
	}
	tag, err := w.pool.Exec(ctx,
		`UPDATE migration_requests SET status = 'cancelled', finished_at = now(), updated_at = now()
		  WHERE id = $1 AND status IN ('pending_review', 'approved')`, reqID)
	if err != nil {
		return Request{}, err
	}
	if tag.RowsAffected() == 0 {
		return Request{}, ErrCannotCancel
	}
	w.log(ctx, actor, "migration.request.cancel", activity.StatusSuccess, map[string]string{
		"database_id": idStr(connID), "request_id": idStr(reqID),
	}, nil)
	return w.GetRequest(ctx, actor, reqID)
}

func (w *Workflow) requestConnection(ctx context.Context, reqID int64) (int64, error) {
	var connID int64
	err := w.pool.QueryRow(ctx, `SELECT connection_id FROM migration_requests WHERE id = $1`, reqID).Scan(&connID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrRequestNotFound
	}
	return connID, err
}

// Bacaan

// GetRequest = detail pengajuan: reviewer per group, SQL yang akan/telah
// dijalankan, dan seluruh attempt eksekusinya.
func (w *Workflow) GetRequest(ctx context.Context, actor auth.User, reqID int64) (Request, error) {
	connID, err := w.requestConnection(ctx, reqID)
	if err != nil {
		return Request{}, err
	}
	if err := w.can(ctx, actor, connID, access.ActionView); err != nil {
		return Request{}, err
	}
	r, err := getRequest(ctx, w.pool, reqID)
	if err != nil {
		return Request{}, err
	}
	if r.Reviewers, err = loadReviewers(ctx, w.pool, connID, reqID); err != nil {
		return Request{}, err
	}
	r.ReviewStatus = ReviewOutcome(r.Reviewers)
	if r.Executions, err = listExecutionsByRequest(ctx, w.pool, reqID); err != nil {
		return Request{}, err
	}
	m, err := w.store.Get(ctx, r.MigrationID)
	if err != nil {
		return Request{}, err
	}
	r.SQL = m.UpSQL
	if r.Action == ActionRollback {
		r.SQL = m.DownSQL
	}
	w.decorate(ctx, actor, &r)
	return r, nil
}

func (w *Workflow) decorate(ctx context.Context, actor auth.User, r *Request) {
	if r.Status == RequestPendingReview {
		if r.Reviewers == nil {
			r.Reviewers, _ = loadReviewers(ctx, w.pool, r.ConnectionID, r.ID)
		}
		r.CanApprove = len(PendingGroupsLedBy(r.Reviewers, actor.ID)) > 0
	}
	if r.Status == RequestPendingReview || r.Status == RequestApproved {
		ok, err := w.policy.Can(ctx, actor.ID, r.ConnectionID, access.ActionEdit)
		r.CanCancel = err == nil && ok
	}
}

// ListRequests = pengajuan pada database (filter status opsional).
func (w *Workflow) ListRequests(ctx context.Context, actor auth.User, connID int64, statuses []string) ([]Request, error) {
	if err := w.can(ctx, actor, connID, access.ActionView); err != nil {
		return nil, err
	}
	items, err := listRequests(ctx, w.pool, connID, statuses)
	if err != nil {
		return nil, err
	}
	for i := range items {
		w.decorate(ctx, actor, &items[i])
		items[i].Reviewers = nil
	}
	return items, nil
}

// Inbox = pengajuan lintas database yang menunggu keputusan actor sebagai leader.
func (w *Workflow) Inbox(ctx context.Context, actor auth.User) ([]Request, error) {
	items, err := listInbox(ctx, w.pool, actor.ID)
	if err != nil {
		return nil, err
	}
	out := items[:0]
	for _, r := range items {
		// Leader tetap harus punya akses ke database (lewat group mana pun).
		if ok, err := w.policy.Can(ctx, actor.ID, r.ConnectionID, access.ActionView); err != nil || !ok {
			continue
		}
		r.CanApprove = true
		out = append(out, r)
	}
	return out, nil
}

// History = riwayat eksekusi. hideRollback menyembunyikan pasangan eksekusi
// yang dibatalkan dan rollback sukses yang membatalkannya (hanya tampilan).
func (w *Workflow) History(ctx context.Context, actor auth.User, connID int64, hideRollback bool) ([]Execution, error) {
	if err := w.can(ctx, actor, connID, access.ActionView); err != nil {
		return nil, err
	}
	items, err := listExecutions(ctx, w.pool, connID, 1000)
	if err != nil {
		return nil, err
	}
	if hideRollback {
		items = HideRolledBack(items)
	}
	return items, nil
}

func (w *Workflow) GetExecution(ctx context.Context, actor auth.User, connID, execID int64) (Execution, error) {
	if err := w.can(ctx, actor, connID, access.ActionView); err != nil {
		return Execution{}, err
	}
	e, err := getExecution(ctx, w.pool, execID)
	if err != nil {
		return Execution{}, err
	}
	if e.ConnectionID != connID {
		return Execution{}, ErrExecutionNotFound
	}
	return e, nil
}

// MigrationExecutions = seluruh attempt milik satu migration (detail migration).
func (w *Workflow) MigrationExecutions(ctx context.Context, actor auth.User, connID, migID int64) ([]Execution, error) {
	if _, err := w.GetMigration(ctx, actor, connID, migID); err != nil {
		return nil, err
	}
	return listExecutionsByMigration(ctx, w.pool, migID)
}

// MigrationRequests = seluruh pengajuan milik satu migration.
func (w *Workflow) MigrationRequests(ctx context.Context, actor auth.User, connID, migID int64) ([]Request, error) {
	if _, err := w.GetMigration(ctx, actor, connID, migID); err != nil {
		return nil, err
	}
	items, err := collectRequests(w.pool.Query(ctx,
		`SELECT `+requestColumns+requestJoins+` WHERE r.migration_id = $1 ORDER BY r.id DESC`, migID))
	if err != nil {
		return nil, err
	}
	for i := range items {
		w.decorate(ctx, actor, &items[i])
		items[i].Reviewers = nil
	}
	return items, nil
}

// AllowedActions = aksi yang sah saat ini; up hanya ditawarkan setelah
// validasi revisi terbaru lolos.
func (w *Workflow) AllowedActions(ctx context.Context, connID int64, m Migration) []string {
	out := []string{"validate"}
	if m.OpenRequest != nil {
		return out
	}
	states, err := loadStates(ctx, w.pool, connID)
	if err != nil {
		return out
	}
	if plan, err := PlanAction(states, m.ID, ActionUp); err == nil {
		if plan.Action != ActionUp || m.ValidatedForCurrent() {
			out = append(out, plan.Action)
		}
	}
	if _, err := PlanAction(states, m.ID, ActionRollback); err == nil {
		out = append(out, ActionRollback)
	}
	if _, err := PlanAction(states, m.ID, ActionResume); err == nil {
		out = append(out, ActionResume)
	}
	return out
}
