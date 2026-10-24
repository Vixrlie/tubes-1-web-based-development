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
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

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
	r.Post("/groups/{id}/access", g.saveAccess)
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

// Tampilan daftar group mengikuti format fe-components
// (templates/admin/groups.html + web.GroupsPage).
type groupsListView struct {
	web.AppShell
	PageHeader web.PageHeader
	Groups     web.Table
}

// Tampilan detail group mengikuti format fe-components
// (templates/admin/group_detail.html + web.GroupDetailPage).
type groupDetailView struct {
	web.AppShell
	PageHeader   web.PageHeader
	DeleteAction string
	Members      web.ListCard
	Access       web.ListCard
	Editor       *web.AccessEditor
	AddMembers   *web.AddMembersModal
}

type groupFormPage struct {
	web.AppShell
	IsNew      bool
	FormAction string
	ID         int64
	Name       string
	Error      string
}

// renderDetail memuat ulang detail group lama (dipanggil jalur galat rename/grant).
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

func (g *groupHandlers) shell(r *http.Request, title string) web.AppShell {
	me, _ := auth.UserFromContext(r.Context())
	return web.AppShell{
		Title:                title,
		Me:                   me,
		CSRFToken:            session.CSRFToken(r.Context()),
		Initials:             web.Initials(me.Username),
		ActiveNav:            "groups",
		CanManageUsers:       true,
		CanManageConnections: me.IsPlatformAdmin,
	}
}

func (g *groupHandlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	g.view.RenderApp(w, name, data)
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

// buildDetailView membangun halaman detail group sesuai format fe-components:
// kartu Anggota (+ pop up tambah anggota) dan kartu Akses Database (mode lihat)
// atau access-editor (mode ubah, ?edit=1).
func (g *groupHandlers) buildDetailView(r *http.Request, grp Group, editMode bool) (groupDetailView, error) {
	members, err := g.store.ListMembers(r.Context(), grp.ID)
	if err != nil {
		return groupDetailView{}, err
	}
	allUsers, err := g.users.List(r.Context())
	if err != nil {
		return groupDetailView{}, err
	}
	grants, err := g.store.ListGrants(r.Context(), grp.ID)
	if err != nil {
		return groupDetailView{}, err
	}
	conns, err := g.conns.ListConnections(r.Context())
	if err != nil {
		return groupDetailView{}, err
	}

	gid := strconv.FormatInt(grp.ID, 10)
	base := "/admin/groups/" + gid
	token := session.CSRFToken(r.Context())

	// Kartu anggota: daftar + pop up "Tambah Anggota" (kandidat = user non-anggota).
	webMembers := make([]web.Member, 0, len(members))
	memberIDs := make([]string, 0, len(members))
	for _, m := range members {
		webMembers = append(webMembers, web.Member{Username: m.Username, FullName: m.FullName})
		memberIDs = append(memberIDs, strconv.FormatInt(m.ID, 10))
	}
	removeAction := func(i int) string {
		return base + "/members/" + memberIDs[i] + "/delete"
	}
	candidates := make([]web.MemberCandidate, 0)
	for _, u := range filterMembers(allUsers, members) {
		candidates = append(candidates, web.MemberCandidate{
			ID:       strconv.FormatInt(u.ID, 10),
			Username: u.Username,
			FullName: u.FullName,
			Inactive: !u.IsActive,
		})
	}

	view := groupDetailView{
		AppShell:    g.shell(r, "Grup "+grp.Name),
		PageHeader:  web.GroupDetailHeader(grp.Name, "/admin/groups"),
		DeleteAction: base + "/delete",
		Members:     web.NewMembersCard(webMembers, base+"/members", removeAction),
		AddMembers:  &web.AddMembersModal{FormAction: base + "/members", Candidates: candidates},
	}
	view.Members.CSRFToken = token
	view.AddMembers.CSRFToken = token

	if editMode {
		rows := make([]web.AccessRow, 0, len(grants))
		for _, gr := range grants {
			rows = append(rows, web.AccessRow{
				ID:    strconv.FormatInt(gr.ConnectionID, 10),
				Name:  gr.ConnectionName,
				Level: string(gr.Level),
			})
		}
		available := make([]web.DropdownOption, 0)
		for _, c := range filterConnections(conns, grants) {
			available = append(available, web.DropdownOption{Value: strconv.FormatInt(c.ID, 10), Label: c.Name})
		}
		view.Editor = &web.AccessEditor{
			FormAction: base + "/access",
			Rows:       rows,
			Available:  available,
			CancelHref: base,
		}
		view.Editor.CSRFToken = token
	} else {
		items := make([]web.ListItem, 0, len(grants))
		for _, gr := range grants {
			items = append(items, web.ListItem{
				Title: gr.ConnectionName,
				Tag:   web.AccessLabel(string(gr.Level)),
			})
		}
		view.Access = web.NewAccessCard(items, base+"?edit=1")
	}
	return view, nil
}
// renderDetail memuat ulang detail group setelah galat (jalur rename/grant lama).
// Template fe-components tidak punya slot pesan; status HTTP tetap dikirim.
func (g *groupHandlers) renderDetail(w http.ResponseWriter, r *http.Request, status int, notice, errMsg string) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	page, err := g.buildDetailView(r, grp, r.URL.Query().Get("edit") == "1")
	if err != nil {
		g.serverError(w, "muat detail group", err)
		return
	}
	g.render(w, status, "admin/group_detail.html", page)
}
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

