package admin

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

type userHandlers struct {
	store   UserStore
	revoker session.Revoker
	view    auth.Renderer
}

// WithUserAdmin mengaktifkan halaman kelola user. Dipasang dari main.go.
func WithUserAdmin(store UserStore, revoker session.Revoker, view auth.Renderer) Option {
	return func(h *Handlers) {
		h.users = &userHandlers{store: store, revoker: revoker, view: view}
	}
}

// Dipasang di bawah /admin dan sudah dibungkus RequireAdmin.
func (h *Handlers) usersRoutes(r chi.Router) {
	if h.users == nil {
		return
	}
	u := h.users
	r.Get("/users", u.list)
	r.Get("/users/new", u.newForm)
	r.Post("/users", u.create)
	r.Get("/users/{id}", u.editForm)
	r.Post("/users/{id}", u.update)
	r.Post("/users/{id}/deactivate", u.deactivate)
	r.Post("/users/{id}/activate", u.activate)
	r.Post("/users/{id}/password", u.resetPassword)
	r.Post("/users/{id}/delete", u.delete)
}

// Pesan sukses dipilih lewat kode di query string; teks bebas dari URL tidak pernah dirender.
var notices = map[string]string{
	"created":     "User berhasil dibuat.",
	"updated":     "Data user diperbarui.",
	"deactivated": "User dinonaktifkan dan semua session-nya dicabut.",
	"activated":   "User diaktifkan kembali.",
	"password":    "Password direset dan semua session user dicabut.",
	"deleted":     "User dihapus permanen beserta keanggotaannya.",
}

type usersPage struct {
	User       *web.ShellUser // layout FE: header + sidebar admin
	Nav        string
	SideActive string
	Title      string
	Crumbs     []web.Crumb
	Actions    []web.Button
	Table      web.Table
	Notice     string
}

type userFormPage struct {
	Me              auth.User
	CSRFToken       string
	User            *web.ShellUser // layout FE: header + sidebar admin
	Nav             string
	SideActive      string
	IsNew           bool
	IsSelf          bool
	FormAction      string
	ID              int64
	Username        string
	FullName        string
	Email           string
	IsPlatformAdmin bool
	IsActive        bool
	Error           string
	Notice          string
}

func (u *userHandlers) base(r *http.Request) (auth.User, string) {
	me, _ := auth.UserFromContext(r.Context())
	return me, session.CSRFToken(r.Context())
}

func (u *userHandlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	u.view.Render(w, name, data)
}

func (u *userHandlers) serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("admin users: %s: %v", what, err) // jangan log password
	http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
}

func isInputError(err error) bool {
	return errors.Is(err, auth.ErrUsernameInvalid) ||
		errors.Is(err, auth.ErrPasswordTooShort) ||
		errors.Is(err, auth.ErrPasswordTooLong)
}

func isConflict(err error) bool {
	return errors.Is(err, auth.ErrUsernameTaken) || errors.Is(err, auth.ErrEmailTaken)
}

// load membaca {id} dari URL lalu mengambil user. Sudah menulis respons bila gagal.
func (u *userHandlers) load(w http.ResponseWriter, r *http.Request) (UserRow, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return UserRow{}, false
	}
	t, err := u.store.Get(r.Context(), id)
	if errors.Is(err, auth.ErrUserNotFound) {
		http.NotFound(w, r)
		return UserRow{}, false
	}
	if err != nil {
		u.serverError(w, "ambil user", err)
		return UserRow{}, false
	}
	return t, true
}

func (u *userHandlers) editPage(r *http.Request, t UserRow) userFormPage {
	me, csrf := u.base(r)
	return userFormPage{
		Me:              me,
		CSRFToken:       csrf,
		User:            shellUserData(me, csrf),
		Nav:             "admin",
		SideActive:      "users",
		IsSelf:          t.ID == me.ID,
		FormAction:      "/admin/users/" + strconv.FormatInt(t.ID, 10),
		ID:              t.ID,
		Username:        t.Username,
		FullName:        t.FullName,
		Email:           t.Email,
		IsPlatformAdmin: t.IsPlatformAdmin,
		IsActive:        t.IsActive,
	}
}

// shellUserData mengisi data shell layout FE (header + sidebar admin).
func shellUserData(me auth.User, csrf string) *web.ShellUser {
	return &web.ShellUser{Username: me.Username, IsPlatformAdmin: me.IsPlatformAdmin, CSRFToken: csrf}
}

func redirectUser(w http.ResponseWriter, r *http.Request, id int64, code string) {
	http.Redirect(w, r, "/admin/users/"+strconv.FormatInt(id, 10)+"?status="+code, http.StatusSeeOther)
}

