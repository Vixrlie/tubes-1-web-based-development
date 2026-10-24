package admin

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// accessHandlers mengelola matriks akses group × database (/admin/access).
type accessHandlers struct {
	store   GroupStore
	conns   ConnectionLister
	revoker session.Revoker
	view    auth.Renderer
}

// WithAccessAdmin mengaktifkan halaman matriks akses group.
func WithAccessAdmin(store GroupStore, conns ConnectionLister, revoker session.Revoker, view auth.Renderer) Option {
	return func(h *Handlers) {
		h.access = &accessHandlers{store: store, conns: conns, revoker: revoker, view: view}
	}
}

func (h *Handlers) accessRoutes(r chi.Router) {
	if h.access == nil {
		return
	}
	a := h.access
	r.Get("/access", a.matrix)
	r.Post("/access", a.save)
}

var accessNotices = map[string]string{
	"saved": "Akses disimpan. Konfigurasi group berhasil diperbarui.",
}

// accessNoticesPage adalah data matriks akses (admin/access.html).
type accessPage struct {
	web.AppShell
	Groups []Group
	Conns  []ConnectionRef
	// Levels[groupID][connID] = "" | "view" | "edit"
	Levels map[int64]map[int64]string
	Notice string
	Error  string
}

func (a *accessHandlers) shell(r *http.Request, title string) web.AppShell {
	me, _ := auth.UserFromContext(r.Context())
	shell := web.AppShell{
		Title:                title,
		Me:                   me,
		CSRFToken:            session.CSRFToken(r.Context()),
		Initials:             web.Initials(me.Username),
		ActiveNav:            "access",
		CanManageUsers:       true,
		CanManageConnections: me.IsPlatformAdmin,
	}
	shell.Breadcrumbs = []web.Crumb{
		{Label: "Admin", URL: "/admin/users"},
		{Label: "Akses"},
		{Label: "Matriks"},
	}
	return shell
}

func (a *accessHandlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	a.view.RenderApp(w, name, data)
}

func (a *accessHandlers) serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("admin access: %s: %v", what, err)
	http.Error(w, "terjadi kesalahan", http.StatusInternalServerError)
}

// buildPage memuat groups + connections + level yang tersimpan.
func (a *accessHandlers) buildPage(r *http.Request) (accessPage, error) {
	groups, err := a.store.ListGroups(r.Context())
	if err != nil {
		return accessPage{}, err
	}
	conns, err := a.conns.ListConnections(r.Context())
	if err != nil {
		return accessPage{}, err
	}
	grants, err := a.store.AllGrants(r.Context())
	if err != nil {
		return accessPage{}, err
	}

	levels := map[int64]map[int64]string{}
	// Isi penuh semua sel (group × database) supaya template selalu menemukan
	// nilai, meski group belum punya grant sama sekali.
	for _, g := range groups {
		levels[g.ID] = map[int64]string{}
		for _, c := range conns {
			levels[g.ID][c.ID] = ""
		}
	}
	for groupID, perConn := range grants {
		if levels[groupID] == nil {
			levels[groupID] = map[int64]string{}
		}
		for connID, level := range perConn {
			levels[groupID][connID] = string(level)
		}
	}
	return accessPage{
		AppShell: a.shell(r, "Akses Group"),
		Groups:   groups,
		Conns:    conns,
		Levels:   levels,
	}, nil
}

func (a *accessHandlers) matrix(w http.ResponseWriter, r *http.Request) {
	page, err := a.buildPage(r)
	if err != nil {
		a.serverError(w, "matriks akses", err)
		return
	}
	page.Notice = accessNotices[r.URL.Query().Get("status")]
	a.render(w, http.StatusOK, "admin/access.html", page)
}

// save menerapkan seluruh sel matriks dari satu formulir:
// field "level_{group}_{conn}" bernilai "", "view", atau "edit".
// Penurunan level atau pencabutan mencabut session semua anggota group.
func (a *accessHandlers) save(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	page, err := a.buildPage(r)
	if err != nil {
		a.serverError(w, "matriks akses", err)
		return
	}

	invalid := false
	revokedGroups := map[int64]bool{}
	for _, g := range page.Groups {
		for _, c := range page.Conns {
			key := "level_" + strconv.FormatInt(g.ID, 10) + "_" + strconv.FormatInt(c.ID, 10)
			newLevel := access.Action(r.PostFormValue(key))
			if newLevel != "" && newLevel != access.ActionView && newLevel != access.ActionEdit {
				invalid = true
				continue
			}

			old, exists, err := a.store.GetGrant(r.Context(), g.ID, c.ID)
			if err != nil {
				a.serverError(w, "ambil grant", err)
				return
			}

			if newLevel == "" {
				if !exists {
					continue
				}
				if err := a.store.RevokeGrant(r.Context(), g.ID, c.ID); err != nil {
					a.serverError(w, "cabut grant", err)
					return
				}
				revokedGroups[g.ID] = true
				continue
			}

			if exists && old == newLevel {
				continue
			}
			if err := a.store.SetGrant(r.Context(), g.ID, c.ID, newLevel); err != nil {
				if errors.Is(err, ErrConnectionNotFound) || errors.Is(err, ErrGroupNotFound) {
					http.NotFound(w, r)
					return
				}
				a.serverError(w, "simpan grant", err)
				return
			}
			if exists && access.Rank(newLevel) < access.Rank(old) {
				revokedGroups[g.ID] = true
			}
		}
	}

	// Cabut session anggota group yang aksesnya berubah turun atau dicabut.
	for groupID := range revokedGroups {
		ids, err := a.store.MemberIDs(r.Context(), groupID)
		if err != nil {
			a.serverError(w, "ambil anggota group", err)
			return
		}
		for _, id := range ids {
			if err := a.revoker.RevokeSessionsByUser(r.Context(), id); err != nil {
				a.serverError(w, "cabut session anggota", err)
				return
			}
		}
	}

	if invalid {
		page.Error = "Level akses tidak valid. Pilih Tidak ada akses, View Only, atau View + Edit."
		// Muat ulang level tersimpan supaya sel yang salah tidak menipu.
		fresh, err := a.buildPage(r)
		if err == nil {
			fresh.Error = page.Error
			a.render(w, http.StatusBadRequest, "admin/access.html", fresh)
			return
		}
	}
	http.Redirect(w, r, "/admin/access?status=saved", http.StatusSeeOther)
}
