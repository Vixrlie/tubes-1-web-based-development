package admin

// Leader group = satu anggota yang wajib menyetujui migration pada database
// yang dapat diakses group-nya (workflow approval Milestone 2).

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

var ErrLeaderNotMember = errors.New("leader harus anggota aktif group")

// LeaderStore dipisah dari GroupStore agar implementasi GroupStore lain
// (mis. fake di test) tidak wajib mendukung leader.
type LeaderStore interface {
	GroupLeader(ctx context.Context, groupID int64) (*int64, error)
	// SetLeader menetapkan leader; userID nil mengosongkannya.
	SetLeader(ctx context.Context, groupID int64, userID *int64) error
}

func (s *PGGroupStore) GroupLeader(ctx context.Context, groupID int64) (*int64, error) {
	var leader *int64
	err := s.pool.QueryRow(ctx, `SELECT leader_id FROM groups WHERE id = $1`, groupID).Scan(&leader)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGroupNotFound
	}
	return leader, err
}

func (s *PGGroupStore) SetLeader(ctx context.Context, groupID int64, userID *int64) error {
	if userID == nil {
		tag, err := s.pool.Exec(ctx, `UPDATE groups SET leader_id = NULL WHERE id = $1`, groupID)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrGroupNotFound
		}
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE groups g SET leader_id = $2
		  WHERE g.id = $1
		    AND EXISTS (SELECT 1 FROM memberships m JOIN users u ON u.id = m.user_id
		                 WHERE m.group_id = g.id AND m.user_id = $2 AND u.is_active)`,
		groupID, *userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := s.GetGroup(ctx, groupID); err != nil {
			return err
		}
		return ErrLeaderNotMember
	}
	return nil
}

// leaderCard menyiapkan pilihan leader dari anggota aktif group.
func (g *groupHandlers) leaderCard(ctx context.Context, groupID int64, members []UserRow, base string) (*web.LeaderForm, error) {
	ls, ok := g.store.(LeaderStore)
	if !ok {
		return nil, nil
	}
	current, err := ls.GroupLeader(ctx, groupID)
	if err != nil {
		return nil, err
	}
	form := &web.LeaderForm{
		FormAction: base + "/leader",
		Options:    []web.DropdownOption{{Value: "", Label: "Tanpa leader"}},
	}
	for _, m := range members {
		if !m.IsActive {
			continue
		}
		id := strconv.FormatInt(m.ID, 10)
		label := m.Username
		if m.FullName != "" {
			label += " (" + m.FullName + ")"
		}
		form.Options = append(form.Options, web.DropdownOption{Value: id, Label: label})
		if current != nil && *current == m.ID {
			form.Current = id
			form.CurrentLabel = label
		}
	}
	return form, nil
}

func (g *groupHandlers) setLeader(w http.ResponseWriter, r *http.Request) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	ls, ok := g.store.(LeaderStore)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var userID *int64
	if raw := r.PostFormValue("leader_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			g.renderDetail(w, r, http.StatusBadRequest, "", "leader tidak valid")
			return
		}
		userID = &id
	}
	if err := ls.SetLeader(r.Context(), grp.ID, userID); err != nil {
		switch {
		case errors.Is(err, ErrLeaderNotMember):
			g.renderDetail(w, r, http.StatusUnprocessableEntity, "", err.Error())
		case errors.Is(err, ErrGroupNotFound):
			http.NotFound(w, r)
		default:
			g.serverError(w, "tetapkan leader", err)
		}
		return
	}
	http.Redirect(w, r, "/admin/groups/"+strconv.FormatInt(grp.ID, 10), http.StatusSeeOther)
}
