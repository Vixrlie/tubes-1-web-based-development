package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

type adminGroupAccessDTO struct {
	ConnectionID   string `json:"connectionId,omitempty"`
	ConnectionName string `json:"connectionName"`
	Level          string `json:"level"`
}

type adminGroupDTO struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	MemberCount int                   `json:"memberCount"`
	Accesses    []adminGroupAccessDTO `json:"accesses"`
	CreatedAt   string                `json:"createdAt"`
}

type adminMemberDTO struct {
	ID              string `json:"id"`
	Username        string `json:"username"`
	FullName        string `json:"fullName"`
	Email           string `json:"email"`
	IsPlatformAdmin bool   `json:"isPlatformAdmin"`
	IsActive        bool   `json:"isActive"`
}

type adminConnRefDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type adminGroupDetailDTO struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	CreatedAt            string                `json:"createdAt"`
	Members              []adminMemberDTO      `json:"members"`
	CandidateUsers       []adminMemberDTO      `json:"candidateUsers"`
	Grants               []adminGroupAccessDTO `json:"grants"`
	AvailableConnections []adminConnRefDTO     `json:"availableConnections"`
	// LeaderID = anggota yang mewakili grup saat approval migration (null bila kosong).
	LeaderID *string `json:"leaderId"`
}

func (a *adminAPI) listGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := a.cfg.Groups.ListGroups(r.Context())
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat daftar grup.")
		return
	}
	accessesByGroup, err := a.cfg.Groups.GroupAccesses(r.Context())
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat akses grup.")
		return
	}
	items := make([]adminGroupDTO, 0, len(groups))
	for _, g := range groups {
		acc := make([]adminGroupAccessDTO, 0, len(accessesByGroup[g.ID]))
		for _, ga := range accessesByGroup[g.ID] {
			acc = append(acc, adminGroupAccessDTO{
				ConnectionName: ga.Database,
				Level:          ga.Level,
			})
		}
		items = append(items, adminGroupDTO{
			ID:          formatID(g.ID),
			Name:        g.Name,
			MemberCount: g.MemberCount,
			Accesses:    acc,
			CreatedAt:   g.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

type groupNameRequest struct {
	Name string `json:"name"`
}

func (a *adminAPI) createGroup(w http.ResponseWriter, r *http.Request) {
	var req groupNameRequest
	if err := decodeAdminJSON(w, r, &req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeAdminValidationError(w, "Nama grup wajib diisi.", map[string]string{"name": "Nama grup wajib diisi."})
		return
	}
	grp, err := a.cfg.Groups.CreateGroup(r.Context(), name)
	if errors.Is(err, ErrGroupNameTaken) {
		writeAdminError(w, http.StatusConflict, "GROUP_NAME_TAKEN", "Nama grup sudah dipakai.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal membuat grup.")
		return
	}
	writeAdminJSON(w, http.StatusCreated, adminGroupDTO{
		ID:          formatID(grp.ID),
		Name:        grp.Name,
		MemberCount: 0,
		Accesses:    []adminGroupAccessDTO{},
		CreatedAt:   grp.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (a *adminAPI) getGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
		return
	}
	grp, err := a.cfg.Groups.GetGroup(r.Context(), id)
	if errors.Is(err, ErrGroupNotFound) {
		writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat detail grup.")
		return
	}

	members, err := a.cfg.Groups.ListMembers(r.Context(), id)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat anggota grup.")
		return
	}
	grants, err := a.cfg.Groups.ListGrants(r.Context(), id)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat akses grup.")
		return
	}

	memberIDSet := make(map[int64]bool, len(members))
	memberDTOs := make([]adminMemberDTO, 0, len(members))
	for _, m := range members {
		memberIDSet[m.ID] = true
		memberDTOs = append(memberDTOs, adminMemberDTO{
			ID:              formatID(m.ID),
			Username:        m.Username,
			FullName:        m.FullName,
			Email:           m.Email,
			IsPlatformAdmin: m.IsPlatformAdmin,
			IsActive:        m.IsActive,
		})
	}

	candidateDTOs := make([]adminMemberDTO, 0)
	if a.cfg.Users != nil {
		allUsers, err := a.cfg.Users.List(r.Context())
		if err == nil {
			for _, u := range allUsers {
				if !memberIDSet[u.ID] {
					candidateDTOs = append(candidateDTOs, adminMemberDTO{
						ID:              formatID(u.ID),
						Username:        u.Username,
						FullName:        u.FullName,
						Email:           u.Email,
						IsPlatformAdmin: u.IsPlatformAdmin,
						IsActive:        u.IsActive,
					})
				}
			}
		}
	}

	grantConnSet := make(map[int64]bool, len(grants))
	grantDTOs := make([]adminGroupAccessDTO, 0, len(grants))
	for _, gr := range grants {
		grantConnSet[gr.ConnectionID] = true
		grantDTOs = append(grantDTOs, adminGroupAccessDTO{
			ConnectionID:   formatID(gr.ConnectionID),
			ConnectionName: gr.ConnectionName,
			Level:          string(gr.Level),
		})
	}

	availConns := make([]adminConnRefDTO, 0)
	if a.cfg.ConnLister != nil {
		conns, err := a.cfg.ConnLister.ListConnections(r.Context())
		if err == nil {
			for _, c := range conns {
				if !grantConnSet[c.ID] {
					availConns = append(availConns, adminConnRefDTO{
						ID:   formatID(c.ID),
						Name: c.Name,
					})
				}
			}
		}
	}

	var leaderID *string
	if ls, ok := a.cfg.Groups.(LeaderStore); ok {
		leader, err := ls.GroupLeader(r.Context(), id)
		if err != nil {
			writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat leader grup.")
			return
		}
		if leader != nil {
			v := formatID(*leader)
			leaderID = &v
		}
	}

	writeAdminJSON(w, http.StatusOK, adminGroupDetailDTO{
		LeaderID:             leaderID,
		ID:                   formatID(grp.ID),
		Name:                 grp.Name,
		CreatedAt:            grp.CreatedAt.UTC().Format(time.RFC3339),
		Members:              memberDTOs,
		CandidateUsers:       candidateDTOs,
		Grants:               grantDTOs,
		AvailableConnections: availConns,
	})
}

func (a *adminAPI) updateGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
		return
	}
	var req groupNameRequest
	if err := decodeAdminJSON(w, r, &req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeAdminValidationError(w, "Nama grup wajib diisi.", map[string]string{"name": "Nama grup wajib diisi."})
		return
	}
	err := a.cfg.Groups.RenameGroup(r.Context(), id, name)
	switch {
	case errors.Is(err, ErrGroupNotFound):
		writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
	case errors.Is(err, ErrGroupNameTaken):
		writeAdminError(w, http.StatusConflict, "GROUP_NAME_TAKEN", "Nama grup sudah dipakai.")
	case err != nil:
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengubah nama grup.")
	default:
		writeAdminJSON(w, http.StatusOK, map[string]string{"id": formatID(id), "name": name})
	}
}

