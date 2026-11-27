package dbm

import (
	"context"
	"net/http"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

// Board = seluruh workflow migration satu database dalam satu respons, untuk
// SPA yang menampilkan draft, review, antrean, dan riwayat sekaligus.
type Board struct {
	Migrations []Migration `json:"migrations"`
	// Requests berisi reviewer dan execution tiap pengajuan, terbaru dulu.
	Requests []Request `json:"requests"`
	// LeaderGroupIDs = grup yang dipimpin user; SPA memakainya untuk tombol review.
	LeaderGroupIDs []int64 `json:"leader_group_ids"`
	CanEdit        bool    `json:"can_edit"`
}

// Board memuat migration (dengan SQL), pengajuan, reviewer, dan execution.
func (w *Workflow) Board(ctx context.Context, actor auth.User, connID int64) (Board, error) {
	b := Board{Migrations: []Migration{}, Requests: []Request{}, LeaderGroupIDs: []int64{}}
	if err := w.can(ctx, actor, connID, access.ActionView); err != nil {
		return b, err
	}
	migrations, err := w.store.List(ctx, connID)
	if err != nil {
		return b, err
	}
	if migrations != nil {
		b.Migrations = migrations
	}
	requests, err := collectRequests(w.pool.Query(ctx,
		`SELECT `+requestColumns+requestJoins+` WHERE r.connection_id = $1 ORDER BY r.id DESC`, connID))
	if err != nil {
		return b, err
	}
	// Reviewer dan execution dimuat sekaligus untuk semua pengajuan (bukan
	// per pengajuan): versi N+1 sebelumnya menghabiskan pool koneksi saat
	// stress test k6 sehingga request baca lain ikut timeout.
	reviewers, err := loadBoardReviewers(ctx, w.pool, connID)
	if err != nil {
		return b, err
	}
	executions, err := collectExecutions(w.pool.Query(ctx,
		`SELECT `+executionColumns+executionJoins+` WHERE e.connection_id = $1 ORDER BY e.id`, connID))
	if err != nil {
		return b, err
	}
	byRequest := map[int64][]Execution{}
	for _, e := range executions {
		byRequest[e.RequestID] = append(byRequest[e.RequestID], e)
	}
	canEdit, err := w.policy.Can(ctx, actor.ID, connID, access.ActionEdit)
	b.CanEdit = err == nil && canEdit
	for i := range requests {
		r := &requests[i]
		r.Reviewers = reviewers[r.ID]
		r.ReviewStatus = ReviewOutcome(r.Reviewers)
		r.Executions = byRequest[r.ID]
		if r.Status == RequestPendingReview {
			r.CanApprove = len(PendingGroupsLedBy(r.Reviewers, actor.ID)) > 0
		}
		r.CanCancel = (r.Status == RequestPendingReview || r.Status == RequestApproved) && b.CanEdit
	}
	b.Requests = requests
	rows, err := w.pool.Query(ctx, `SELECT id FROM groups WHERE leader_id = $1 ORDER BY id`, actor.ID)
	if err != nil {
		return b, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return b, err
		}
		b.LeaderGroupIDs = append(b.LeaderGroupIDs, id)
	}
	if err := rows.Err(); err != nil {
		return b, err
	}
	return b, nil
}

// loadBoardReviewers = loadReviewers untuk semua pengajuan satu database
// dalam satu query: setiap grup yang saat ini punya grant ke database beserta
// keputusan grup itu pada tiap pengajuan.
func loadBoardReviewers(ctx context.Context, q pgxQuerier, connID int64) (map[int64][]ReviewerGroup, error) {
	rows, err := q.Query(ctx,
		`SELECT r.id, gr.id, gr.name, g.level,
		        CASE WHEN lu.id IS NULL THEN 0 ELSE gr.leader_id END,
		        COALESCE(lu.username, ''),
		        COALESCE(rv.decision, 'pending'), COALESCE(rv.note, ''), COALESCE(du.username, ''), rv.decided_at
		   FROM migration_requests r
		   JOIN grants g ON g.connection_id = r.connection_id
		   JOIN groups gr ON gr.id = g.group_id
		   LEFT JOIN users lu ON lu.id = gr.leader_id AND lu.is_active
		   LEFT JOIN migration_request_reviews rv ON rv.request_id = r.id AND rv.group_id = gr.id
		   LEFT JOIN users du ON du.id = rv.decided_by
		  WHERE r.connection_id = $1
		  ORDER BY r.id, gr.name`, connID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]ReviewerGroup{}
	for rows.Next() {
		var reqID int64
		var g ReviewerGroup
		if err := rows.Scan(&reqID, &g.GroupID, &g.GroupName, &g.Level, &g.LeaderID, &g.LeaderName,
			&g.Decision, &g.Note, &g.DecidedBy, &g.DecidedAt); err != nil {
			return nil, err
		}
		out[reqID] = append(out[reqID], g)
	}
	return out, rows.Err()
}

func (h *Handlers) apiBoard(w http.ResponseWriter, r *http.Request) {
	user, ok := apiActor(w, r)
	if !ok {
		return
	}
	connID, ok := pathID(w, r, "connID")
	if !ok {
		return
	}
	b, err := h.workflow.Board(r.Context(), user, connID)
	if err != nil {
		h.apiFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}
