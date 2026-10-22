package auth

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

const (
	loginPath    = "/login"
	landingPath  = "/"
	maxLoginBody = 4 << 10 // 4 KiB, cukup untuk username + password

	// Satu pesan untuk semua penyebab gagal login (username tidak ada,
	// password salah, user nonaktif) agar tidak membocorkan keberadaan akun.
	msgLoginFailed = "Username atau password salah."
)

// SessionManager adalah bagian dari session.Manager yang dipakai handler.
type SessionManager interface {
	Create(ctx context.Context, userID int64) (string, error)
	Revoke(ctx context.Context, token string) error
}

// Renderer dipenuhi oleh web.Renderer.
type Renderer interface {
	Render(w http.ResponseWriter, name string, data any)
}

type Handlers struct {
	store    Store
	sessions SessionManager
	view     Renderer
	secure   bool
	cop      *http.CrossOriginProtection
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

// PublicRoutes dipasang DI LUAR AuthMiddleware (belum punya session).
func (h *Handlers) PublicRoutes(r chi.Router) {
	r.Get(loginPath, h.showLogin)
	r.Post(loginPath, h.login)
}

// ProtectedRoutes dipasang di grup AuthMiddleware -> CSRFProtect.
func (h *Handlers) ProtectedRoutes(r chi.Router) {
	r.Get("/", h.home)
	r.Post("/logout", h.logout)
}

type loginPage struct {
	Username string
	Error    string
}

func (h *Handlers) renderLogin(w http.ResponseWriter, status int, username, errMsg string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8") // harus sebelum WriteHeader
	w.WriteHeader(status)
	h.view.Render(w, "login.html", loginPage{Username: username, Error: errMsg})
}

func (h *Handlers) showLogin(w http.ResponseWriter, r *http.Request) {
	h.renderLogin(w, http.StatusOK, "", "")
}

// dummyHash dipakai agar waktu respons untuk username yang tidak ada
// kira-kira sama dengan username yang ada (tanpa bcrypt tambahan).
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
	// POST /login tidak punya token CSRF (belum ada session), jadi dilindungi
	// pengecekan Origin / Sec-Fetch-Site.
	if err := h.cop.Check(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	username := NormalizeUsername(r.PostFormValue("username"))
	password := r.PostFormValue("password")

	acc, err := h.store.GetByUsername(r.Context(), username)
	switch {
	case errors.Is(err, ErrUserNotFound):
		VerifyPassword(dummyHash(), password)
		h.renderLogin(w, http.StatusUnauthorized, username, msgLoginFailed)
		return
	case err != nil:
		log.Printf("login: gagal mengambil user: %v", err) // jangan log password
		http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
		return
	}

	// Cek password dulu, baru status aktif, supaya user nonaktif
	// tidak bisa dibedakan dari password salah.
	if !VerifyPassword(acc.PasswordHash, password) || !acc.IsActive {
		h.renderLogin(w, http.StatusUnauthorized, username, msgLoginFailed)
		return
	}

	// Session fixation: cabut session lama di browser ini (bila ada),
	// lalu buat session baru dengan token baru (SES-09).
	if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
		if err := h.sessions.Revoke(r.Context(), c.Value); err != nil {
			log.Printf("login: gagal mencabut session lama: %v", err)
		}
	}

	token, err := h.sessions.Create(r.Context(), acc.ID)
	if err != nil {
		log.Printf("login: gagal membuat session: %v", err)
		http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
		return
	}

	SetSessionCookie(w, token, h.secure)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, landingPath, http.StatusSeeOther)
}

func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
		if err := h.sessions.Revoke(r.Context(), c.Value); err != nil {
			// Tetap hapus cookie, tapi jangan pura-pura sukses.
			log.Printf("logout: gagal mencabut session: %v", err)
			ClearSessionCookie(w, h.secure)
			http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
			return
		}
	}
	ClearSessionCookie(w, h.secure)
	http.Redirect(w, r, loginPath, http.StatusSeeOther)
}

type homePage struct {
	Username  string
	CSRFToken string
}

// home adalah halaman sementara sebagai tujuan setelah login dan tempat
// tombol logout. Ganti dengan /databases saat fitur viewer selesai.
func (h *Handlers) home(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context())
	h.view.Render(w, "home.html", homePage{
		Username:  u.Username,
		CSRFToken: session.CSRFToken(r.Context()),
	})
}
