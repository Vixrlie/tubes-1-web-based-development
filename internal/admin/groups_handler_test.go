package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
)

type fakeGroupStore struct {
	listGroups   func(context.Context) ([]Group, error)
	getGroup     func(context.Context, int64) (Group, error)
	createGroup  func(context.Context, string) (Group, error)
	renameGroup  func(context.Context, int64, string) error
	deleteGroup  func(context.Context, int64) error
	listMembers  func(context.Context, int64) ([]UserRow, error)
	addMember    func(context.Context, int64, int64) error
	removeMember func(context.Context, int64, int64) error
	memberIDs    func(context.Context, int64) ([]int64, error)
	listGrants   func(context.Context, int64) ([]GrantRow, error)
	getGrant     func(context.Context, int64, int64) (access.Action, bool, error)
	setGrant     func(context.Context, int64, int64, access.Action) error
	revokeGrant  func(context.Context, int64, int64) error

	accessSummaries func(context.Context) (map[int64][]string, error)
	allGrants       func(context.Context) (map[int64]map[int64]access.Action, error)
}

func (f *fakeGroupStore) ListGroups(ctx context.Context) ([]Group, error) {
	if f.listGroups != nil {
		return f.listGroups(ctx)
	}
	return nil, nil
}

func (f *fakeGroupStore) GetGroup(ctx context.Context, id int64) (Group, error) {
	if f.getGroup != nil {
		return f.getGroup(ctx, id)
	}
	return Group{ID: id, Name: "dev"}, nil
}

func (f *fakeGroupStore) CreateGroup(ctx context.Context, name string) (Group, error) {
	if f.createGroup != nil {
		return f.createGroup(ctx, name)
	}
	return Group{ID: 1, Name: name}, nil
}

func (f *fakeGroupStore) RenameGroup(ctx context.Context, id int64, name string) error {
	if f.renameGroup != nil {
		return f.renameGroup(ctx, id, name)
	}
	return nil
}

func (f *fakeGroupStore) DeleteGroup(ctx context.Context, id int64) error {
	if f.deleteGroup != nil {
		return f.deleteGroup(ctx, id)
	}
	return nil
}

func (f *fakeGroupStore) ListMembers(ctx context.Context, groupID int64) ([]UserRow, error) {
	if f.listMembers != nil {
		return f.listMembers(ctx, groupID)
	}
	return nil, nil
}

func (f *fakeGroupStore) AddMember(ctx context.Context, groupID, userID int64) error {
	if f.addMember != nil {
		return f.addMember(ctx, groupID, userID)
	}
	return nil
}

func (f *fakeGroupStore) RemoveMember(ctx context.Context, groupID, userID int64) error {
	if f.removeMember != nil {
		return f.removeMember(ctx, groupID, userID)
	}
	return nil
}

func (f *fakeGroupStore) MemberIDs(ctx context.Context, groupID int64) ([]int64, error) {
	if f.memberIDs != nil {
		return f.memberIDs(ctx, groupID)
	}
	return nil, nil
}

func (f *fakeGroupStore) ListGrants(ctx context.Context, groupID int64) ([]GrantRow, error) {
	if f.listGrants != nil {
		return f.listGrants(ctx, groupID)
	}
	return nil, nil
}

func (f *fakeGroupStore) GetGrant(ctx context.Context, groupID, connID int64) (access.Action, bool, error) {
	if f.getGrant != nil {
		return f.getGrant(ctx, groupID, connID)
	}
	return "", false, nil
}

func (f *fakeGroupStore) SetGrant(ctx context.Context, groupID, connID int64, level access.Action) error {
	if f.setGrant != nil {
		return f.setGrant(ctx, groupID, connID, level)
	}
	return nil
}

func (f *fakeGroupStore) RevokeGrant(ctx context.Context, groupID, connID int64) error {
	if f.revokeGrant != nil {
		return f.revokeGrant(ctx, groupID, connID)
	}
	return nil
}

func (f *fakeGroupStore) AccessSummaries(ctx context.Context) (map[int64][]string, error) {
	if f.accessSummaries != nil {
		return f.accessSummaries(ctx)
	}
	return map[int64][]string{}, nil
}

func (f *fakeGroupStore) AllGrants(ctx context.Context) (map[int64]map[int64]access.Action, error) {
	if f.allGrants != nil {
		return f.allGrants(ctx)
	}
	return map[int64]map[int64]access.Action{}, nil
}

type fakeUserLister struct {
	users []UserRow
	err   error
}

func (f fakeUserLister) List(context.Context) ([]UserRow, error) { return f.users, f.err }

type fakeConnLister struct {
	conns []ConnectionRef
	err   error
}

func (f fakeConnLister) ListConnections(context.Context) ([]ConnectionRef, error) {
	return f.conns, f.err
}

type fakeRevoker struct {
	ids []int64
	err error
}

func (f *fakeRevoker) RevokeSessionsByUser(ctx context.Context, userID int64) error {
	f.ids = append(f.ids, userID)
	return f.err
}

type fakeRenderer struct {
	names []string
}

