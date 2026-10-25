package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

// SessionCookieName tetap; hanya nilainya yang berubah saat rotasi.
const SessionCookieName = "pathgres_session"

// ErrInvalidSession dikembalikan SessionValidator untuk session yang tidak ada,
// kedaluwarsa (idle atau absolute), sudah dicabut, atau token lama di luar grace period.
var ErrInvalidSession = errors.New("session tidak valid")

type User struct {
	ID              int64
	Username        string
	IsPlatformAdmin bool
	IsActive        bool
}

type contextKey struct{}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, contextKey{}, u)
}

// UserFromContext memberikan user yang sedang login (identifikasi principal).
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(contextKey{}).(User)
	return u, ok
}

// SessionInfo adalah hasil validasi session.
type SessionInfo struct {
	User      User
	CSRFToken string
	// NewToken terisi kalau session baru saja dirotasi (rotasi berkala).
	// Middleware akan menulis ulang cookie dengan nilai ini.
	NewToken string
}

// SessionValidator diimplementasikan session manager. Validate menerima token lama
// selama grace period, menolak revoked & timeout, menyegarkan last_seen_at, merotasi token.
type SessionValidator interface {
	Validate(ctx context.Context, token string) (SessionInfo, error)
}

// SetSessionCookie menulis cookie session. Dipakai juga oleh handler login.
func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie menghapus cookie session di browser.
func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// AuthMiddleware menolak request tanpa session valid (redirect ke /login).
// Kalau valid, user dan token CSRF disimpan di context.
func AuthMiddleware(v SessionValidator, secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(SessionCookieName)
			if err != nil || c.Value == "" {
				redirectToLogin(w, r)
				return
			}

			info, err := v.Validate(r.Context(), c.Value)
			if err != nil {
				if errors.Is(err, ErrInvalidSession) {
					ClearSessionCookie(w, secure)
					redirectToLogin(w, r)
					return
				}
				// Jangan log token atau isi cookie.
				log.Printf("validasi session gagal: %v", err)
				http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
				return
			}

			if !info.User.IsActive {
				ClearSessionCookie(w, secure)
				redirectToLogin(w, r)
				return
			}

			if info.NewToken != "" {
				SetSessionCookie(w, info.NewToken, secure)
			}
			w.Header().Set("Cache-Control", "no-store")

			ctx := WithUser(r.Context(), info.User)
			ctx = session.WithCSRFToken(ctx, info.CSRFToken)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin menolak non-admin dengan 403. Pasang SETELAH AuthMiddleware.
// Ini hanya cek peran Platform Admin, bukan hak akses ke database.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFromContext(r.Context())
		if !ok || !u.IsPlatformAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CSRFProtect memvalidasi token CSRF untuk request pengubah state.
// Token dibaca dari header X-CSRF-Token (htmx) atau field form csrf_token.
// Pasang SETELAH AuthMiddleware (butuh token dari context).
func CSRFProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		want := session.CSRFToken(r.Context())
		got := r.Header.Get("X-CSRF-Token")
		if got == "" {
			got = r.PostFormValue("csrf_token")
		}
		if want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