func (a *adminAPI) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
		return
	}
	memberIDs, _ := a.cfg.Groups.MemberIDs(r.Context(), id)
	err := a.cfg.Groups.DeleteGroup(r.Context(), id)
	if errors.Is(err, ErrGroupNotFound) {
		writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menghapus grup.")
		return
	}
	a.revokeUsers(r.Context(), memberIDs)
	w.WriteHeader(http.StatusNoContent)
}

type addMembersRequest struct {
	UserIDs []string `json:"userIds"`
}

func (a *adminAPI) addGroupMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
		return
	}
	var req addMembersRequest
	if err := decodeAdminJSON(w, r, &req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.")
		return
	}
	if len(req.UserIDs) == 0 {
		writeAdminError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Pilih minimal satu pengguna.")
		return
	}
	grants, err := a.cfg.Groups.ListGrants(r.Context(), id)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memeriksa akses grup.")
		return
	}

	added := make([]int64, 0, len(req.UserIDs))
	for _, raw := range req.UserIDs {
		uid, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || uid <= 0 {
			writeAdminError(w, http.StatusBadRequest, "VALIDATION_ERROR", "ID pengguna tidak valid.")
			return
		}
		if err := a.cfg.Groups.AddMember(r.Context(), id, uid); err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
				return
			}
			if errors.Is(err, auth.ErrUserNotFound) {
				writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
				return
			}
			writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menambahkan anggota.")
			return
		}
		added = append(added, uid)
	}
	if len(grants) > 0 {
		a.forceRotateUsers(r.Context(), added)
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{"added": len(added)})
}

func (a *adminAPI) removeGroupMember(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
		return
	}
	userID, ok := parseParamID(r, "userID")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "MEMBER_NOT_FOUND", "Anggota tidak ditemukan.")
		return
	}
	err := a.cfg.Groups.RemoveMember(r.Context(), id, userID)
	if errors.Is(err, ErrMemberNotFound) {
		writeAdminError(w, http.StatusNotFound, "MEMBER_NOT_FOUND", "Anggota tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menghapus anggota.")
		return
	}
	a.revokeUsers(r.Context(), []int64{userID})
	w.WriteHeader(http.StatusNoContent)
}

