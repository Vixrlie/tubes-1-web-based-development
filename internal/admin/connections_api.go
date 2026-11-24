package admin

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

var credRedactPattern = regexp.MustCompile(`(?i)((?:postgres(?:ql)?|mysql)://[^:\s/@]+:)[^@\s]+(@)`)

func sanitizeTestErr(err error) string {
	if err == nil {
		return ""
	}
	msg := credRedactPattern.ReplaceAllString(err.Error(), "${1}***${2}")
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return msg
}

type adminConnectionGroupDTO struct {
	Name        string `json:"name"`
	MemberCount int    `json:"memberCount"`
	Level       string `json:"level"`
}

type adminConnectionDTO struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	DBMS        string                    `json:"dbms"`
	Host        string                    `json:"host"`
	Port        string                    `json:"port"`
	Database    string                    `json:"database"`
	Username    string                    `json:"username"`
	Groups      []adminConnectionGroupDTO `json:"groups"`
	CreatedAt   string                    `json:"createdAt"`
}

func toAdminConnectionDTO(c ConnectionRow, groups []adminConnectionGroupDTO) adminConnectionDTO {
	if groups == nil {
		groups = []adminConnectionGroupDTO{}
	}
	dbms := c.DBMS
	if dbms == "" {
		dbms = "postgres"
	}
	return adminConnectionDTO{
		ID:          formatID(c.ID),
		Name:        c.Name,
		Description: c.Description,
		DBMS:        dbms,
		Host:        c.Host,
		Port:        c.Port,
		Database:    c.Database,
		Username:    c.Username,
		Groups:      groups,
		CreatedAt:   c.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (a *adminAPI) listConnections(w http.ResponseWriter, r *http.Request) {
	rows, err := a.cfg.Connections.List(r.Context())
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat daftar koneksi.")
		return
	}
	groupsMap, _ := a.cfg.Connections.ConnectionGroups(r.Context())
	items := make([]adminConnectionDTO, 0, len(rows))
	for _, row := range rows {
		var grps []adminConnectionGroupDTO
		for _, g := range groupsMap[row.ID] {
			grps = append(grps, adminConnectionGroupDTO{
				Name:        g.Name,
				MemberCount: g.MemberCount,
				Level:       g.Level,
			})
		}
		items = append(items, toAdminConnectionDTO(row, grps))
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

type connectionPayload struct {
	ID          string `json:"id,omitempty"`
	DBMS        string `json:"dbms"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Host        string `json:"host"`
	Port        string `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

func (p connectionPayload) toInput() ConnectionInput {
	dbms := strings.ToLower(strings.TrimSpace(p.DBMS))
	if dbms != "mysql" {
		dbms = "postgres"
	}
	port := strings.TrimSpace(p.Port)
	if port == "" {
		if dbms == "mysql" {
			port = "3306"
		} else {
			port = "5432"
		}
	}
	return ConnectionInput{
		DBMS:        dbms,
		Name:        strings.TrimSpace(p.Name),
		Description: strings.TrimSpace(p.Description),
		Host:        strings.TrimSpace(p.Host),
		Port:        port,
		Database:    strings.TrimSpace(p.Database),
		Username:    strings.TrimSpace(p.Username),
		Password:    p.Password,
	}
}

func (a *adminAPI) createConnection(w http.ResponseWriter, r *http.Request) {
	var req connectionPayload
	if err := decodeAdminJSON(w, r, &req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.")
		return
	}
	in := req.toInput()
	if errs := ValidateConnection(in, true); len(errs) > 0 {
		writeAdminValidationError(w, "Periksa kembali data koneksi yang diisi.", errs)
		return
	}
	me, _ := auth.UserFromContext(r.Context())
	row, err := a.cfg.Connections.Create(r.Context(), in, me.ID)
	if errors.Is(err, ErrConnectionNameTaken) {
		writeAdminError(w, http.StatusConflict, "CONNECTION_NAME_TAKEN", "Nama koneksi sudah dipakai.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menyimpan koneksi.")
		return
	}
	writeAdminJSON(w, http.StatusCreated, toAdminConnectionDTO(row, nil))
}

func (a *adminAPI) getConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Koneksi database tidak ditemukan.")
		return
	}
	row, err := a.cfg.Connections.Get(r.Context(), id)
	if errors.Is(err, ErrConnectionNotFound) {
		writeAdminError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Koneksi database tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat detail koneksi.")
		return
	}
	groupsMap, _ := a.cfg.Connections.ConnectionGroups(r.Context())
	var grps []adminConnectionGroupDTO
	for _, g := range groupsMap[row.ID] {
		grps = append(grps, adminConnectionGroupDTO{
			Name:        g.Name,
			MemberCount: g.MemberCount,
			Level:       g.Level,
		})
	}
	writeAdminJSON(w, http.StatusOK, toAdminConnectionDTO(row, grps))
}

func (a *adminAPI) updateConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Koneksi database tidak ditemukan.")
		return
	}
	var req connectionPayload
	if err := decodeAdminJSON(w, r, &req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.")
		return
	}
	in := req.toInput()
	if errs := ValidateConnection(in, false); len(errs) > 0 {
		writeAdminValidationError(w, "Periksa kembali data koneksi yang diisi.", errs)
		return
	}
	row, err := a.cfg.Connections.Update(r.Context(), id, in)
	switch {
	case errors.Is(err, ErrConnectionNotFound):
		writeAdminError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Koneksi database tidak ditemukan.")
		return
	case errors.Is(err, ErrConnectionNameTaken):
		writeAdminError(w, http.StatusConflict, "CONNECTION_NAME_TAKEN", "Nama koneksi sudah dipakai.")
		return
	case err != nil:
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memperbarui koneksi.")
		return
	}
	a.evictPool(id)
	writeAdminJSON(w, http.StatusOK, toAdminConnectionDTO(row, nil))
}

