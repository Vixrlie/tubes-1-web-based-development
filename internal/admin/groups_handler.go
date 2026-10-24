package admin

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

// UserLister menyediakan daftar user untuk pilihan anggota group.
// admin.UserStore memenuhi interface ini.
type UserLister interface {
	List(ctx context.Context) ([]UserRow, error)
}

type groupHandlers struct {
	store   GroupStore
	users   UserLister
	conns   ConnectionLister
	revoker session.Revoker
	view    auth.Renderer
}

// WithGroupAdmin mengaktifkan halaman kelola group. Dipasang dari main.go.
func WithGroupAdmin(
	store GroupStore,
	users UserLister,
	conns ConnectionLister,
	revoker session.Revoker,
	view auth.Renderer,
) Option {
	return func(h *Handlers) {
		h.groups = &groupHandlers{
			store:   store,
			users:   users,
			conns:   conns,
			revoker: revoker,
			view:    view,
		}
	}
}

// groupsRoutes dipasang di bawah /admin dan sudah dibungkus RequireAdmin.
func (h *Handlers) groupsRoutes(r chi.Router) {
	if h.groups == nil {
		return
	}
	g := h.groups
	r.Get("/groups", g.list)
	r.Get("/groups/new", g.newForm)
	r.Post("/groups", g.create)
	r.Get("/groups/{id}", g.detail)
	r.Post("/groups/{id}", g.rename)
	r.Post("/groups/{id}/delete", g.delete)
	r.Post("/groups/{id}/members", g.addMember)
	r.Post("/groups/{id}/members/{userID}/delete", g.removeMember)
	r.Post("/groups/{id}/grants", g.setGrant)
	r.Post("/groups/{id}/grants/{connID}/delete", g.revokeGrant)
}

var groupNotices = map[string]string{
	"created":        "Group berhasil dibuat.",
	"renamed":        "Nama group diperbarui.",
	"deleted":        "Group dihapus.",
	"member_added":   "Anggota ditambahkan dan session-nya dicabut.",
	"member_removed": "Anggota dihapus dan session-nya dicabut.",
	"grant_set":      "Akses group disimpan.",
	"grant_revoked":  "Akses group dicabut.",
}

type groupsPage struct {
	Me        auth.User
	CSRFToken string
	Groups    []Group
	Notice    string
}

type groupFormPage struct {
	Me         auth.User
	CSRFToken  string
	IsNew      bool
	FormAction string
	ID         int64
	Name       string
	Error      string
}

type groupDetailPage struct {
	Me                   auth.User
	CSRFToken            string
	Group                Group
	Members              []UserRow
	AvailableUsers       []UserRow
	Grants               []GrantRow
	AvailableConnections []ConnectionRef
	Notice               string
	Error                string
}

func filterMembers(all []UserRow, members []UserRow) []UserRow {
	taken := make(map[int64]struct{}, len(members))
	for _, u := range members {
		taken[u.ID] = struct{}{}
	}
	out := make([]UserRow, 0, len(all))
	for _, u := range all {
		if _, ok := taken[u.ID]; !ok {
			out = append(out, u)
		}
	}
	return out
}

func filterConnections(all []ConnectionRef, grants []GrantRow) []ConnectionRef {
	taken := make(map[int64]struct{}, len(grants))
	for _, g := range grants {
		taken[g.ConnectionID] = struct{}{}
	}
	out := make([]ConnectionRef, 0, len(all))
	for _, c := range all {
		if _, ok := taken[c.ID]; !ok {
			out = append(out, c)
		}
	}
	return out
}

func (g *groupHandlers) base(r *http.Request) (auth.User, string) {
	me, _ := auth.UserFromContext(r.Context())
	return me, session.CSRFToken(r.Context())
}

func (g *groupHandlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	g.view.Render(w, name, data)
}

func (g *groupHandlers) serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("admin groups: %s: %v", what, err)
	http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
}

func validateGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("nama group wajib diisi")
	}
	if len(name) > 64 {
		return "", errors.New("nama group maksimal 64 karakter")
	}
	return name, nil
}

func (g *groupHandlers) loadGroup(w http.ResponseWriter, r *http.Request) (Group, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return Group{}, false
	}
	grp, err := g.store.GetGroup(r.Context(), id)
	if errors.Is(err, ErrGroupNotFound) {
		http.NotFound(w, r)
		return Group{}, false
	}
	if err != nil {
		g.serverError(w, "ambil group", err)
		return Group{}, false
	}
	return grp, true
}

