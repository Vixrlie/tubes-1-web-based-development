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
	for i := range requests {
		r := &requests[i]
		if r.Reviewers, err = loadReviewers(ctx, w.pool, connID, r.ID); err != nil {
			return b, err
		}
		r.ReviewStatus = ReviewOutcome(r.Reviewers)
		if r.Executions, err = listExecutionsByRequest(ctx, w.pool, r.ID); err != nil {
			return b, err
		}
		w.decorate(ctx, actor, r)
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
	ok, err := w.policy.Can(ctx, actor.ID, connID, access.ActionEdit)
	b.CanEdit = err == nil && ok
	return b, nil
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