// list menampilkan tabel kelola group format fe-components (web.GroupsTable).
func (g *groupHandlers) list(w http.ResponseWriter, r *http.Request) {
	groups, err := g.store.ListGroups(r.Context())
	if err != nil {
		g.serverError(w, "daftar group", err)
		return
	}
	accesses, err := g.store.GroupAccesses(r.Context())
	if err != nil {
		g.serverError(w, "ringkasan akses", err)
		return
	}
	summaries := make([]web.GroupSummary, 0, len(groups))
	for _, grp := range groups {
		summaries = append(summaries, web.GroupSummary{
			Name:        grp.Name,
			Href:        "/admin/groups/" + strconv.FormatInt(grp.ID, 10),
			MemberCount: grp.MemberCount,
			Access:      accesses[grp.ID],
		})
	}
	shell := g.shell(r, "Kelola Grup")
	// breadcrumb dirender oleh komponen page-header di dalam konten.
	shell.Breadcrumbs = nil
	g.render(w, http.StatusOK, "admin/groups.html", groupsListView{
		AppShell:  shell,
		PageHeader: web.GroupsHeader("/admin/groups/new"),
		Groups:    web.NewGroupsTable(summaries),
	})
}
func (g *groupHandlers) newForm(w http.ResponseWriter, r *http.Request) {
	g.render(w, http.StatusOK, "admin/group_form.html", groupFormPage{
		AppShell:   g.shell(r, "Tambah Group"),
		IsNew:      true,
		FormAction: "/admin/groups",
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
	g.render(w, status, "admin/group_form.html", groupFormPage{
		AppShell:   g.shell(r, "Tambah Group"),
		IsNew:      true,
		FormAction: "/admin/groups",
		Name:       name,
		Error:      errMsg,
	})
}

func (g *groupHandlers) detail(w http.ResponseWriter, r *http.Request) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	page, err := g.buildDetailView(r, grp, r.URL.Query().Get("edit") == "1")
	if err != nil {
		g.serverError(w, "muat detail group", err)
		return
	}
	g.render(w, http.StatusOK, "admin/group_detail.html", page)
}
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

// addMember menerima satu user_id (form lama) atau banyak user_id berulang
// (pop up "Tambah Anggota" fe-components).
func (g *groupHandlers) addMember(w http.ResponseWriter, r *http.Request) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	rawIDs := r.PostForm["user_id"]
	if len(rawIDs) == 0 {
		g.renderDetail(w, r, http.StatusBadRequest, "", "user tidak valid")
		return
	}
	added := make([]int64, 0, len(rawIDs))
	for _, raw := range rawIDs {
		userID, err := strconv.ParseInt(raw, 10, 64)
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
		added = append(added, userID)
	}
	if err := g.revokeUsers(r.Context(), added); err != nil {
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

// saveAccess menerima perubahan batch dari access-editor (format fe-components).
// Field form: access:<connID> (level baru), remove (ID yang dicabut, boleh
// berulang), serta pasangan add_db/add_access untuk penambahan baru.
// Penurunan level atau pencabutan mencabut session semua anggota group.
func (g *groupHandlers) saveAccess(w http.ResponseWriter, r *http.Request) {
	grp, ok := g.loadGroup(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	validLevel := func(l access.Action) bool {
		return l == access.ActionView || l == access.ActionEdit
	}
	// apply menulis grant dan menandai bila perlu mencabut session anggota.
	apply := func(connID int64, level access.Action) (bool, error) {
		old, existed, err := g.store.GetGrant(r.Context(), grp.ID, connID)
		if err != nil {
			return false, err
		}
		if existed && old == level {
			return false, nil
		}
		if err := g.store.SetGrant(r.Context(), grp.ID, connID, level); err != nil {
			return false, err
		}
		return existed && access.Rank(level) < access.Rank(old), nil
	}

	needsRevoke := false

	// 1. Database yang dicabut.
	for _, raw := range r.PostForm["remove"] {
		connID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || connID <= 0 {
			continue
		}
		if _, existed, err := g.store.GetGrant(r.Context(), grp.ID, connID); err != nil {
			g.serverError(w, "ambil grant", err)
			return
		} else if existed {
			if err := g.store.RevokeGrant(r.Context(), grp.ID, connID); err != nil {
				g.serverError(w, "cabut grant", err)
				return
			}
			needsRevoke = true
		}
	}

	// 2. Level database lama: field "access:<ID>".
	for key, vals := range r.PostForm {
		if !strings.HasPrefix(key, "access:") || len(vals) == 0 {
			continue
		}
		connID, err := strconv.ParseInt(strings.TrimPrefix(key, "access:"), 10, 64)
		if err != nil || connID <= 0 {
			continue
		}
		level := access.Action(vals[0])
		if !validLevel(level) {
			g.renderDetail(w, r, http.StatusBadRequest, "", ErrInvalidLevel.Error())
			return
		}
		downgraded, err := apply(connID, level)
		if errors.Is(err, ErrConnectionNotFound) || errors.Is(err, ErrGroupNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			g.serverError(w, "simpan grant", err)
			return
		}
		if downgraded {
			needsRevoke = true
		}
	}

	// 3. Penambahan baru: add_db[i] + add_access[i] sejajar ("" = placeholder).
	dbs, levels := r.PostForm["add_db"], r.PostForm["add_access"]
	for i := 0; i < len(dbs) && i < len(levels); i++ {
		if dbs[i] == "" {
			continue
		}
		connID, err := strconv.ParseInt(dbs[i], 10, 64)
		if err != nil || connID <= 0 {
			g.renderDetail(w, r, http.StatusBadRequest, "", "database tidak valid")
			return
		}
		level := access.Action(levels[i])
		if !validLevel(level) {
			g.renderDetail(w, r, http.StatusBadRequest, "", ErrInvalidLevel.Error())
			return
		}
		downgraded, err := apply(connID, level)
		if errors.Is(err, ErrConnectionNotFound) || errors.Is(err, ErrGroupNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			g.serverError(w, "simpan grant", err)
			return
		}
		if downgraded {
			needsRevoke = true
		}
	}

	if needsRevoke {
		if err := g.revokeGroupMembers(r.Context(), grp.ID); err != nil {
			g.serverError(w, "cabut session anggota", err)
			return
		}
	}
	redirectGroup(w, r, "/"+strconv.FormatInt(grp.ID, 10), "grant_set")
}