func (g *groupHandlers) detailData(r *http.Request, grp Group) (groupDetailPage, error) {
	members, err := g.store.ListMembers(r.Context(), grp.ID)
	if err != nil {
		return groupDetailPage{}, err
	}
	allUsers, err := g.users.List(r.Context())
	if err != nil {
		return groupDetailPage{}, err
	}
	grants, err := g.store.ListGrants(r.Context(), grp.ID)
	if err != nil {
		return groupDetailPage{}, err
	}
	conns, err := g.conns.ListConnections(r.Context())
	if err != nil {
		return groupDetailPage{}, err
	}
	me, csrf := g.base(r)
	return groupDetailPage{
		Me:                   me,
		CSRFToken:            csrf,
		Group:                grp,
		Members:              members,
		AvailableUsers:       filterMembers(allUsers, members),
		Grants:               grants,
		AvailableConnections: filterConnections(conns, grants),
	}, nil
}

// renderDetail memuat ulang seluruh data detail group lalu merender halaman.
func (g *groupHandlers) renderDetail(w http.ResponseWriter, r *http.Request, status int, notice, errMsg string) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	page, err := g.detailData(r, grp)
	if err != nil {
		g.serverError(w, "muat detail group", err)
		return
	}
	page.Notice = notice
	page.Error = errMsg
	g.render(w, status, "admin/group_detail.html", page)
}

// revokeUsers mencabut session semua user terdampak perubahan akses.
func (g *groupHandlers) revokeUsers(ctx context.Context, ids []int64) error {
	for _, id := range ids {
		if err := g.revoker.RevokeSessionsByUser(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (g *groupHandlers) revokeGroupMembers(ctx context.Context, groupID int64) error {
	ids, err := g.store.MemberIDs(ctx, groupID)
	if err != nil {
		return err
	}
	return g.revokeUsers(ctx, ids)
}

func redirectGroup(w http.ResponseWriter, r *http.Request, suffix, code string) {
	http.Redirect(w, r, "/admin/groups"+suffix+"?status="+code, http.StatusSeeOther)
}

// ---- daftar ----

func (g *groupHandlers) list(w http.ResponseWriter, r *http.Request) {
	groups, err := g.store.ListGroups(r.Context())
	if err != nil {
		g.serverError(w, "daftar group", err)
		return
	}
	me, csrf := g.base(r)
	g.render(w, http.StatusOK, "admin/groups.html", groupsPage{
		Me:        me,
		CSRFToken: csrf,
		Groups:    groups,
		Notice:    groupNotices[r.URL.Query().Get("status")],
	})
}

// ---- buat ----

func (g *groupHandlers) newForm(w http.ResponseWriter, r *http.Request) {
	me, csrf := g.base(r)
	g.render(w, http.StatusOK, "admin/group_form.html", groupFormPage{
		Me: me, CSRFToken: csrf, IsNew: true, FormAction: "/admin/groups",
	})
}

func (g *groupHandlers) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	name, err := validateGroupName(r.PostFormValue("name"))
	if err != nil {
		g.renderForm(w, r, http.StatusBadRequest, err.Error(), name)
		return
	}

	if _, err := g.store.CreateGroup(r.Context(), name); err != nil {
		if errors.Is(err, ErrGroupNameTaken) {
			g.renderForm(w, r, http.StatusConflict, err.Error(), name)
			return
		}
		g.serverError(w, "buat group", err)
		return
	}
	redirectGroup(w, r, "", "created")
}

func (g *groupHandlers) renderForm(w http.ResponseWriter, r *http.Request, status int, errMsg, name string) {
	me, csrf := g.base(r)
	g.render(w, status, "admin/group_form.html", groupFormPage{
		Me: me, CSRFToken: csrf, IsNew: true, FormAction: "/admin/groups",
		Name: name, Error: errMsg,
	})
}

// ---- detail ----

func (g *groupHandlers) detail(w http.ResponseWriter, r *http.Request) {
	g.renderDetail(w, r, http.StatusOK, groupNotices[r.URL.Query().Get("status")], "")
}

// ---- ubah nama ----

func (g *groupHandlers) rename(w http.ResponseWriter, r *http.Request) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	name, verr := validateGroupName(r.PostFormValue("name"))
	if verr != nil {
		g.renderDetail(w, r, http.StatusBadRequest, "", verr.Error())
		return
	}

	if err := g.store.RenameGroup(r.Context(), grp.ID, name); err != nil {
		if errors.Is(err, ErrGroupNameTaken) {
			g.renderDetail(w, r, http.StatusConflict, "", err.Error())
			return
		}
		g.serverError(w, "ubah nama group", err)
		return
	}
	redirectGroup(w, r, "/"+strconv.FormatInt(grp.ID, 10), "renamed")
}

// ---- hapus group ----

func (g *groupHandlers) delete(w http.ResponseWriter, r *http.Request) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	ids, err := g.store.MemberIDs(r.Context(), grp.ID)
	if err != nil {
		g.serverError(w, "ambil anggota group", err)
		return
	}
	if err := g.store.DeleteGroup(r.Context(), grp.ID); err != nil {
		if errors.Is(err, ErrGroupNotFound) {
			http.NotFound(w, r)
			return
		}
		g.serverError(w, "hapus group", err)
		return
	}
	if err := g.revokeUsers(r.Context(), ids); err != nil {
		g.serverError(w, "cabut session setelah hapus group", err)
		return
	}
	redirectGroup(w, r, "", "deleted")
}