func (a *adminAPI) deleteConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Koneksi database tidak ditemukan.")
		return
	}
	affected, _ := a.cfg.Connections.UsersWithAccess(r.Context(), id)
	err := a.cfg.Connections.Delete(r.Context(), id)
	if errors.Is(err, ErrConnectionNotFound) {
		writeAdminError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Koneksi database tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menghapus koneksi.")
		return
	}
	a.evictPool(id)
	a.revokeUsers(r.Context(), affected)
	w.WriteHeader(http.StatusNoContent)
}

func (a *adminAPI) testConnectionInput(w http.ResponseWriter, r *http.Request) {
	var req connectionPayload
	if err := decodeAdminJSON(w, r, &req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.")
		return
	}
	var existingID int64
	if strings.TrimSpace(req.ID) != "" {
		existingID, _ = strconv.ParseInt(strings.TrimSpace(req.ID), 10, 64)
	}
	in := req.toInput()
	if in.Name == "" {
		in.Name = "test"
	}
	if errs := ValidateConnection(in, existingID <= 0); len(errs) > 0 {
		writeAdminValidationError(w, "Lengkapi parameter koneksi sebelum menguji.", errs)
		return
	}
	start := time.Now()
	err := a.cfg.Connections.TestInput(r.Context(), existingID, in)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		writeAdminJSON(w, http.StatusOK, map[string]any{
			"ok":        false,
			"latencyMs": latencyMs,
			"message":   sanitizeTestErr(err),
		})
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"latencyMs": latencyMs,
		"message":   "Koneksi berhasil diuji.",
	})
}

func (a *adminAPI) testSavedConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := parseParamID(r, "id")
	if !ok {
		writeAdminError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Koneksi database tidak ditemukan.")
		return
	}
	start := time.Now()
	err := a.cfg.Connections.Test(r.Context(), id)
	latencyMs := time.Since(start).Milliseconds()
	if errors.Is(err, ErrConnectionNotFound) {
		writeAdminError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Koneksi database tidak ditemukan.")
		return
	}
	if err != nil {
		writeAdminJSON(w, http.StatusOK, map[string]any{
			"ok":        false,
			"latencyMs": latencyMs,
			"message":   sanitizeTestErr(err),
		})
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"latencyMs": latencyMs,
		"message":   "Koneksi berhasil diuji.",
	})
}
