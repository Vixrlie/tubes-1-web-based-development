package admin

import (
	"html"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// Halaman group memakai format fe-components (web.GroupDetailPage /
// web.GroupsPage); tes memastikan kedua mode detail ter-render utuh.

func detailViewFixture(editMode bool) groupDetailView {
	membersCard := web.NewMembersCard(
		[]web.Member{{Username: "member", FullName: "Anggota Satu"}},
		"/admin/groups/1/members",
		func(i int) string { return "/admin/groups/1/members/1/delete" },
	)
	membersCard.CSRFToken = "token"
	v := groupDetailView{
		AppShell: web.AppShell{
			Title:     "Grup developers",
			Me:        fixtureAdmin(),
			CSRFToken: "token",
			Initials:  "A",
			ActiveNav: "groups",
		},
		PageHeader:   web.GroupDetailHeader("developers", "/admin/groups"),
		DeleteAction: "/admin/groups/1/delete",
		Members:      membersCard,
		AddMembers: &web.AddMembersModal{
			FormAction: "/admin/groups/1/members",
			CSRFToken:  "token",
			Candidates: []web.MemberCandidate{{ID: "2", Username: "candidate"}},
		},
	}
	if editMode {
		v.Editor = &web.AccessEditor{
			FormAction: "/admin/groups/1/access",
			CSRFToken:  "token",
			Rows:       []web.AccessRow{{ID: "5", Name: "sample", Level: web.AccessEdit}},
			Available:  []web.DropdownOption{{Value: "6", Label: "candidate-db"}},
			CancelHref: "/admin/groups/1",
		}
	} else {
		v.Access = web.NewAccessCard([]web.ListItem{{
			Title: "sample",
			Tag:   web.AccessLabel(web.AccessEdit),
		}}, "/admin/groups/1?edit=1")
	}
	return v
}

func TestGroupDetailViewModeRenders(t *testing.T) {
	view := realRenderer(t)
	w := httptest.NewRecorder()
	view.RenderApp(w, "admin/group_detail.html", detailViewFixture(false))

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{
		"Grup developers",           // page-header
		"Anggota (1)",               // kartu anggota
		"Tambah Anggota",            // pembuka pop up
		"Akses Database",            // kartu akses (mode lihat)
		"sample",                    // koneksi
		"View + Edit",               // label level
		"?edit=1",                   // jalan ke mode ubah
		"add-members-modal",         // pop up ikut terrender
	} {
		if !strings.Contains(body, want) {
			t.Errorf("mode lihat tidak memuat %q", want)
		}
	}
	if strings.Contains(body, "access-editor") {
		t.Error("mode lihat tidak boleh merender access-editor")
	}
}

func TestGroupDetailEditModeRenders(t *testing.T) {
	view := realRenderer(t)
	w := httptest.NewRecorder()
	view.RenderApp(w, "admin/group_detail.html", detailViewFixture(true))

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{
		"access-form",                  // form simpan batch
		`name="access:5"`,              // field level lama
		"candidate-db",                 // dropdown database tersedia
		"/admin/groups/1/access",       // FormAction
		"Simpan Perubahan",             // action bar
		`/admin/groups/1/delete`,       // form hapus
	} {
		if !strings.Contains(body, want) {
			t.Errorf("mode ubah tidak memuat %q", want)
		}
	}
}

func TestGroupsListRendersFeComponentsTable(t *testing.T) {
	view := realRenderer(t)
	w := httptest.NewRecorder()
	view.RenderApp(w, "admin/groups.html", groupsListView{
		AppShell: web.AppShell{
			Title:     "Kelola Grup",
			Me:        fixtureAdmin(),
			CSRFToken: "token",
			Initials:  "A",
			ActiveNav: "groups",
		},
		PageHeader: web.GroupsHeader("/admin/groups/new"),
		Groups: web.NewGroupsTable([]web.GroupSummary{{
			Name:        "analis",
			Href:        "/admin/groups/1",
			MemberCount: 2,
			Access:      []web.GroupAccess{{Database: "toko", Level: web.AccessEdit}},
		}}),
	})

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{"Kelola Grup", "analis", "2 anggota", "toko (View + Edit)", "/admin/groups/new"} {
		if !strings.Contains(body, want) {
			t.Errorf("daftar group tidak memuat %q", want)
		}
	}
}