func (f *fakeRenderer) Render(w http.ResponseWriter, name string, data any) {
	f.names = append(f.names, name)
}

func (f *fakeRenderer) RenderApp(w http.ResponseWriter, name string, data any) {
	f.names = append(f.names, name)
}

func newGroupHandler(store GroupStore, rev *fakeRevoker, view *fakeRenderer) *groupHandlers {
	if rev == nil {
		rev = &fakeRevoker{}
	}
	if view == nil {
		view = &fakeRenderer{}
	}
	return &groupHandlers{
		store:   store,
		users:   fakeUserLister{},
		conns:   fakeConnLister{},
		revoker: rev,
		view:    view,
	}
}

func formRequest(method, target string, form url.Values) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func withURLParam(r *http.Request, name, value string) *http.Request {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		rctx.URLParams.Add(name, value)
		return r
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(name, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestCreateGroupSuccess(t *testing.T) {
	g := newGroupHandler(&fakeGroupStore{}, nil, nil)
	w := httptest.NewRecorder()

	g.create(w, formRequest(http.MethodPost, "/admin/groups", url.Values{"name": {"dev"}}))

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/admin/groups?status=created" {
		t.Fatalf("Location = %q", got)
	}
}

func TestCreateGroupEmptyName(t *testing.T) {
	view := &fakeRenderer{}
	g := newGroupHandler(&fakeGroupStore{}, nil, view)
	w := httptest.NewRecorder()

	g.create(w, formRequest(http.MethodPost, "/admin/groups", url.Values{"name": {"   "}}))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if len(view.names) != 1 || view.names[0] != "admin/group_form.html" {
		t.Fatalf("rendered = %v", view.names)
	}
}

func TestCreateGroupDuplicate(t *testing.T) {
	view := &fakeRenderer{}
	store := &fakeGroupStore{
		createGroup: func(context.Context, string) (Group, error) {
			return Group{}, ErrGroupNameTaken
		},
	}
	g := newGroupHandler(store, nil, view)
	w := httptest.NewRecorder()

	g.create(w, formRequest(http.MethodPost, "/admin/groups", url.Values{"name": {"dev"}}))

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestSetGrantInvalidLevel(t *testing.T) {
	called := false
	store := &fakeGroupStore{
		setGrant: func(context.Context, int64, int64, access.Action) error {
			called = true
			return nil
		},
	}
	g := newGroupHandler(store, nil, nil)
	req := withURLParam(formRequest(http.MethodPost, "/admin/groups/1/grants",
		url.Values{"connection_id": {"5"}, "level": {"owner"}}), "id", "1")
	w := httptest.NewRecorder()

	g.setGrant(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("SetGrant must not be called for an invalid level")
	}
}

func TestSetGrantDowngradeRevokesMembers(t *testing.T) {
	store := &fakeGroupStore{
		getGrant: func(context.Context, int64, int64) (access.Action, bool, error) {
			return access.ActionEdit, true, nil
		},
		memberIDs: func(context.Context, int64) ([]int64, error) {
			return []int64{10, 11}, nil
		},
	}
	rev := &fakeRevoker{}
	g := newGroupHandler(store, rev, nil)
	req := withURLParam(formRequest(http.MethodPost, "/admin/groups/1/grants",
		url.Values{"connection_id": {"5"}, "level": {"view"}}), "id", "1")
	w := httptest.NewRecorder()

	g.setGrant(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if len(rev.ids) != 2 || rev.ids[0] != 10 || rev.ids[1] != 11 {
		t.Fatalf("revoked = %v, want [10 11]", rev.ids)
	}
}

func TestAddMemberRevokesUser(t *testing.T) {
	store := &fakeGroupStore{}
	rev := &fakeRevoker{}
	g := newGroupHandler(store, rev, nil)
	req := withURLParam(formRequest(http.MethodPost, "/admin/groups/1/members",
		url.Values{"user_id": {"7"}}), "id", "1")
	w := httptest.NewRecorder()

	g.addMember(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if len(rev.ids) != 1 || rev.ids[0] != 7 {
		t.Fatalf("revoked = %v, want [7]", rev.ids)
	}
}

func TestRemoveMemberNotFound(t *testing.T) {
	store := &fakeGroupStore{
		removeMember: func(context.Context, int64, int64) error {
			return ErrMemberNotFound
		},
	}
	g := newGroupHandler(store, nil, nil)
	req := withURLParam(withURLParam(
		httptest.NewRequest(http.MethodPost, "/admin/groups/1/members/2/delete", nil),
		"id", "1"), "userID", "2")
	w := httptest.NewRecorder()

	g.removeMember(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGroupsRoutesRequireAdmin(t *testing.T) {
	r := chi.NewRouter()
	NewHandlers(WithGroupAdmin(
		&fakeGroupStore{}, fakeUserLister{}, fakeConnLister{}, &fakeRevoker{}, &fakeRenderer{},
	)).Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/groups", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRenameGroupSuccess(t *testing.T) {
	store := &fakeGroupStore{}
	g := newGroupHandler(store, nil, nil)
	req := withURLParam(formRequest(http.MethodPost, "/admin/groups/1",
		url.Values{"name": {"platform"}}), "id", "1")
	w := httptest.NewRecorder()

	g.rename(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/admin/groups/1?status=renamed" {
		t.Fatalf("Location = %q", got)
	}
}

func TestRenameGroupConflict(t *testing.T) {
	store := &fakeGroupStore{
		renameGroup: func(context.Context, int64, string) error {
			return ErrGroupNameTaken
		},
	}
	view := &fakeRenderer{}
	g := newGroupHandler(store, nil, view)
	req := withURLParam(formRequest(http.MethodPost, "/admin/groups/1",
		url.Values{"name": {"taken"}}), "id", "1")
	w := httptest.NewRecorder()

	g.rename(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusConflict)
	}
	if len(view.names) != 1 || view.names[0] != "admin/group_detail.html" {
		t.Fatalf("rendered = %v", view.names)
	}
}

func TestSetGrantUpgradeDoesNotRevoke(t *testing.T) {
	memberIDsCalled := false
	store := &fakeGroupStore{
		getGrant: func(context.Context, int64, int64) (access.Action, bool, error) {
			return access.ActionView, true, nil
		},
		memberIDs: func(context.Context, int64) ([]int64, error) {
			memberIDsCalled = true
			return []int64{10}, nil
		},
	}
	rev := &fakeRevoker{}
	g := newGroupHandler(store, rev, nil)
	req := withURLParam(formRequest(http.MethodPost, "/admin/groups/1/grants",
		url.Values{"connection_id": {"5"}, "level": {"edit"}}), "id", "1")
	w := httptest.NewRecorder()

	g.setGrant(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if memberIDsCalled || len(rev.ids) != 0 {
		t.Fatalf("upgrade revoked sessions: memberIDsCalled=%v ids=%v", memberIDsCalled, rev.ids)
	}
}

func TestRevokeGrantRevokesAllMembers(t *testing.T) {
	store := &fakeGroupStore{
		memberIDs: func(context.Context, int64) ([]int64, error) {
			return []int64{3, 4}, nil
		},
	}
	rev := &fakeRevoker{}
	g := newGroupHandler(store, rev, nil)
	req := withURLParam(withURLParam(
		httptest.NewRequest(http.MethodPost, "/admin/groups/1/grants/5/delete", nil),
		"id", "1"), "connID", "5")
	w := httptest.NewRecorder()

	g.revokeGrant(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if len(rev.ids) != 2 || rev.ids[0] != 3 || rev.ids[1] != 4 {
		t.Fatalf("revoked = %v, want [3 4]", rev.ids)
	}
}

func TestDeleteGroupRevokesMembers(t *testing.T) {
	store := &fakeGroupStore{
		memberIDs: func(context.Context, int64) ([]int64, error) {
			return []int64{8, 9}, nil
		},
	}
	rev := &fakeRevoker{}
	g := newGroupHandler(store, rev, nil)
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/admin/groups/1/delete", nil), "id", "1")
	w := httptest.NewRecorder()

	g.delete(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if len(rev.ids) != 2 || rev.ids[0] != 8 || rev.ids[1] != 9 {
		t.Fatalf("revoked = %v, want [8 9]", rev.ids)
	}
}

func TestGroupHandlersRejectInvalidIDs(t *testing.T) {
	cases := []struct {
		name   string
		method string
		target string
		params map[string]string
		call   func(*groupHandlers, http.ResponseWriter, *http.Request)
	}{
		{"detail bad id", http.MethodGet, "/admin/groups/bad", map[string]string{"id": "bad"}, (*groupHandlers).detail},
		{"rename bad id", http.MethodPost, "/admin/groups/bad", map[string]string{"id": "bad"}, (*groupHandlers).rename},
		{"delete bad id", http.MethodPost, "/admin/groups/bad/delete", map[string]string{"id": "bad"}, (*groupHandlers).delete},
		{"remove bad user id", http.MethodPost, "/admin/groups/1/members/bad/delete", map[string]string{"id": "1", "userID": "bad"}, (*groupHandlers).removeMember},
		{"revoke bad conn id", http.MethodPost, "/admin/groups/1/grants/bad/delete", map[string]string{"id": "1", "connID": "bad"}, (*groupHandlers).revokeGrant},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newGroupHandler(&fakeGroupStore{}, nil, nil)
			req := httptest.NewRequest(tc.method, tc.target, nil)
			for k, v := range tc.params {
				req = withURLParam(req, k, v)
			}
			w := httptest.NewRecorder()

			tc.call(g, w, req)

			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
			}
		})
	}
}

func TestSetGrantInvalidConnection(t *testing.T) {
	view := &fakeRenderer{}
	g := newGroupHandler(&fakeGroupStore{}, nil, view)
	req := withURLParam(formRequest(http.MethodPost, "/admin/groups/1/grants",
		url.Values{"connection_id": {"bad"}, "level": {"view"}}), "id", "1")
	w := httptest.NewRecorder()

	g.setGrant(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