type groupAccessUpdateItem struct {
	ConnectionID string `json:"connectionId"`
	Level        string `json:"level"`
}

type updateGroupAccessRequest struct {
	Grants []groupAccessUpdateItem `json:"grants"`
}

func (a *adminAPI) updateGroupAccess(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
		return
	}
	var req updateGroupAccessRequest
	if err := decodeAdminJSON(w, r, &req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.")
		return
	}

	desired := make(map[int64]access.Action, len(req.Grants))
	for _, item := range req.Grants {
		cid, err := strconv.ParseInt(strings.TrimSpace(item.ConnectionID), 10, 64)
		if err != nil || cid <= 0 {
			writeAdminError(w, http.StatusBadRequest, "VALIDATION_ERROR", "ID koneksi tidak valid.")
			return
		}
		lvl := access.Action(strings.TrimSpace(item.Level))
		if lvl != access.ActionView && lvl != access.ActionEdit {
			writeAdminError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Level akses harus view atau edit.")
			return
		}
		desired[cid] = lvl
	}

	existingList, err := a.cfg.Groups.ListGrants(r.Context(), id)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat akses grup saat ini.")
		return
	}
	existing := make(map[int64]access.Action, len(existingList))
	for _, g := range existingList {
		existing[g.ConnectionID] = g.Level
	}

	var escalated, downgraded bool
	for cid, oldLvl := range existing {
		newLvl, keep := desired[cid]
		if !keep {
			if err := a.cfg.Groups.RevokeGrant(r.Context(), id, cid); err != nil && !errors.Is(err, ErrGrantNotFound) {
				writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mencabut akses database.")
				return
			}
			downgraded = true
			continue
		}
		if access.Rank(newLvl) < access.Rank(oldLvl) {
			downgraded = true
		} else if access.Rank(newLvl) > access.Rank(oldLvl) {
			escalated = true
		}
	}

	for cid, lvl := range desired {
		if existing[cid] == lvl {
			continue
		}
		if _, had := existing[cid]; !had {
			escalated = true
		}
		if err := a.cfg.Groups.SetGrant(r.Context(), id, cid, lvl); err != nil {
			switch {
			case errors.Is(err, ErrGroupNotFound):
				writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
			case errors.Is(err, ErrConnectionNotFound):
				writeAdminError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Koneksi database tidak ditemukan.")
			case errors.Is(err, ErrInvalidLevel):
				writeAdminError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			default:
				writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menyimpan akses database.")
			}
			return
		}
	}

	memberIDs, _ := a.cfg.Groups.MemberIDs(r.Context(), id)
	if downgraded {
		a.revokeUsers(r.Context(), memberIDs)
	} else if escalated {
		a.forceRotateUsers(r.Context(), memberIDs)
	}

	writeAdminJSON(w, http.StatusOK, map[string]any{"updated": true})
}

type setLeaderRequest struct {
	// UserID kosong atau null mengosongkan leader.
	UserID *string `json:"userId"`
}

// setGroupLeader menetapkan leader grup (PUT /groups/{id}/leader).
func (a *adminAPI) setGroupLeader(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
		return
	}
	ls, ok := a.cfg.Groups.(LeaderStore)
	if !ok {
		writeAdminError(w, http.StatusNotImplemented, "NOT_SUPPORTED", "Leader grup tidak didukung.")
		return
	}
	var req setLeaderRequest
	if err := decodeAdminJSON(w, r, &req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.")
		return
	}
	var userID *int64
	if req.UserID != nil && strings.TrimSpace(*req.UserID) != "" {
		v, err := strconv.ParseInt(strings.TrimSpace(*req.UserID), 10, 64)
		if err != nil || v <= 0 {
			writeAdminError(w, http.StatusBadRequest, "VALIDATION_ERROR", "ID user tidak valid.")
			return
		}
		userID = &v
	}
	err := ls.SetLeader(r.Context(), id, userID)
	switch {
	case errors.Is(err, ErrGroupNotFound):
		writeAdminError(w, http.StatusNotFound, "GROUP_NOT_FOUND", "Grup tidak ditemukan.")
		return
	case errors.Is(err, ErrLeaderNotMember):
		writeAdminError(w, http.StatusUnprocessableEntity, "LEADER_NOT_MEMBER", "Leader harus anggota aktif grup.")
		return
	case err != nil:
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menetapkan leader.")
		return
	}
	var out *string
	if userID != nil {
		v := formatID(*userID)
		out = &v
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{"leaderId": out})
}
