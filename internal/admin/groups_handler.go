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

type groupHandlers struct {
	store GroupStore
	view  auth.Renderer
}

func WithGroupAdmin(store GroupStore, view auth.Renderer) Option {
	return func(h *Handlers) {
		h.groups = &groupHandlers{store: store, view: view}
	}
}

func (h *Handlers) groupsRoutes(r chi.Router) {
	if h.groups == nil {
		return
	}
	g := h.groups
	r.Get("/groups", g.list)
	r.Post("/groups", g.create)
	r.Post("/groups/{id}", g.updateName)
	r.Post("/groups/{id}/delete", g.delete)
	r.Post("/groups/{groupID}/members/{userID}/delete", g.removeMember)
}

type groupsPage struct {
	web.AppShell
	Groups   []GroupRow
	Selected *GroupDetail
	Notice   string
	Error    string
}

func (g *groupHandlers) shell(r *http.Request) web.AppShell {
	me, _ := auth.UserFromContext(r.Context())
	return web.AppShell{
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

var groupNotices = map[string]string{
	"created": "Group berhasil dibuat.",
	"updated": "Nama group diperbarui.",
	"deleted": "Group dihapus beserta keanggotaan dan aksesnya.",
}

func (g *groupHandlers) list(w http.ResponseWriter, r *http.Request) {
	groups, err := g.store.ListGroups(r.Context())
	if err != nil {
		g.serverError(w, "daftar group", err)
		return
	}
	page := groupsPage{
		AppShell: g.shell(r),
		Groups:   groups,
		Notice:   groupNotices[r.URL.Query().Get("status")],
	}
	page.Breadcrumbs = []web.Crumb{
		{Label: "Admin", URL: "/admin/users"},
		{Label: "Group"},
		{Label: "Daftar"},
	}
	if groupID := r.URL.Query().Get("group"); groupID != "" {
		id, err := strconv.ParseInt(groupID, 10, 64)
		if err != nil || id <= 0 {
			http.NotFound(w, r)
			return
		}
		detail, err := g.store.ListGroupDetail(r.Context(), id)
		if err != nil {
			g.serverError(w, "detail group", err)
			return
		}
		if detail == nil {
			http.NotFound(w, r)
			return
		}
		page.Selected = detail
	}
	g.render(w, http.StatusOK, "admin/groups.html", page)
}

func (g *groupHandlers) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		g.render(w, http.StatusBadRequest, "admin/groups.html", groupsPage{
			AppShell: g.shell(r),
			Error:    "Nama group belum diisi.",
		})
		return
	}
	if _, err := g.store.CreateGroup(r.Context(), name); err != nil {
		if errors.Is(err, ErrGroupTaken) {
			g.render(w, http.StatusConflict, "admin/groups.html", groupsPage{
				AppShell: g.shell(r),
				Error:    "Nama group sudah digunakan.",
			})
			return
		}
		g.serverError(w, "buat group", err)
		return
	}
	http.Redirect(w, r, "/admin/groups?status=created", http.StatusSeeOther)
}

func (g *groupHandlers) updateName(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		g.render(w, http.StatusBadRequest, "admin/groups.html", groupsPage{
			AppShell: g.shell(r),
			Error:    "Nama group belum diisi.",
		})
		return
	}
	if err := g.store.UpdateGroupName(r.Context(), id, name); err != nil {
		if errors.Is(err, ErrGroupTaken) {
			g.render(w, http.StatusConflict, "admin/groups.html", groupsPage{
				AppShell: g.shell(r),
				Error:    "Nama group sudah digunakan.",
			})
			return
		}
		g.serverError(w, "ubah nama group", err)
		return
	}
	http.Redirect(w, r, "/admin/groups?group="+strconv.FormatInt(id, 10)+"&status=updated", http.StatusSeeOther)
}

func (g *groupHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	if err := g.store.DeleteGroup(r.Context(), id); err != nil {
		g.serverError(w, "hapus group", err)
		return
	}
	http.Redirect(w, r, "/admin/groups?status=deleted", http.StatusSeeOther)
}

func (g *groupHandlers) removeMember(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseInt(chi.URLParam(r, "groupID"), 10, 64)
	if err != nil || groupID <= 0 {
		http.NotFound(w, r)
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	if err := g.store.RemoveMember(r.Context(), groupID, userID); err != nil {
		g.serverError(w, "hapus anggota", err)
		return
	}
	http.Redirect(w, r, "/admin/groups?group="+strconv.FormatInt(groupID, 10), http.StatusSeeOther)
}
