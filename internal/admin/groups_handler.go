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

// UserLister menyediakan daftar user untuk kandidat anggota group.
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

// WithGroupAdmin mengaktifkan halaman kelola group (/admin/groups).
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
	// Form "Grup Baru": tampilan (GET) + simpan (POST).
	r.Get("/groups/new", g.createForm)
	r.Post("/groups", g.create)
	r.Get("/groups/{id}", g.detail)
	r.Post("/groups/{id}/delete", g.delete)
	r.Post("/groups/{id}/members", g.addMember)
	r.Post("/groups/{id}/members/{userID}/delete", g.removeMember)
	r.Post("/groups/{id}/access", g.saveAccess)
}

// shell membangun data shell FE (layout.html: header + sidebar admin).
func (g *groupHandlers) shell(r *http.Request) web.Shell {
	me, _ := auth.UserFromContext(r.Context())
	return web.Shell{
		Nav:        "admin",
		SideActive: "groups",
		User: &web.ShellUser{
			Username:        me.Username,
			IsPlatformAdmin: me.IsPlatformAdmin,
			CSRFToken:       session.CSRFToken(r.Context()),
		},
	}
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

// buildDetailView membangun halaman detail group format fe-components:
// kartu Anggota (+ pop up tambah anggota) dan kartu Akses Database (mode lihat)
// atau access-editor (mode ubah, ?edit=1).
func (g *groupHandlers) buildDetailView(r *http.Request, grp Group, editMode bool) (web.GroupDetailPage, error) {
	members, err := g.store.ListMembers(r.Context(), grp.ID)
	if err != nil {
		return web.GroupDetailPage{}, err
	}
	allUsers, err := g.users.List(r.Context())
	if err != nil {
		return web.GroupDetailPage{}, err
	}
	grants, err := g.store.ListGrants(r.Context(), grp.ID)
	if err != nil {
		return web.GroupDetailPage{}, err
	}
	conns, err := g.conns.ListConnections(r.Context())
	if err != nil {
		return web.GroupDetailPage{}, err
	}

	gid := strconv.FormatInt(grp.ID, 10)
	base := "/admin/groups/" + gid

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

	page := web.GroupDetailPage{
		Shell:        g.shell(r),
		GroupName:    grp.Name,
		PageHeader:   web.GroupDetailHeader(grp.Name, "/admin/groups"),
		DeleteAction: base + "/delete",
		Members:      web.NewMembersCard(webMembers, base+"/members", removeAction),
		AddMembers:   &web.AddMembersModal{FormAction: base + "/members", Candidates: candidates},
	}

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
		page.Editor = &web.AccessEditor{
			FormAction: base + "/access",
			Rows:       rows,
			Available:  available,
			CancelHref: base,
		}
	} else {
		items := make([]web.ListItem, 0, len(grants))
		for _, gr := range grants {
			items = append(items, web.ListItem{
				Title: gr.ConnectionName,
				Tag:   web.AccessLabel(string(gr.Level)),
			})
		}
		page.Access = web.NewAccessCard(items, base+"?edit=1")
	}
	page.SetCSRF(session.CSRFToken(r.Context()))
	return page, nil
}

// renderDetail memuat ulang detail group (jalur galat pesan lama).
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

// revokeUsers mencabut session semua user terdampak perubahan akses.
// User yang sedang melakukan aksi dilewati supaya admin tidak ikut ter-logout
// saat mengubah grup yang di dalamnya ia sendiri menjadi anggota.
func (g *groupHandlers) revokeUsers(ctx context.Context, ids []int64) error {
	me, _ := auth.UserFromContext(ctx)
	for _, id := range ids {
		if me.ID != 0 && id == me.ID {
			continue
		}
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

// list menampilkan tabel kelola group format fe-components.
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
	g.render(w, http.StatusOK, "admin/groups.html", web.GroupsPage{
		Shell:      g.shell(r),
		PageHeader: web.GroupsHeader("/admin/groups/new"),
		Groups:     web.NewGroupsTable(summaries),
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
	http.Redirect(w, r, "/admin/groups", http.StatusSeeOther)
}

// addMember menerima satu user_id (form lama) atau banyak user_id berulang
// (pop up "Tambah Anggota" format fe-components).
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
	http.Redirect(w, r, "/admin/groups/"+strconv.FormatInt(grp.ID, 10), http.StatusSeeOther)
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
	http.Redirect(w, r, "/admin/groups/"+strconv.FormatInt(grp.ID, 10), http.StatusSeeOther)
}

// saveAccess memproses batch access-editor: access:<connID> (level baru), remove
// (boleh berulang), pasangan add_db/add_access. Penurunan/pencabutan mencabut session anggota.
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
	http.Redirect(w, r, "/admin/groups/"+strconv.FormatInt(grp.ID, 10), http.StatusSeeOther)
}
