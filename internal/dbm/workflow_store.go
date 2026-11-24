package dbm

// Kueri baca untuk pengajuan (request), reviewer per group, dan riwayat
// eksekusi. Operasi tulis ada di workflow.go dan executor.go.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrRequestNotFound   = errors.New("pengajuan migration tidak ditemukan")
	ErrExecutionNotFound = errors.New("eksekusi migration tidak ditemukan")
)

// Request = satu pengajuan up/rollback/forward beserta status workflow-nya.
type Request struct {
	ID                int64      `json:"id"`
	MigrationID       int64      `json:"migration_id"`
	ConnectionID      int64      `json:"connection_id"`
	ConnectionName    string     `json:"connection_name"`
	MigrationName     string     `json:"migration_name"`
	MigrationVersion  int64      `json:"migration_version"`
	Action            string     `json:"action"`
	TargetExecutionID *int64     `json:"target_execution_id,omitempty"`
	Revision          int        `json:"revision"`
	Checksum          string     `json:"checksum"`
	Status            string     `json:"status"`
	Note              string     `json:"note,omitempty"`
	Error             string     `json:"error,omitempty"`
	RequestedBy       *int64     `json:"requested_by,omitempty"`
	RequestedByName   string     `json:"requested_by_name,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
	QueuedAt          *time.Time `json:"queued_at,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	JobID             *int64     `json:"job_id,omitempty"`
	JobStatus         string     `json:"job_status,omitempty"`
	JobAttempts       int        `json:"job_attempts,omitempty"`

	// Hanya diisi pada detail.
	ReviewStatus string          `json:"review_status,omitempty"`
	Reviewers    []ReviewerGroup `json:"reviewers,omitempty"`
	SQL          string          `json:"sql,omitempty"`
	Executions   []Execution     `json:"executions,omitempty"`
	// Diisi relatif terhadap user yang meminta.
	CanApprove bool `json:"can_approve"`
	CanCancel  bool `json:"can_cancel"`
}

// Execution = satu attempt eksekusi; baris riwayat migration.
type Execution struct {
	ID                 int64      `json:"id"`
	RequestID          int64      `json:"request_id"`
	MigrationID        int64      `json:"migration_id"`
	ConnectionID       int64      `json:"connection_id"`
	MigrationName      string     `json:"migration_name"`
	Action             string     `json:"action"`
	RevertsExecutionID *int64     `json:"reverts_execution_id,omitempty"`
	RevertedBy         *int64     `json:"reverted_by,omitempty"`
	Version            int64      `json:"migration_version"`
	Revision           int        `json:"migration_revision"`
	SQL                string     `json:"sql_executed,omitempty"`
	Status             string     `json:"status"`
	ErrorStage         string     `json:"error_stage,omitempty"`
	Error              string     `json:"error,omitempty"`
	SQLState           string     `json:"sqlstate,omitempty"`
	Attempt            int        `json:"attempt"`
	JobID              *int64     `json:"job_id,omitempty"`
	ExecutedBy         *int64     `json:"executed_by,omitempty"`
	ExecutedByName     string     `json:"executed_by_name,omitempty"`
	StartedAt          time.Time  `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	DurationMS         *int64     `json:"duration_ms,omitempty"`
	RowsAffected       *int64     `json:"rows_affected,omitempty"`

	// Progres mode batch (nil BatchKey = migration biasa).
	BatchKey         *int64            `json:"batch_key,omitempty"`
	BatchSize        *int              `json:"batch_size,omitempty"`
	CommittedBatches int               `json:"committed_batches"`
	CommittedRows    int64             `json:"committed_rows"`
	TotalBatches     *int              `json:"total_batches,omitempty"`
	CursorLast       string            `json:"cursor_last,omitempty"`
	Checkpoints      []BatchCheckpoint `json:"checkpoints,omitempty"`
}

func (e Execution) ExecID() int64      { return e.ID }
func (e Execution) ExecStatus() string { return e.Status }
func (e Execution) RevertsID() int64 {
	if e.RevertsExecutionID == nil {
		return 0
	}
	return *e.RevertsExecutionID
}

const requestColumns = `
       r.id, r.migration_id, r.connection_id, dc.name, m.name, m.version, r.action,
       r.target_execution_id, r.migration_revision, r.checksum, r.status,
       COALESCE(r.note, ''), COALESCE(r.error, ''), r.requested_by, COALESCE(u.username, ''),
       r.created_at, r.decided_at, r.queued_at, r.started_at, r.finished_at,
       j.id, COALESCE(j.status, ''), COALESCE(j.attempts, 0)`

const requestJoins = `
      FROM migration_requests r
      JOIN migrations m      ON m.id = r.migration_id
      JOIN db_connections dc ON dc.id = r.connection_id
      LEFT JOIN users u      ON u.id = r.requested_by
      LEFT JOIN migration_jobs j ON j.request_id = r.id`

func scanRequest(row pgx.Row) (Request, error) {
	var r Request
	err := row.Scan(
		&r.ID, &r.MigrationID, &r.ConnectionID, &r.ConnectionName, &r.MigrationName,
		&r.MigrationVersion, &r.Action, &r.TargetExecutionID, &r.Revision, &r.Checksum,
		&r.Status, &r.Note, &r.Error, &r.RequestedBy, &r.RequestedByName,
		&r.CreatedAt, &r.DecidedAt, &r.QueuedAt, &r.StartedAt, &r.FinishedAt,
		&r.JobID, &r.JobStatus, &r.JobAttempts,
	)
	return r, err
}

func collectRequests(rows pgx.Rows, err error) ([]Request, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Request, 0, 8)
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func getRequest(ctx context.Context, q pgxQuerier, id int64) (Request, error) {
	r, err := scanRequest(q.QueryRow(ctx, `SELECT `+requestColumns+requestJoins+` WHERE r.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrRequestNotFound
	}
	return r, err
}

