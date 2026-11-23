package admin

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

const maxAdminJSONBytes = 1 << 20 // 1 MiB

// AdminAPIConfig mengumpulkan dependency untuk endpoint JSON /api/admin/*.
type AdminAPIConfig struct {
	Users        UserStore
	Groups       GroupStore
	Connections  ConnectionStore
	ConnLister   ConnectionLister
	Revoker      session.Revoker
	Evictor      PoolEvictor
	Validator    auth.SessionValidator
	CookieSecure bool
	Logger       *activity.Logger
}

type adminAPI struct {
	cfg AdminAPIConfig
}

// RegisterAdminJSONRoutes mendaftarkan endpoint JSON /api/admin/* (terpisah dari
// handler HTML lama) dengan autentikasi cookie JSON, pengecekan admin, dan CSRF.
func RegisterAdminJSONRoutes(r chi.Router, cfg AdminAPIConfig) {
	api := &adminAPI{cfg: cfg}

	r.Route("/api/admin", func(r chi.Router) {
		r.Use(api.authAndCSRFMiddleware)
		r.Use(AdminActivityMiddleware(cfg.Logger))

		r.Get("/dashboard", api.dashboard)

		if cfg.Users != nil {
			r.Get("/users", api.listUsers)
			r.Post("/users", api.createUser)
			r.Get("/users/{id}", api.getUser)
			r.Patch("/users/{id}", api.updateUser)
			r.Post("/users/{id}/activate", api.activateUser)
			r.Post("/users/{id}/deactivate", api.deactivateUser)
			r.Post("/users/{id}/password", api.resetUserPassword)
			r.Delete("/users/{id}", api.deleteUser)
		}

		if cfg.Groups != nil {
			r.Get("/groups", api.listGroups)
			r.Post("/groups", api.createGroup)
			r.Get("/groups/{id}", api.getGroup)
			r.Patch("/groups/{id}", api.updateGroup)
			r.Delete("/groups/{id}", api.deleteGroup)
			r.Post("/groups/{id}/members", api.addGroupMembers)
			r.Delete("/groups/{id}/members/{userID}", api.removeGroupMember)
			r.Put("/groups/{id}/access", api.updateGroupAccess)
		}
	})
}

func (a *adminAPI) authAndCSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if a.cfg.Validator == nil {
			writeAdminError(w, http.StatusServiceUnavailable, "SESSION_UNAVAILABLE", "Sesi belum dapat diperiksa. Coba lagi.")
			return
		}
		cookie, err := r.Cookie(auth.SessionCookieName)
		if err != nil || cookie.Value == "" {
			writeAdminError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Silakan masuk untuk melanjutkan.")
			return
		}
		info, err := a.cfg.Validator.Validate(r.Context(), cookie.Value)
		if errors.Is(err, auth.ErrInvalidSession) || (err == nil && (!info.User.IsActive || info.User.ID <= 0)) {
			auth.ClearSessionCookie(w, a.cfg.CookieSecure)
			writeAdminError(w, http.StatusUnauthorized, "SESSION_EXPIRED", "Sesi sudah berakhir. Silakan masuk kembali.")
			return
		}
		if err != nil {
			writeAdminError(w, http.StatusServiceUnavailable, "SESSION_UNAVAILABLE", "Sesi belum dapat diperiksa. Coba lagi.")
			return
		}
		if info.NewToken != "" {
			auth.SetSessionCookie(w, info.NewToken, a.cfg.CookieSecure)
		}
		if !info.User.IsPlatformAdmin {
			writeAdminError(w, http.StatusForbidden, "ADMIN_REQUIRED", "Halaman ini hanya dapat diakses administrator platform.")
			return
		}

		if isMutatingMethod(r.Method) {
			got := r.Header.Get("X-CSRF-Token")
			if info.CSRFToken == "" || subtle.ConstantTimeCompare([]byte(got), []byte(info.CSRFToken)) != 1 {
				writeAdminError(w, http.StatusForbidden, "CSRF_INVALID", "Token keamanan tidak valid. Muat ulang halaman lalu coba lagi.")
				return
			}
		}

		ctx := auth.WithUser(r.Context(), info.User)
		ctx = session.WithCSRFToken(ctx, info.CSRFToken)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func writeAdminJSON(w http.ResponseWriter, status int, payload any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAdminError(w http.ResponseWriter, status int, code, message string) {
	writeAdminJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func writeAdminValidationError(w http.ResponseWriter, message string, fields map[string]string) {
	errObj := map[string]any{
		"code":    "VALIDATION_ERROR",
		"message": message,
	}
	if len(fields) > 0 {
		errObj["fields"] = fields
	}
	writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": errObj})
}

func decodeAdminJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Body == nil {
		return io.EOF
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAdminJSONBytes)
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}

func parseParamID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func formatID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func (a *adminAPI) revokeUsers(ctx context.Context, ids []int64) {
	if a.cfg.Revoker == nil {
		return
	}
	me, _ := auth.UserFromContext(ctx)
	for _, id := range ids {
		if me.ID != 0 && id == me.ID {
			continue
		}
		_ = a.cfg.Revoker.RevokeSessionsByUser(ctx, id)
	}
}

func (a *adminAPI) forceRotateUsers(ctx context.Context, ids []int64) {
	if a.cfg.Revoker == nil {
		return
	}
	for _, id := range ids {
		_ = a.cfg.Revoker.ForceRotateSessionsByUser(ctx, id)
	}
}

func (a *adminAPI) evictPool(connID int64) {
	if a.cfg.Evictor != nil {
		a.cfg.Evictor.Evict(connID)
	}
}

// --- Dashboard ---

type adminDashboardDTO struct {
	TotalUsers       int `json:"totalUsers"`
	ActiveUsers      int `json:"activeUsers"`
	TotalGroups      int `json:"totalGroups"`
	TotalConnections int `json:"totalConnections"`
}

func (a *adminAPI) dashboard(w http.ResponseWriter, r *http.Request) {
	var dto adminDashboardDTO
	if a.cfg.Users != nil {
		users, err := a.cfg.Users.List(r.Context())
		if err != nil {
			writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat ringkasan pengguna.")
			return
		}
		dto.TotalUsers = len(users)
		for _, u := range users {
			if u.IsActive {
				dto.ActiveUsers++
			}
		}
	}
	if a.cfg.Groups != nil {
		groups, err := a.cfg.Groups.ListGroups(r.Context())
		if err != nil {
			writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat ringkasan grup.")
			return
		}
		dto.TotalGroups = len(groups)
	}
	if a.cfg.ConnLister != nil {
		conns, err := a.cfg.ConnLister.ListConnections(r.Context())
		if err != nil {
			writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat ringkasan koneksi.")
			return
		}
		dto.TotalConnections = len(conns)
	} else if a.cfg.Connections != nil {
		conns, err := a.cfg.Connections.List(r.Context())
		if err != nil {
			writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memuat ringkasan koneksi.")
			return
		}
		dto.TotalConnections = len(conns)
	}
	writeAdminJSON(w, http.StatusOK, dto)
}
