package admin

// Form group peserta: tambah (/admin/groups/new). Ubah nama (rename) tetap
// dari halaman detail group; izin akses diatur lewat access-editor di detail.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// groupFormPage = data templates/admin/group_form.html fe-components.
type groupFormPage struct {
	web.Shell
	Title      string
	Crumbs     []web.Crumb
	FormAction string
	Error      string
	Fields     []map[string]any
	Actions    []web.Button
}

// newGroupFormPage membangun halaman form "Grup Baru" (errMsg untuk galat validasi).
func (g *groupHandlers) newGroupFormPage(r *http.Request, errMsg string) groupFormPage {
	return groupFormPage{
		Shell: g.shell(r),
		Title: "Grup Baru",
		Crumbs: []web.Crumb{
			{Label: "Admin", Href: "/admin/users"},
			{Label: "Kelola Grup", Href: "/admin/groups"},
			{Label: "Baru"},
		},
		FormAction: "/admin/groups",
		Error:      errMsg,
		Fields: []map[string]any{
			{"Label": "Nama grup", "Name": "name", "Placeholder": "analis",
				"Helper": "Harus unik di antara grup lain.", "Required": true},
		},
		Actions: []web.Button{
			{Label: "Simpan", Type: "submit"},
			{Label: "Batal", Variant: "secondary", Href: "/admin/groups"},
		},
	}
}

func (g *groupHandlers) createForm(w http.ResponseWriter, r *http.Request) {
	g.render(w, http.StatusOK, "admin/group_form.html", g.newGroupFormPage(r, ""))
}

func (g *groupHandlers) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		g.render(w, http.StatusBadRequest, "admin/group_form.html",
			g.newGroupFormPage(r, "Nama grup wajib diisi."))
		return
	}
	grp, err := g.store.CreateGroup(r.Context(), name)
	if err != nil {
		if errors.Is(err, ErrGroupNameTaken) {
			g.render(w, http.StatusConflict, "admin/group_form.html",
				g.newGroupFormPage(r, "Nama grup sudah dipakai."))
			return
		}
		g.serverError(w, "buat group", err)
		return
	}
	http.Redirect(w, r, "/admin/groups/"+strconv.FormatInt(grp.ID, 10), http.StatusSeeOther)
}