// listRequests mengembalikan pengajuan pada koneksi, terbaru dahulu.
// statuses kosong berarti semua status.
func listRequests(ctx context.Context, q pgxQuerier, connID int64, statuses []string) ([]Request, error) {
	if len(statuses) == 0 {
		return collectRequests(q.Query(ctx,
			`SELECT `+requestColumns+requestJoins+`
			  WHERE r.connection_id = $1 ORDER BY r.id DESC LIMIT 200`, connID))
	}
	return collectRequests(q.Query(ctx,
		`SELECT `+requestColumns+requestJoins+`
		  WHERE r.connection_id = $1 AND r.status = ANY($2)
		  ORDER BY r.id DESC LIMIT 200`, connID, statuses))
}

// listInbox = pengajuan yang menunggu keputusan dari group yang dipimpin user.
func listInbox(ctx context.Context, q pgxQuerier, userID int64) ([]Request, error) {
	return collectRequests(q.Query(ctx,
		`SELECT `+requestColumns+requestJoins+`
		  WHERE r.status = 'pending_review'
		    AND EXISTS (
		        SELECT 1 FROM groups gr
		          JOIN grants g ON g.group_id = gr.id AND g.connection_id = r.connection_id
		         WHERE gr.leader_id = $1
		           AND NOT EXISTS (SELECT 1 FROM migration_request_reviews rv
		                            WHERE rv.request_id = r.id AND rv.group_id = gr.id))
		  ORDER BY r.id`, userID))
}