// Daftar

func (u *userHandlers) list(w http.ResponseWriter, r *http.Request) {
	users, err := u.store.List(r.Context())
	if err != nil {
		u.serverError(w, "daftar user", err)
		return
	}
	groups, err := u.store.GroupNamesByUser(r.Context())
	if err != nil {
		u.serverError(w, "grup per user", err)
		return
	}
	me, csrf := u.base(r)

	table := web.Table{
		Title: "Akun platform",
		Tag:   web.TableCell{Text: strconv.Itoa(len(users)) + " item"},
		Columns: []web.TableColumn{
			{Key: "username", Label: "Username", Bold: true},
			{Key: "status", Label: "Status / peran platform"},
			{Key: "group", Label: "Group"},
			{Key: "aksi", Label: "Aksi"},
		},
		Empty: "Belum ada user.",
	}
	for _, usr := range users {
		status := "Nonaktif"
		if usr.IsActive {
			status = "Aktif"
		}
		role := "Non-admin"
		if usr.IsPlatformAdmin {
			role = "Admin"
		}
		groupList := strings.Join(groups[usr.ID], ", ")
		if groupList == "" {
			groupList = "—"
		}
		table.Rows = append(table.Rows, web.TableRow{
			"username": {Text: usr.Username},
			"status":   {Text: status + " · " + role},
			"group":    {Text: groupList},
			"aksi":     {Text: "Kelola", Href: "/admin/users/" + strconv.FormatInt(usr.ID, 10)},
		})
	}

	u.render(w, http.StatusOK, "admin/users.html", usersPage{
		User:       shellUserData(me, csrf),
		Nav:        "admin",
		SideActive: "users",
		Title:      "Kelola User",
		Crumbs: []web.Crumb{
			{Label: "Admin", Href: "/admin"},
			{Label: "Daftar User"},
		},
		Actions: []web.Button{
			{Label: "Tambah User", IconLeft: "plus", Href: "/admin/users/new"},
		},
		Table:  table,
		Notice: notices[r.URL.Query().Get("status")],
	})
}

// Buat

func (u *userHandlers) newForm(w http.ResponseWriter, r *http.Request) {
	me, csrf := u.base(r)
	u.render(w, http.StatusOK, "admin/user_form.html", userFormPage{
		Me: me, CSRFToken: csrf, User: shellUserData(me, csrf), Nav: "admin", SideActive: "users", IsNew: true, FormAction: "/admin/users", IsActive: true,
	})
}

func (u *userHandlers) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	in := auth.NewUser{
		Username:        r.PostFormValue("username"),
		FullName:        strings.TrimSpace(r.PostFormValue("full_name")),
		Email:           strings.TrimSpace(r.PostFormValue("email")),
		Password:        r.PostFormValue("password"),
		IsPlatformAdmin: r.PostFormValue("is_platform_admin") == "on",
	}

	_, err := auth.CreateUser(r.Context(), u.store, in)
	if err == nil {
		http.Redirect(w, r, "/admin/users?status=created", http.StatusSeeOther)
		return
	}

	status := http.StatusInternalServerError
	switch {
	case isInputError(err):
		status = http.StatusBadRequest
	case isConflict(err):
		status = http.StatusConflict
	default:
		u.serverError(w, "buat user", err)
		return
	}

	me, csrf := u.base(r)
	u.render(w, status, "admin/user_form.html", userFormPage{
		Me: me, CSRFToken: csrf, User: shellUserData(me, csrf), Nav: "admin", SideActive: "users", IsNew: true, FormAction: "/admin/users", IsActive: true,
		Username: in.Username, FullName: in.FullName, Email: in.Email,
		IsPlatformAdmin: in.IsPlatformAdmin,
		Error:           err.Error(), // pesan sentinel auth, aman ditampilkan; password tidak dirender ulang
	})
}

// Ubah

func (u *userHandlers) editForm(w http.ResponseWriter, r *http.Request) {
	t, ok := u.load(w, r)
	if !ok {
		return
	}
	page := u.editPage(r, t)
	page.Notice = notices[r.URL.Query().Get("status")]
	u.render(w, http.StatusOK, "admin/user_form.html", page)
}