// ---- anggota ----

func (g *groupHandlers) addMember(w http.ResponseWriter, r *http.Request) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	userID, err := strconv.ParseInt(r.PostFormValue("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		g.renderDetail(w, r, http.StatusBadRequest, "", "user tidak valid")
		return
	}

	if err := g.store.AddMember(r.Context(), grp.ID, userID); err != nil {
		switch {
		case errors.Is(err, auth.ErrUserNotFound):
			g.renderDetail(w, r, http.StatusBadRequest, "", "user tidak ditemukan")
		case errors.Is(err, ErrGroupNotFound):
			http.NotFound(w, r)
		default:
			g.serverError(w, "tambah anggota", err)
		}
		return
	}
	if err := g.revokeUsers(r.Context(), []int64{userID}); err != nil {
		g.serverError(w, "cabut session anggota baru", err)
		return
	}
	redirectGroup(w, r, "/"+strconv.FormatInt(grp.ID, 10), "member_added")
}

func (g *groupHandlers) removeMember(w http.ResponseWriter, r *http.Request) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		http.NotFound(w, r)
		return
	}

	if err := g.store.RemoveMember(r.Context(), grp.ID, userID); err != nil {
		if errors.Is(err, ErrMemberNotFound) {
			http.NotFound(w, r)
			return
		}
		g.serverError(w, "hapus anggota", err)
		return
	}
	if err := g.revokeUsers(r.Context(), []int64{userID}); err != nil {
		g.serverError(w, "cabut session setelah hapus anggota", err)
		return
	}
	redirectGroup(w, r, "/"+strconv.FormatInt(grp.ID, 10), "member_removed")
}

// ---- grant ----

func (g *groupHandlers) setGrant(w http.ResponseWriter, r *http.Request) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	connID, err := strconv.ParseInt(r.PostFormValue("connection_id"), 10, 64)
	if err != nil || connID <= 0 {
		g.renderDetail(w, r, http.StatusBadRequest, "", "koneksi tidak valid")
		return
	}
	level := access.Action(r.PostFormValue("level"))
	if level != access.ActionView && level != access.ActionEdit {
		g.renderDetail(w, r, http.StatusBadRequest, "", ErrInvalidLevel.Error())
		return
	}

	old, exists, err := g.store.GetGrant(r.Context(), grp.ID, connID)
	if err != nil {
		g.serverError(w, "ambil grant", err)
		return
	}
	if err := g.store.SetGrant(r.Context(), grp.ID, connID, level); err != nil {
		switch {
		case errors.Is(err, ErrConnectionNotFound), errors.Is(err, ErrGroupNotFound):
			http.NotFound(w, r)
		case errors.Is(err, ErrInvalidLevel):
			g.renderDetail(w, r, http.StatusBadRequest, "", err.Error())
		default:
			g.serverError(w, "simpan grant", err)
		}
		return
	}

	// Menurunkan level atau mencabut akses harus mematikan session lama.
	if exists && access.Rank(level) < access.Rank(old) {
		if err := g.revokeGroupMembers(r.Context(), grp.ID); err != nil {
			g.serverError(w, "cabut session setelah turunkan akses", err)
			return
		}
	}
	redirectGroup(w, r, "/"+strconv.FormatInt(grp.ID, 10), "grant_set")
}

func (g *groupHandlers) revokeGrant(w http.ResponseWriter, r *http.Request) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	connID, err := strconv.ParseInt(chi.URLParam(r, "connID"), 10, 64)
	if err != nil || connID <= 0 {
		http.NotFound(w, r)
		return
	}

	if err := g.store.RevokeGrant(r.Context(), grp.ID, connID); err != nil {
		if errors.Is(err, ErrGrantNotFound) {
			http.NotFound(w, r)
			return
		}
		g.serverError(w, "cabut grant", err)
		return
	}
	if err := g.revokeGroupMembers(r.Context(), grp.ID); err != nil {
		g.serverError(w, "cabut session setelah cabut akses", err)
		return
	}
	redirectGroup(w, r, "/"+strconv.FormatInt(grp.ID, 10), "grant_revoked")
}