// loadReviewers dihitung dari grant saat ini: group yang baru mendapat akses
// setelah pengajuan ikut wajib menyetujui.
func loadReviewers(ctx context.Context, q pgxQuerier, connID, reqID int64) ([]ReviewerGroup, error) {
	rows, err := q.Query(ctx,
		`SELECT gr.id, gr.name, g.level,
		        CASE WHEN lu.id IS NULL THEN 0 ELSE gr.leader_id END,
		        COALESCE(lu.username, ''),
		        COALESCE(rv.decision, 'pending'), COALESCE(rv.note, ''), COALESCE(du.username, ''), rv.decided_at
		   FROM grants g
		   JOIN groups gr ON gr.id = g.group_id
		   LEFT JOIN users lu ON lu.id = gr.leader_id AND lu.is_active
		   LEFT JOIN migration_request_reviews rv ON rv.request_id = $2 AND rv.group_id = gr.id
		   LEFT JOIN users du ON du.id = rv.decided_by
		  WHERE g.connection_id = $1
		  ORDER BY gr.name`, connID, reqID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReviewerGroup
	for rows.Next() {
		var g ReviewerGroup
		if err := rows.Scan(&g.GroupID, &g.GroupName, &g.Level, &g.LeaderID, &g.LeaderName,
			&g.Decision, &g.Note, &g.DecidedBy, &g.DecidedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

const executionColumns = `
       e.id, e.request_id, e.migration_id, e.connection_id, m.name, e.action,
       e.reverts_execution_id, rb.id, e.migration_version, e.migration_revision,
       e.sql_executed, e.status, COALESCE(e.error_stage, ''), COALESCE(e.error, ''),
       COALESCE(e.sqlstate, ''), e.attempt, e.job_id, e.executed_by, COALESCE(u.username, ''),
       e.started_at, e.finished_at, e.duration_ms, e.rows_affected,
       e.batch_key, e.batch_size, e.committed_batches, e.committed_rows, e.total_batches,
       COALESCE(e.cursor_last, '')`

const executionJoins = `
      FROM migration_executions e
      JOIN migrations m ON m.id = e.migration_id
      LEFT JOIN users u ON u.id = e.executed_by
      LEFT JOIN migration_executions rb
             ON rb.reverts_execution_id = e.id AND rb.status = 'success'`

func scanExecution(row pgx.Row) (Execution, error) {
	var e Execution
	err := row.Scan(
		&e.ID, &e.RequestID, &e.MigrationID, &e.ConnectionID, &e.MigrationName, &e.Action,
		&e.RevertsExecutionID, &e.RevertedBy, &e.Version, &e.Revision,
		&e.SQL, &e.Status, &e.ErrorStage, &e.Error,
		&e.SQLState, &e.Attempt, &e.JobID, &e.ExecutedBy, &e.ExecutedByName,
		&e.StartedAt, &e.FinishedAt, &e.DurationMS, &e.RowsAffected,
		&e.BatchKey, &e.BatchSize, &e.CommittedBatches, &e.CommittedRows, &e.TotalBatches,
		&e.CursorLast,
	)
	return e, err
}

func collectExecutions(rows pgx.Rows, err error) ([]Execution, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Execution, 0, 8)
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// listExecutions = riwayat eksekusi koneksi, terbaru dahulu.
func listExecutions(ctx context.Context, q pgxQuerier, connID int64, limit int) ([]Execution, error) {
	return collectExecutions(q.Query(ctx,
		`SELECT `+executionColumns+executionJoins+`
		  WHERE e.connection_id = $1 ORDER BY e.id DESC LIMIT $2`, connID, limit))
}

func listExecutionsByRequest(ctx context.Context, q pgxQuerier, reqID int64) ([]Execution, error) {
	return collectExecutions(q.Query(ctx,
		`SELECT `+executionColumns+executionJoins+`
		  WHERE e.request_id = $1 ORDER BY e.id`, reqID))
}

func listExecutionsByMigration(ctx context.Context, q pgxQuerier, migID int64) ([]Execution, error) {
	return collectExecutions(q.Query(ctx,
		`SELECT `+executionColumns+executionJoins+`
		  WHERE e.migration_id = $1 ORDER BY e.id DESC`, migID))
}

func getExecution(ctx context.Context, q pgxQuerier, id int64) (Execution, error) {
	e, err := scanExecution(q.QueryRow(ctx, `SELECT `+executionColumns+executionJoins+` WHERE e.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Execution{}, ErrExecutionNotFound
	}
	if err != nil || e.BatchKey == nil {
		return e, err
	}
	// Detail eksekusi batch menyertakan checkpoint yang dicatat attempt ini.
	rows, err := q.Query(ctx,
		`SELECT batch_number, cursor_start, cursor_end, rows_affected
		   FROM migration_batch_checkpoints WHERE execution_id = $1 ORDER BY id`, id)
	if err != nil {
		return e, err
	}
	defer rows.Close()
	for rows.Next() {
		var cp BatchCheckpoint
		if err := rows.Scan(&cp.Number, &cp.CursorStart, &cp.CursorEnd, &cp.Rows); err != nil {
			return e, err
		}
		e.Checkpoints = append(e.Checkpoints, cp)
	}
	return e, rows.Err()
}
