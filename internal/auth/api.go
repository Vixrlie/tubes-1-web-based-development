package auth

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/go-chi/chi/v5"
)

// JSONRoutes adds the SPA contract without changing the server-rendered routes.
func (h *Handlers) JSONRoutes(r chi.Router, validator SessionValidator) {
	r.Route("/api/auth", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				next.ServeHTTP(w, r)
			})
		})
		r.Post("/login", func(w http.ResponseWriter, r *http.Request) { h.jsonLogin(w, r, validator) })
		r.Get("/session", func(w http.ResponseWriter, r *http.Request) {
			if info, ok := h.jsonSession(w, r, validator); ok {
				writeSession(w, info)
			}
		})
		r.Post("/logout", func(w http.ResponseWriter, r *http.Request) {
			if h.cop.Check(r) != nil {
				apiError(w, 403, "ORIGIN_REJECTED", "Permintaan lintas situs ditolak.")
				return
			}
			info, ok := h.jsonSession(w, r, validator)
			if !ok {
				return
			}
			if info.CSRFToken == "" || subtle.ConstantTimeCompare([]byte(info.CSRFToken), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
				apiError(w, 403, "CSRF_INVALID", "Token keamanan tidak valid. Muat ulang halaman lalu coba lagi.")
				return
			}
			cookie, _ := r.Cookie(SessionCookieName)
			token := cookie.Value
			if info.NewToken != "" {
				token = info.NewToken
			}
			err := h.sessions.Revoke(r.Context(), token)
			ClearSessionCookie(w, h.secure)
			if err != nil {
				h.logActivity(r, "user.logout", activity.StatusFailed, info.User.Username, err)
				apiError(w, 500, "LOGOUT_FAILED", "Cookie sudah dihapus, tetapi server gagal mencabut sesi. Silakan masuk kembali.")
				return
			}
			h.logActivity(r, "user.logout", activity.StatusSuccess, info.User.Username, nil)
			w.WriteHeader(http.StatusNoContent)
		})
	})
}

func apiError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeSession(w http.ResponseWriter, info SessionInfo) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user":      map[string]any{"id": strconv.FormatInt(info.User.ID, 10), "username": info.User.Username, "isPlatformAdmin": info.User.IsPlatformAdmin},
		"csrfToken": info.CSRFToken,
	})
}

func (h *Handlers) jsonSession(w http.ResponseWriter, r *http.Request, v SessionValidator) (SessionInfo, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		apiError(w, 401, "UNAUTHENTICATED", "Silakan masuk untuk melanjutkan.")
		return SessionInfo{}, false
	}
	info, err := v.Validate(r.Context(), cookie.Value)
	if errors.Is(err, ErrInvalidSession) || (err == nil && !info.User.IsActive) {
		ClearSessionCookie(w, h.secure)
		apiError(w, 401, "SESSION_EXPIRED", "Sesi sudah berakhir. Silakan masuk kembali.")
		return SessionInfo{}, false
	}
	if err != nil {
		apiError(w, 503, "SESSION_UNAVAILABLE", "Sesi belum dapat diperiksa. Coba lagi.")
		return SessionInfo{}, false
	}
	if info.NewToken != "" {
		SetSessionCookie(w, info.NewToken, h.secure)
	}
	return info, true
}

func (h *Handlers) jsonLogin(w http.ResponseWriter, r *http.Request, v SessionValidator) {
	if h.cop.Check(r) != nil {
		apiError(w, 403, "ORIGIN_REJECTED", "Permintaan lintas situs ditolak.")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		apiError(w, 415, "JSON_REQUIRED", "Gunakan Content-Type application/json.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		apiError(w, 400, "INVALID_INPUT", "Permintaan login tidak valid.")
		return
	}
	username := NormalizeUsername(input.Username)
	if username == "" || input.Password == "" {
		apiError(w, 400, "INVALID_INPUT", "Isi username dan password.")
		return
	}
	account, err := h.store.GetByUsername(r.Context(), username)
	if errors.Is(err, ErrUserNotFound) {
		VerifyPassword(dummyHash(), input.Password)
	} else if err != nil {
		h.logActivity(r, "user.login", activity.StatusFailed, username, err)
		apiError(w, 503, "LOGIN_UNAVAILABLE", "Login belum dapat diproses. Coba lagi.")
		return
	} else if VerifyPassword(account.PasswordHash, input.Password) && account.IsActive {
		// Revoke the previous browser session before replacing its cookie.
		if cookie, e := r.Cookie(SessionCookieName); e == nil && cookie.Value != "" {
			if e = h.sessions.Revoke(r.Context(), cookie.Value); e != nil {
				apiError(w, 503, "LOGIN_UNAVAILABLE", "Sesi lama belum dapat ditutup. Coba lagi.")
				return
			}
		}
		token, e := h.sessions.Create(r.Context(), account.ID)
		if e != nil {
			h.logActivity(r, "user.login", activity.StatusFailed, username, e)
			apiError(w, 503, "LOGIN_UNAVAILABLE", "Login belum dapat diproses. Coba lagi.")
			return
		}
		info, e := v.Validate(r.Context(), token)
		if e != nil || !info.User.IsActive || info.User.ID != account.ID || info.CSRFToken == "" {
			_ = h.sessions.Revoke(r.Context(), token)
			apiError(w, 503, "SESSION_UNAVAILABLE", "Sesi belum dapat disiapkan. Coba lagi.")
			return
		}
		if info.NewToken != "" {
			token = info.NewToken
		}
		SetSessionCookie(w, token, h.secure)
		h.logActivity(r, "user.login", activity.StatusSuccess, username, nil)
		writeSession(w, info)
		return
	}
	h.logActivity(r, "user.login", activity.StatusFailed, username, errors.New(msgLoginFailed))
	apiError(w, 401, "INVALID_CREDENTIALS", msgLoginFailed)
}
