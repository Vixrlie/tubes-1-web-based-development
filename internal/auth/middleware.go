package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

const SessionCookieName = "pathgres_session"

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

func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(contextKey{}).(User)
	return u, ok
}

type SessionInfo struct {
	User      User
	CSRFToken string
	NewToken  string
}

type SessionValidator interface {
	Validate(ctx context.Context, token string) (SessionInfo, error)
}

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
			ctx = activity.WithActor(ctx, info.User.ID)
			ctx = session.WithCSRFToken(ctx, info.CSRFToken)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

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
	// Klien JSON API tidak mengikuti redirect HTML; beri 401 agar SPA bisa
	// mengarahkan user ke halaman login sendiri.
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"sesi tidak valid atau sudah berakhir","code":"unauthenticated"}`))
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
