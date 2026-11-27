package auth

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

const (
	loginPath      = "/login"
	landingPath    = "/databases"
	maxLoginBody   = 4 << 10
	msgLoginFailed = "Username atau password salah."
)

type SessionManager interface {
	Create(ctx context.Context, userID int64) (string, error)
	Revoke(ctx context.Context, token string) error
}

type Renderer interface {
	Render(w http.ResponseWriter, name string, data any)
}

type Handlers struct {
	store    Store
	sessions SessionManager
	view     Renderer
	secure   bool
	cop      *http.CrossOriginProtection
	logger   *activity.Logger // opsional, nil = logging dimatikan
}

func NewHandlers(store Store, sessions SessionManager, view Renderer, secureCookie bool) *Handlers {
	return &Handlers{
		store:    store,
		sessions: sessions,
		view:     view,
		secure:   secureCookie,
		cop:      http.NewCrossOriginProtection(),
	}
}

func (h *Handlers) SetLogger(l *activity.Logger) { h.logger = l }

func (h *Handlers) PublicRoutes(r chi.Router) {
	r.Get(loginPath, h.showLogin)
	r.Post(loginPath, h.login)
}

func (h *Handlers) ProtectedRoutes(r chi.Router) {
	r.Get("/", h.home)
	r.Post("/logout", h.logout)
}

type loginPage struct {
	Title    string
	Username string
	Error    string

	User any
	Nav  string
}

func (h *Handlers) renderLogin(w http.ResponseWriter, status int, username, errMsg string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	h.view.Render(w, "login.html", loginPage{Title: "Masuk", Username: username, Error: errMsg})
}

func (h *Handlers) showLogin(w http.ResponseWriter, r *http.Request) {
	h.renderLogin(w, http.StatusOK, "", "")
}

var (
	dummyOnce sync.Once
	dummyVal  string
)

func dummyHash() string {
	dummyOnce.Do(func() {
		dummyVal, _ = HashPassword("dummy-password-untuk-timing")
	})
	return dummyVal
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	if err := h.cop.Check(r); err != nil {
		h.logActivity(r, "user.login", activity.StatusFailed, "", err)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
	if err := r.ParseForm(); err != nil {
		h.logActivity(r, "user.login", activity.StatusFailed, "", err)
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	username := NormalizeUsername(r.PostFormValue("username"))
	password := r.PostFormValue("password")

	acc, err := h.store.GetByUsername(r.Context(), username)
	switch {
	case errors.Is(err, ErrUserNotFound):
		VerifyPassword(dummyHash(), password)
		h.logActivity(r, "user.login", activity.StatusFailed, username, errors.New(msgLoginFailed))
		h.renderLogin(w, http.StatusUnauthorized, username, msgLoginFailed)
		return
	case err != nil:
		log.Printf("login: gagal mengambil user: %v", err) // jangan log password
		h.logActivity(r, "user.login", activity.StatusFailed, username, err)
		http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
		return
	}

	if !VerifyPassword(acc.PasswordHash, password) || !acc.IsActive {
		h.logActivity(r, "user.login", activity.StatusFailed, username, errors.New(msgLoginFailed))
		h.renderLogin(w, http.StatusUnauthorized, username, msgLoginFailed)
		return
	}

	if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
		if err := h.sessions.Revoke(r.Context(), c.Value); err != nil {
			log.Printf("login: gagal mencabut session lama: %v", err)
		}
	}

	token, err := h.sessions.Create(r.Context(), acc.ID)
	if err != nil {
		log.Printf("login: gagal membuat session: %v", err)
		h.logActivity(r, "user.login", activity.StatusFailed, username, err)
		http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
		return
	}

	h.logActivity(r, "user.login", activity.StatusSuccess, username, nil)
	SetSessionCookie(w, token, h.secure)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, landingPath, http.StatusSeeOther)
}

func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	logoutUser := "anonymous"
	if u, ok := UserFromContext(r.Context()); ok && u.Username != "" {
		logoutUser = u.Username
	}

	if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
		if err := h.sessions.Revoke(r.Context(), c.Value); err != nil {
			// Tetap hapus cookie, tapi jangan pura-pura sukses.
			log.Printf("logout: gagal mencabut session: %v", err)
			h.logActivity(r, "user.logout", activity.StatusFailed, logoutUser, err)
			ClearSessionCookie(w, h.secure)
			http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
			return
		}
	}
	h.logActivity(r, "user.logout", activity.StatusSuccess, logoutUser, nil)
	ClearSessionCookie(w, h.secure)
	http.Redirect(w, r, loginPath, http.StatusSeeOther)
}

func (h *Handlers) logActivity(r *http.Request, action, status, user string, err error) {
	if h.logger == nil {
		return
	}
	e := activity.Event{
		UserID: user,
		Action: action,
		Status: status,
		Detail: map[string]string{"method": r.Method, "path": r.URL.Path},
	}
	if err != nil {
		e.Err = err
	}
	h.logger.Log(r.Context(), e)
}

type homePage struct {
	Title           string
	Username        string
	CSRFToken       string
	IsPlatformAdmin bool

	User any
	Nav  string
}

func (h *Handlers) home(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context())
	csrf := session.CSRFToken(r.Context())
	h.view.Render(w, "home.html", homePage{
		Title:           "Beranda",
		Username:        u.Username,
		CSRFToken:       csrf,
		IsPlatformAdmin: u.IsPlatformAdmin,
		User: &web.ShellUser{
			Username:        u.Username,
			IsPlatformAdmin: u.IsPlatformAdmin,
			CSRFToken:       csrf,
		},
		Nav: "databases",
	})
}