func (u *userHandlers) update(w http.ResponseWriter, r *http.Request) {
	t, ok := u.load(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	me, _ := auth.UserFromContext(r.Context())

	upd := UserUpdate{
		FullName:        strings.TrimSpace(r.PostFormValue("full_name")),
		Email:           strings.TrimSpace(r.PostFormValue("email")),
		IsPlatformAdmin: r.PostFormValue("is_platform_admin") == "on",
	}
	// Admin tidak boleh mengubah peran dirinya sendiri (mencegah terkunci tanpa admin).
	if t.ID == me.ID {
		upd.IsPlatformAdmin = t.IsPlatformAdmin
	}

	err := u.store.Update(r.Context(), t.ID, upd)
	if isConflict(err) {
		page := u.editPage(r, t)
		page.FullName, page.Email = upd.FullName, upd.Email
		page.Error = err.Error()
		u.render(w, http.StatusConflict, "admin/user_form.html", page)
		return
	}
	if err != nil {
		u.serverError(w, "ubah user", err)
		return
	}

	// Peran berubah (naik atau turun): session lama tidak boleh membawa hak lama.
	if upd.IsPlatformAdmin != t.IsPlatformAdmin {
		if err := u.revoker.RevokeSessionsByUser(r.Context(), t.ID); err != nil {
			u.serverError(w, "cabut session setelah perubahan peran", err)
			return
		}
	}
	redirectUser(w, r, t.ID, "updated")
}

// Nonaktifkan / aktifkan

func (u *userHandlers) deactivate(w http.ResponseWriter, r *http.Request) {
	t, ok := u.load(w, r)
	if !ok {
		return
	}
	me, _ := auth.UserFromContext(r.Context())
	if t.ID == me.ID {
		page := u.editPage(r, t)
		page.Error = "Anda tidak bisa menonaktifkan akun sendiri."
		u.render(w, http.StatusBadRequest, "admin/user_form.html", page)
		return
	}

	if err := u.store.SetActive(r.Context(), t.ID, false); err != nil {
		u.serverError(w, "nonaktifkan user", err)
		return
	}
	if err := u.revoker.RevokeSessionsByUser(r.Context(), t.ID); err != nil {
		u.serverError(w, "cabut session setelah nonaktif", err)
		return
	}
	redirectUser(w, r, t.ID, "deactivated")
}

func (u *userHandlers) activate(w http.ResponseWriter, r *http.Request) {
	t, ok := u.load(w, r)
	if !ok {
		return
	}
	if err := u.store.SetActive(r.Context(), t.ID, true); err != nil {
		u.serverError(w, "aktifkan user", err)
		return
	}
	redirectUser(w, r, t.ID, "activated")
}

// delete menghapus user permanen. Guard dari store: admin aktif terakhir dan
// user ber-riwayat migration ditolak (muncul sebagai pesan error di form).
func (u *userHandlers) delete(w http.ResponseWriter, r *http.Request) {
	t, ok := u.load(w, r)
	if !ok {
		return
	}
	me, _ := auth.UserFromContext(r.Context())
	if t.ID == me.ID {
		page := u.editPage(r, t)
		page.Error = "Anda tidak bisa menghapus akun sendiri."
		u.render(w, http.StatusBadRequest, "admin/user_form.html", page)
		return
	}
	if err := u.store.Delete(r.Context(), t.ID); err != nil {
		switch {
		case errors.Is(err, ErrLastAdmin), errors.Is(err, ErrUserHasHistory):
			page := u.editPage(r, t)
			page.Error = err.Error()
			u.render(w, http.StatusBadRequest, "admin/user_form.html", page)
		case errors.Is(err, auth.ErrUserNotFound):
			http.NotFound(w, r)
		default:
			u.serverError(w, "hapus user", err)
		}
		return
	}
	// Keanggotaan & session terhapus cascade oleh store.
	http.Redirect(w, r, "/admin/users?status=deleted", http.StatusSeeOther)
}

// Reset password

func (u *userHandlers) resetPassword(w http.ResponseWriter, r *http.Request) {
	t, ok := u.load(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	pw := r.PostFormValue("new_password")
	if err := auth.ValidatePassword(pw); err != nil {
		page := u.editPage(r, t)
		page.Error = err.Error()
		u.render(w, http.StatusBadRequest, "admin/user_form.html", page)
		return
	}

	hash, err := auth.HashPassword(pw)
	if err != nil {
		u.serverError(w, "hash password", err)
		return
	}
	if err := u.store.SetPassword(r.Context(), t.ID, hash); err != nil {
		u.serverError(w, "simpan password", err)
		return
	}
	if err := u.revoker.RevokeSessionsByUser(r.Context(), t.ID); err != nil {
		u.serverError(w, "cabut session setelah reset password", err)
		return
	}
	redirectUser(w, r, t.ID, "password")
}
