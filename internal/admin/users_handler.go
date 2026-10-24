package admin

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

type userHandlers struct {
	store   UserStore
	revoker session.Revoker
	view    auth.Renderer
}

func WithUserAdmin(store UserStore, revoker session.Revoker, view auth.Renderer) Option {
	return func(h *Handlers) {
		h.users = &userHandlers{store: store, revoker: revoker, view: view}
	}
}

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
	r.Post("/users/{id}/delete", u.remove)
	r.Get("/users/{id}/access", u.accessPage)
}

// Pesan sukses dipilih lewat kode di query string; teks bebas dari URL tidak pernah dirender.
var notices = map[string]string{
	"created":     "User berhasil dibuat.",
	"updated":     "Data user diperbarui.",
	"deactivated": "User dinonaktifkan dan semua session-nya dicabut.",
	"activated":   "User diaktifkan kembali.",
	"password":    "Password direset dan semua session user dicabut.",
	"deleted":     "User dihapus beserta keanggotaan dan session-nya.",
}

// userItem adalah baris daftar user: data akun plus keanggotaan group-nya.
type userItem struct {
	UserRow
	Groups      []string
	IsLastAdmin bool
}

type usersPage struct {
	web.AppShell
	Users  []userItem
	Notice string
}

type userFormPage struct {
	web.AppShell
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

func (u *userHandlers) shell(r *http.Request) web.AppShell {
	me, _ := auth.UserFromContext(r.Context())
	return web.AppShell{
		Me:                   me,
		CSRFToken:            session.CSRFToken(r.Context()),
		Initials:             web.Initials(me.Username),
		ActiveNav:            "users",
		CanManageUsers:       true,
		CanManageConnections: me.IsPlatformAdmin,
	}
}

func (u *userHandlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	u.view.RenderApp(w, name, data)
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

func (u *userHandlers) editPage(sh web.AppShell, t UserRow) userFormPage {
	return userFormPage{
		AppShell:        sh,
		IsSelf:          t.ID == sh.Me.ID,
		FormAction:      "/admin/users/" + strconv.FormatInt(t.ID, 10),
		ID:              t.ID,
		Username:        t.Username,
		FullName:        t.FullName,
		Email:           t.Email,
		IsPlatformAdmin: t.IsPlatformAdmin,
		IsActive:        t.IsActive,
	}
}

func redirectUser(w http.ResponseWriter, r *http.Request, id int64, code string) {
	http.Redirect(w, r, "/admin/users/"+strconv.FormatInt(id, 10)+"?status="+code, http.StatusSeeOther)
}

func (u *userHandlers) list(w http.ResponseWriter, r *http.Request) {
	users, err := u.store.List(r.Context())
	if err != nil {
		u.serverError(w, "daftar user", err)
		return
	}
	groups, err := u.store.GroupNamesByUser(r.Context())
	if err != nil {
		u.serverError(w, "keanggotaan group", err)
		return
	}

	// Tandai admin aktif terakhir — tidak boleh dihapus/dinonaktifkan.
	var activeAdmins int
	for _, t := range users {
		if t.IsPlatformAdmin && t.IsActive {
			activeAdmins++
		}
	}

	items := make([]userItem, 0, len(users))
	for _, t := range users {
		items = append(items, userItem{
			UserRow:     t,
			Groups:      groups[t.ID],
			IsLastAdmin: activeAdmins == 1 && t.IsPlatformAdmin && t.IsActive,
		})
	}

	u.render(w, http.StatusOK, "admin/users.html", usersPage{
		AppShell: u.shell(r),
		Users:    items,
		Notice:   notices[r.URL.Query().Get("status")],
	})
}

func (u *userHandlers) newForm(w http.ResponseWriter, r *http.Request) {
	u.render(w, http.StatusOK, "admin/user_form.html", userFormPage{
		AppShell: u.shell(r), IsNew: true, FormAction: "/admin/users", IsActive: true,
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

	u.render(w, status, "admin/user_form.html", userFormPage{
		AppShell: u.shell(r), IsNew: true, FormAction: "/admin/users", IsActive: true,
		Username: in.Username, FullName: in.FullName, Email: in.Email,
		IsPlatformAdmin: in.IsPlatformAdmin,
		Error:           err.Error(), // pesan sentinel auth, aman ditampilkan; password tidak dirender ulang
	})
}

func (u *userHandlers) editForm(w http.ResponseWriter, r *http.Request) {
	t, ok := u.load(w, r)
	if !ok {
		return
	}
	page := u.editPage(u.shell(r), t)
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
		page := u.editPage(u.shell(r), t)
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

func (u *userHandlers) deactivate(w http.ResponseWriter, r *http.Request) {
	t, ok := u.load(w, r)
	if !ok {
		return
	}
	me, _ := auth.UserFromContext(r.Context())
	if t.ID == me.ID {
		page := u.editPage(u.shell(r), t)
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
		page := u.editPage(u.shell(r), t)
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

// remove menghapus user setelah konfirmasi ketik-nama + checkbox.
// Ditolak untuk admin terakhir, user dengan riwayat migration, dan diri sendiri.
func (u *userHandlers) remove(w http.ResponseWriter, r *http.Request) {
	t, ok := u.load(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	me, _ := auth.UserFromContext(r.Context())

	page := func(msg string) {
		p := u.editPage(u.shell(r), t)
		p.Error = msg
		u.render(w, http.StatusBadRequest, "admin/user_form.html", p)
	}

	if t.ID == me.ID {
		page("Anda tidak bisa menghapus akun sendiri.")
		return
	}
	confirmName := strings.TrimSpace(r.PostFormValue("confirm_name"))
	confirmed := r.PostFormValue("confirm_delete") == "on"
	if confirmName != t.Username || !confirmed {
		page("Konfirmasi tidak cocok. Ketik username persis dan centang persetujuan sebelum menghapus.")
		return
	}

	if err := u.store.Delete(r.Context(), t.ID); err != nil {
		switch {
		case errors.Is(err, ErrLastAdmin):
			page("Ini adalah admin aktif terakhir. Tambahkan admin lain sebelum menghapus akun ini.")
		case errors.Is(err, ErrUserHasHistory):
			page("User ini memiliki riwayat migration dan hanya dapat dinonaktifkan, bukan dihapus.")
		case errors.Is(err, auth.ErrUserNotFound):
			http.NotFound(w, r)
		default:
			u.serverError(w, "hapus user", err)
		}
		return
	}
	http.Redirect(w, r, "/admin/users?status=deleted", http.StatusSeeOther)
}

// dbAccess adalah satu baris hasil gabungan grant untuk sebuah database.
type dbAccess struct {
	ConnectionName string
	Effective      string // "View + Edit" | "View Only" | "Tidak ada akses"
	Source         string // "group → level" untuk hak tertinggi
	Others         []string
	CanEdit        bool
}

// userAccessPage adalah data halaman akses efektif (admin/user_access.html).
type userAccessPage struct {
	web.AppShell
	Target     UserRow
	GroupNames []string
	Rows       []dbAccess
}

// accessPage menampilkan hak akses efektif user: gabungan grant seluruh
// group yang dianggoteinya, per database, dengan sumber hak tertingginya.
func (u *userHandlers) accessPage(w http.ResponseWriter, r *http.Request) {
	t, ok := u.load(w, r)
	if !ok {
		return
	}
	grants, err := u.store.EffectiveAccess(r.Context(), t.ID)
	if err != nil {
		u.serverError(w, "akses efektif", err)
		return
	}

	// Kelompokkan per database; himpun nama group keanggotaan (urut unik).
	byConn := map[int64]*dbAccess{}
	var connOrder []int64
	groupSeen := map[string]bool{}
	var groupNames []string
	for _, g := range grants {
		if !groupSeen[g.GroupName] {
			groupSeen[g.GroupName] = true
			groupNames = append(groupNames, g.GroupName)
		}
		row, exists := byConn[g.ConnectionID]
		if !exists {
			row = &dbAccess{ConnectionName: g.ConnectionName}
			byConn[g.ConnectionID] = row
			connOrder = append(connOrder, g.ConnectionID)
		}
		label := "Tidak ada akses"
		if g.Level == "edit" {
			label = "View + Edit"
		} else if g.Level == "view" {
			label = "View Only"
		}
		entry := g.GroupName + " → " + label
		// Hak tertinggi menang; yang pertama dicatat sebagai sumber.
		if row.Effective == "" || (label == "View + Edit" && row.Effective != "View + Edit") ||
			(label == "View Only" && row.Effective == "Tidak ada akses") {
			row.Effective = label
			row.Source = entry
			row.CanEdit = label == "View + Edit"
			continue
		}
		row.Others = append(row.Others, entry)
	}

	page := userAccessPage{
		AppShell:    u.shell(r),
		Target:      t,
		GroupNames:  groupNames,
	}
	for _, id := range connOrder {
		page.Rows = append(page.Rows, *byConn[id])
	}

	shell := page.AppShell
	shell.Title = "Akses efektif · " + t.Username
	shell.Breadcrumbs = []web.Crumb{
		{Label: "Admin", URL: "/admin/users"},
		{Label: "User", URL: "/admin/users"},
		{Label: t.Username},
	}
	page.AppShell = shell
	u.render(w, http.StatusOK, "admin/user_access.html", page)
}
