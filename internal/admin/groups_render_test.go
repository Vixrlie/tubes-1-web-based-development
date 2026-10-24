package admin

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

func TestGroupDetailRendersEditLevel(t *testing.T) {
	view, err := web.NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	page := groupDetailPage{
		Me:        auth.User{ID: 1, Username: "admin", IsPlatformAdmin: true},
		CSRFToken: "token",
		Group:     Group{ID: 1, Name: "developers"},
		Grants: []GrantRow{
			{GroupID: 1, ConnectionID: 5, ConnectionName: "sample", Level: access.ActionEdit},
		},
	}

	w := httptest.NewRecorder()
	view.Render(w, "admin/group_detail.html", page)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "View + Edit") {
		t.Fatalf("detail page did not render edit level; body=%s", w.Body.String())
	}
}

func TestGroupDetailHidesExistingChoices(t *testing.T) {
	view, err := web.NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	page := groupDetailPage{
		Group:          Group{ID: 1, Name: "developers"},
		Members:        []UserRow{{ID: 1, Username: "member"}},
		AvailableUsers: []UserRow{{ID: 2, Username: "candidate"}},
		Grants:         []GrantRow{{GroupID: 1, ConnectionID: 5, ConnectionName: "granted"}},
		AvailableConnections: []ConnectionRef{
			{ID: 6, Name: "candidate-db"},
		},
	}

	w := httptest.NewRecorder()
	view.Render(w, "admin/group_detail.html", page)
	body := w.Body.String()

	if strings.Contains(body, `<option value="1">member</option>`) {
		t.Fatal("existing member is still offered in the add-member dropdown")
	}
	if !strings.Contains(body, `<option value="2">candidate</option>`) {
		t.Fatal("available user is missing from the add-member dropdown")
	}
	if strings.Contains(body, `<option value="5">granted</option>`) {
		t.Fatal("already granted connection is still offered in the grant dropdown")
	}
	if !strings.Contains(body, `<option value="6">candidate-db</option>`) {
		t.Fatal("available connection is missing from the grant dropdown")
	}
}
