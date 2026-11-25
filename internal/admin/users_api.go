package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

type adminUserDTO struct {
	ID              string   `json:"id"`
	Username        string   `json:"username"`
	FullName        string   `json:"fullName"`
	Email           string   `json:"email"`
	IsPlatformAdmin bool     `json:"isPlatformAdmin"`
	IsActive        bool     `json:"isActive"`
	Groups          []string `json:"groups"`
	CreatedAt       string   `json:"createdAt"`
}

type adminUserDetailDTO struct {
	adminUserDTO
	IsSelf          bool                   `json:"isSelf"`
	EffectiveAccess []adminEffectiveAccess `json:"effectiveAccess"`
}

type adminEffectiveAccess struct {
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	GroupID        string `json:"groupId"`
	GroupName      string `json:"groupName"`
	Level          string `json:"level"`
}

func toAdminUserDTO(u UserRow, groups []string) adminUserDTO {
	if groups == nil {
		groups = []string{}
	}
	return adminUserDTO{
		ID:              formatID(u.ID),
		Username:        u.Username,
		FullName:        u.FullName,
		Email:           u.Email,
		IsPlatformAdmin: u.IsPlatformAdmin,
		IsActive:        u.IsActive,
		Groups:          groups,
		CreatedAt:       u.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (a *adminAPI) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.cfg.Users.List(r.Context())
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat daftar pengguna.")
		return
	}
	groupsByUser, err := a.cfg.Users.GroupNamesByUser(r.Context())
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat grup pengguna.")
		return
	}
	items := make([]adminUserDTO, 0, len(users))
	for _, u := range users {
		items = append(items, toAdminUserDTO(u, groupsByUser[u.ID]))
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

type createUserRequest struct {
	Username        string `json:"username"`
	FullName        string `json:"fullName"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	IsPlatformAdmin bool   `json:"isPlatformAdmin"`
}

func (a *adminAPI) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeAdminJSON(w, r, &req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.")
		return
	}
	acc, err := auth.CreateUser(r.Context(), a.cfg.Users, auth.NewUser{
		Username:        req.Username,
		FullName:        strings.TrimSpace(req.FullName),
		Email:           strings.TrimSpace(req.Email),
		Password:        req.Password,
		IsPlatformAdmin: req.IsPlatformAdmin,
	})
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUsernameTaken):
			writeAdminError(w, http.StatusConflict, "USERNAME_TAKEN", "Username sudah dipakai.")
		case errors.Is(err, auth.ErrEmailTaken):
			writeAdminError(w, http.StatusConflict, "EMAIL_TAKEN", "Email sudah dipakai.")
		case errors.Is(err, auth.ErrUsernameInvalid), errors.Is(err, auth.ErrPasswordTooShort), errors.Is(err, auth.ErrPasswordTooLong):
			writeAdminError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		default:
			writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal membuat pengguna.")
		}
		return
	}
	row, err := a.cfg.Users.Get(r.Context(), acc.ID)
	if err != nil {
		writeAdminJSON(w, http.StatusCreated, adminUserDTO{
			ID:              formatID(acc.ID),
			Username:        acc.Username,
			FullName:        acc.FullName,
			Email:           acc.Email,
			IsPlatformAdmin: acc.IsPlatformAdmin,
			IsActive:        acc.IsActive,
			Groups:          []string{},
			CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	writeAdminJSON(w, http.StatusCreated, toAdminUserDTO(row, nil))
}

func (a *adminAPI) getUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	u, err := a.cfg.Users.Get(r.Context(), id)
	if errors.Is(err, auth.ErrUserNotFound) {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat detail pengguna.")
		return
	}
	groupsByUser, _ := a.cfg.Users.GroupNamesByUser(r.Context())
	eff, _ := a.cfg.Users.EffectiveAccess(r.Context(), id)
	accessList := make([]adminEffectiveAccess, 0, len(eff))
	for _, g := range eff {
		accessList = append(accessList, adminEffectiveAccess{
			ConnectionID:   formatID(g.ConnectionID),
			ConnectionName: g.ConnectionName,
			GroupID:        formatID(g.GroupID),
			GroupName:      g.GroupName,
			Level:          g.Level,
		})
	}
	me, _ := auth.UserFromContext(r.Context())
	writeAdminJSON(w, http.StatusOK, adminUserDetailDTO{
		adminUserDTO:    toAdminUserDTO(u, groupsByUser[u.ID]),
		IsSelf:          u.ID == me.ID,
		EffectiveAccess: accessList,
	})
}

type updateUserRequest struct {
	FullName        string `json:"fullName"`
	Email           string `json:"email"`
	IsPlatformAdmin bool   `json:"isPlatformAdmin"`
}

func (a *adminAPI) countOtherActiveAdmins(ctx context.Context, excludeID int64) (int, error) {
	users, err := a.cfg.Users.List(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, u := range users {
		if u.ID != excludeID && u.IsPlatformAdmin && u.IsActive {
			count++
		}
	}
	return count, nil
}

func (a *adminAPI) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	var req updateUserRequest
	if err := decodeAdminJSON(w, r, &req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.")
		return
	}
	before, err := a.cfg.Users.Get(r.Context(), id)
	if errors.Is(err, auth.ErrUserNotFound) {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat pengguna.")
		return
	}

	me, _ := auth.UserFromContext(r.Context())
	if before.IsPlatformAdmin && !req.IsPlatformAdmin {
		if id == me.ID {
			writeAdminError(w, http.StatusConflict, "SELF_DEMOTE_FORBIDDEN", "Tidak dapat mencabut peran admin dari akun sendiri.")
			return
		}
		if before.IsActive {
			other, err := a.countOtherActiveAdmins(r.Context(), id)
			if err != nil {
				writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memeriksa admin aktif.")
				return
			}
			if other == 0 {
				writeAdminError(w, http.StatusConflict, "LAST_ADMIN", "Admin aktif terakhir tidak dapat didemosi.")
				return
			}
		}
	}

	err = a.cfg.Users.Update(r.Context(), id, UserUpdate{
		FullName:        strings.TrimSpace(req.FullName),
		Email:           strings.TrimSpace(req.Email),
		IsPlatformAdmin: req.IsPlatformAdmin,
	})
	if errors.Is(err, auth.ErrEmailTaken) {
		writeAdminError(w, http.StatusConflict, "EMAIL_TAKEN", "Email sudah dipakai.")
		return
	}
	if errors.Is(err, auth.ErrUserNotFound) {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memperbarui pengguna.")
		return
	}

	if !before.IsPlatformAdmin && req.IsPlatformAdmin {
		a.forceRotateUsers(r.Context(), []int64{id})
	} else if before.IsPlatformAdmin && !req.IsPlatformAdmin {
		a.revokeUsers(r.Context(), []int64{id})
	}

	updated, err := a.cfg.Users.Get(r.Context(), id)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat pengguna.")
		return
	}
	groupsByUser, _ := a.cfg.Users.GroupNamesByUser(r.Context())
	writeAdminJSON(w, http.StatusOK, toAdminUserDTO(updated, groupsByUser[updated.ID]))
}

func (a *adminAPI) activateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	err := a.cfg.Users.SetActive(r.Context(), id, true)
	if errors.Is(err, auth.ErrUserNotFound) {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengaktifkan pengguna.")
		return
	}
	u, err := a.cfg.Users.Get(r.Context(), id)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat pengguna.")
		return
	}
	groupsByUser, _ := a.cfg.Users.GroupNamesByUser(r.Context())
	writeAdminJSON(w, http.StatusOK, toAdminUserDTO(u, groupsByUser[u.ID]))
}

func (a *adminAPI) deactivateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	me, _ := auth.UserFromContext(r.Context())
	if id == me.ID {
		writeAdminError(w, http.StatusConflict, "SELF_DEACTIVATE_FORBIDDEN", "Tidak dapat menonaktifkan akun sendiri.")
		return
	}
	u, err := a.cfg.Users.Get(r.Context(), id)
	if errors.Is(err, auth.ErrUserNotFound) {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat pengguna.")
		return
	}
	if u.IsPlatformAdmin && u.IsActive {
		other, err := a.countOtherActiveAdmins(r.Context(), id)
		if err != nil {
			writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memeriksa admin aktif.")
			return
		}
		if other == 0 {
			writeAdminError(w, http.StatusConflict, "LAST_ADMIN", "Admin aktif terakhir tidak dapat dinonaktifkan.")
			return
		}
	}
	if err := a.cfg.Users.SetActive(r.Context(), id, false); err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menonaktifkan pengguna.")
		return
	}
	a.revokeUsers(r.Context(), []int64{id})
	updated, err := a.cfg.Users.Get(r.Context(), id)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat pengguna.")
		return
	}
	groupsByUser, _ := a.cfg.Users.GroupNamesByUser(r.Context())
	writeAdminJSON(w, http.StatusOK, toAdminUserDTO(updated, groupsByUser[updated.ID]))
}

type resetPasswordRequest struct {
	Password string `json:"password"`
}

func (a *adminAPI) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	var req resetPasswordRequest
	if err := decodeAdminJSON(w, r, &req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.")
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeAdminError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memproses password.")
		return
	}
	err = a.cfg.Users.SetPassword(r.Context(), id, hash)
	if errors.Is(err, auth.ErrUserNotFound) {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal mengganti password.")
		return
	}
	a.revokeUsers(r.Context(), []int64{id})
	writeAdminJSON(w, http.StatusOK, map[string]string{"status": "password_updated"})
}

func (a *adminAPI) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		return
	}
	me, _ := auth.UserFromContext(r.Context())
	if id == me.ID {
		writeAdminError(w, http.StatusConflict, "SELF_DELETE_FORBIDDEN", "Tidak dapat menghapus akun sendiri.")
		return
	}
	err := a.cfg.Users.Delete(r.Context(), id)
	switch {
	case errors.Is(err, auth.ErrUserNotFound):
		writeAdminError(w, http.StatusNotFound, "USER_NOT_FOUND", "Pengguna tidak ditemukan.")
	case errors.Is(err, ErrLastAdmin):
		writeAdminError(w, http.StatusConflict, "LAST_ADMIN", err.Error())
	case errors.Is(err, ErrUserHasHistory):
		writeAdminError(w, http.StatusConflict, "USER_HAS_HISTORY", err.Error())
	case err != nil:
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menghapus pengguna.")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
