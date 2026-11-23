package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

type stubValidator struct {
	info auth.SessionInfo
	err  error
}

func (v stubValidator) Validate(context.Context, string) (auth.SessionInfo, error) {
	return v.info, v.err
}

type stubRevoker struct {
	revoked []int64
	rotated []int64
}

func (r *stubRevoker) RevokeSessionsByUser(_ context.Context, userID int64) error {
	r.revoked = append(r.revoked, userID)
	return nil
}

func (r *stubRevoker) ForceRotateSessionsByUser(_ context.Context, userID int64) error {
	r.rotated = append(r.rotated, userID)
	return nil
}

type stubEvictor struct {
	evicted []int64
}

func (e *stubEvictor) Evict(connID int64) {
	e.evicted = append(e.evicted, connID)
}

type fakeUserStore struct {
	users      map[int64]UserRow
	nextID     int64
	deleteErr  error
	groupNames map[int64][]string
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		nextID: 3,
		users: map[int64]UserRow{
			1: {ID: 1, Username: "admin", FullName: "Admin", IsPlatformAdmin: true, IsActive: true, CreatedAt: time.Now()},
			2: {ID: 2, Username: "alice", FullName: "Alice", IsPlatformAdmin: false, IsActive: true, CreatedAt: time.Now()},
		},
		groupNames: map[int64][]string{2: {"backend"}},
	}
}

func (s *fakeUserStore) GetByUsername(_ context.Context, username string) (auth.Account, error) {
	for _, u := range s.users {
		if u.Username == username {
			return auth.Account{User: auth.User{ID: u.ID, Username: u.Username, IsPlatformAdmin: u.IsPlatformAdmin, IsActive: u.IsActive}}, nil
		}
	}
	return auth.Account{}, auth.ErrUserNotFound
}

func (s *fakeUserStore) GetByID(_ context.Context, id int64) (auth.Account, error) {
	u, ok := s.users[id]
	if !ok {
		return auth.Account{}, auth.ErrUserNotFound
	}
	return auth.Account{User: auth.User{ID: u.ID, Username: u.Username, IsPlatformAdmin: u.IsPlatformAdmin, IsActive: u.IsActive}}, nil
}

func (s *fakeUserStore) Create(_ context.Context, in auth.NewAccount) (auth.Account, error) {
	for _, u := range s.users {
		if u.Username == in.Username {
			return auth.Account{}, auth.ErrUsernameTaken
		}
	}
	id := s.nextID
	s.nextID++
	s.users[id] = UserRow{
		ID: id, Username: in.Username, FullName: in.FullName, Email: in.Email,
		IsPlatformAdmin: in.IsPlatformAdmin, IsActive: true, CreatedAt: time.Now(),
	}
	return auth.Account{
		User:     auth.User{ID: id, Username: in.Username, IsPlatformAdmin: in.IsPlatformAdmin, IsActive: true},
		FullName: in.FullName,
		Email:    in.Email,
	}, nil
}

func (s *fakeUserStore) List(context.Context) ([]UserRow, error) {
	out := make([]UserRow, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	return out, nil
}

func (s *fakeUserStore) Get(_ context.Context, id int64) (UserRow, error) {
	u, ok := s.users[id]
	if !ok {
		return UserRow{}, auth.ErrUserNotFound
	}
	return u, nil
}

func (s *fakeUserStore) Update(_ context.Context, id int64, upd UserUpdate) error {
	u, ok := s.users[id]
	if !ok {
		return auth.ErrUserNotFound
	}
	u.FullName = upd.FullName
	u.Email = upd.Email
	u.IsPlatformAdmin = upd.IsPlatformAdmin
	s.users[id] = u
	return nil
}

func (s *fakeUserStore) SetActive(_ context.Context, id int64, active bool) error {
	u, ok := s.users[id]
	if !ok {
		return auth.ErrUserNotFound
	}
	u.IsActive = active
	s.users[id] = u
	return nil
}

func (s *fakeUserStore) SetPassword(_ context.Context, id int64, _ string) error {
	if _, ok := s.users[id]; !ok {
		return auth.ErrUserNotFound
	}
	return nil
}

func (s *fakeUserStore) GroupNamesByUser(context.Context) (map[int64][]string, error) {
	return s.groupNames, nil
}

func (s *fakeUserStore) Delete(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	if _, ok := s.users[id]; !ok {
		return auth.ErrUserNotFound
	}
	delete(s.users, id)
	return nil
}

func (s *fakeUserStore) EffectiveAccess(context.Context, int64) ([]EffectiveGrant, error) {
	return nil, nil
}

type fakeGroupStore struct {
	groups  map[int64]Group
	members map[int64][]int64
	grants  map[int64]map[int64]access.Action
	nextID  int64
}

func newFakeGroupStore() *fakeGroupStore {
	return &fakeGroupStore{
		nextID: 2,
		groups: map[int64]Group{
			1: {ID: 1, Name: "backend", MemberCount: 1, CreatedAt: time.Now()},
		},
		members: map[int64][]int64{1: {2}},
		grants:  map[int64]map[int64]access.Action{1: {10: access.ActionView}},
	}
}

func (s *fakeGroupStore) ListGroups(context.Context) ([]Group, error) {
	out := make([]Group, 0, len(s.groups))
	for _, g := range s.groups {
		out = append(out, g)
	}
	return out, nil
}
func (s *fakeGroupStore) GetGroup(_ context.Context, id int64) (Group, error) {
	g, ok := s.groups[id]
	if !ok {
		return Group{}, ErrGroupNotFound
	}
	return g, nil
}
func (s *fakeGroupStore) CreateGroup(_ context.Context, name string) (Group, error) {
	for _, g := range s.groups {
		if g.Name == name {
			return Group{}, ErrGroupNameTaken
		}
	}
	id := s.nextID
	s.nextID++
	g := Group{ID: id, Name: name, CreatedAt: time.Now()}
	s.groups[id] = g
	return g, nil
}
func (s *fakeGroupStore) RenameGroup(_ context.Context, id int64, name string) error {
	g, ok := s.groups[id]
	if !ok {
		return ErrGroupNotFound
	}
	g.Name = name
	s.groups[id] = g
	return nil
}
func (s *fakeGroupStore) DeleteGroup(_ context.Context, id int64) error {
	if _, ok := s.groups[id]; !ok {
		return ErrGroupNotFound
	}
	delete(s.groups, id)
	return nil
}
func (s *fakeGroupStore) ListMembers(_ context.Context, groupID int64) ([]UserRow, error) {
	var out []UserRow
	for _, uid := range s.members[groupID] {
		out = append(out, UserRow{ID: uid, Username: "alice", IsActive: true})
	}
	return out, nil
}
func (s *fakeGroupStore) AddMember(_ context.Context, groupID, userID int64) error {
	s.members[groupID] = append(s.members[groupID], userID)
	return nil
}
func (s *fakeGroupStore) RemoveMember(_ context.Context, groupID, userID int64) error {
	list := s.members[groupID]
	for i, id := range list {
		if id == userID {
			s.members[groupID] = append(list[:i], list[i+1:]...)
			return nil
		}
	}
	return ErrMemberNotFound
}
func (s *fakeGroupStore) MemberIDs(_ context.Context, groupID int64) ([]int64, error) {
	return append([]int64(nil), s.members[groupID]...), nil
}
func (s *fakeGroupStore) ListGrants(_ context.Context, groupID int64) ([]GrantRow, error) {
	var out []GrantRow
	for cid, lvl := range s.grants[groupID] {
		out = append(out, GrantRow{GroupID: groupID, ConnectionID: cid, ConnectionName: "target-demo", Level: lvl})
	}
	return out, nil
}
func (s *fakeGroupStore) GetGrant(_ context.Context, groupID, connID int64) (access.Action, bool, error) {
	lvl, ok := s.grants[groupID][connID]
	return lvl, ok, nil
}
func (s *fakeGroupStore) SetGrant(_ context.Context, groupID, connID int64, level access.Action) error {
	if s.grants[groupID] == nil {
		s.grants[groupID] = map[int64]access.Action{}
	}
	s.grants[groupID][connID] = level
	return nil
}
func (s *fakeGroupStore) RevokeGrant(_ context.Context, groupID, connID int64) error {
	delete(s.grants[groupID], connID)
	return nil
}
func (s *fakeGroupStore) AccessSummaries(context.Context) (map[int64][]string, error) { return nil, nil }
func (s *fakeGroupStore) AllGrants(context.Context) (map[int64]map[int64]access.Action, error) {
	return s.grants, nil
}
func (s *fakeGroupStore) GroupAccesses(context.Context) (map[int64][]web.GroupAccess, error) {
	return map[int64][]web.GroupAccess{1: {{Database: "target-demo", Level: "view"}}}, nil
}

type fakeConnStore struct {
	conns   map[int64]ConnectionRow
	nextID  int64
	testErr error
}

func newFakeConnStore() *fakeConnStore {
	return &fakeConnStore{
		nextID: 11,
		conns: map[int64]ConnectionRow{
			10: {ID: 10, Name: "target-demo", DBMS: "postgres", Host: "127.0.0.1", Port: "5433", Database: "sample", Username: "target", CreatedAt: time.Now()},
		},
	}
}

func (s *fakeConnStore) List(context.Context) ([]ConnectionRow, error) {
	var out []ConnectionRow
	for _, c := range s.conns {
		out = append(out, c)
	}
	return out, nil
}
func (s *fakeConnStore) Get(_ context.Context, id int64) (ConnectionRow, error) {
	c, ok := s.conns[id]
	if !ok {
		return ConnectionRow{}, ErrConnectionNotFound
	}
	return c, nil
}
func (s *fakeConnStore) Create(_ context.Context, in ConnectionInput, _ int64) (ConnectionRow, error) {
	id := s.nextID
	s.nextID++
	c := ConnectionRow{ID: id, Name: in.Name, Description: in.Description, DBMS: in.DBMS, Host: in.Host, Port: in.Port, Database: in.Database, Username: in.Username, CreatedAt: time.Now()}
	s.conns[id] = c
	return c, nil
}
func (s *fakeConnStore) Update(_ context.Context, id int64, in ConnectionInput) (ConnectionRow, error) {
	c, ok := s.conns[id]
	if !ok {
		return ConnectionRow{}, ErrConnectionNotFound
	}
	c.Name = in.Name
	c.Host = in.Host
	c.Port = in.Port
	c.Database = in.Database
	c.Username = in.Username
	s.conns[id] = c
	return c, nil
}
func (s *fakeConnStore) Delete(_ context.Context, id int64) error {
	if _, ok := s.conns[id]; !ok {
		return ErrConnectionNotFound
	}
	delete(s.conns, id)
	return nil
}
func (s *fakeConnStore) UsersWithAccess(context.Context, int64) ([]int64, error) { return []int64{2}, nil }
func (s *fakeConnStore) Test(context.Context, int64) error                       { return s.testErr }
func (s *fakeConnStore) TestInput(context.Context, int64, ConnectionInput) error { return s.testErr }
func (s *fakeConnStore) ConnectionGroups(context.Context) (map[int64][]web.ConnectionGroup, error) {
	return map[int64][]web.ConnectionGroup{10: {{Name: "backend", MemberCount: 1, Level: "view"}}}, nil
}

func (s *fakeConnStore) ListConnections(context.Context) ([]ConnectionRef, error) {
	var out []ConnectionRef
	for _, c := range s.conns {
		out = append(out, ConnectionRef{ID: c.ID, Name: c.Name})
	}
	return out, nil
}

func newAdminAPITestRouter(t *testing.T, v auth.SessionValidator, users *fakeUserStore, groups *fakeGroupStore, conns *fakeConnStore, rev *stubRevoker, ev *stubEvictor) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	RegisterAdminJSONRoutes(r, AdminAPIConfig{
		Users:       users,
		Groups:      groups,
		Connections: conns,
		ConnLister:  conns,
		Revoker:     rev,
		Evictor:     ev,
		Validator:   v,
	})
	return r
}

func adminReq(method, path, body, csrf string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "tok"})
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	return req
}

func TestAdminAPIAuthAndCSRF(t *testing.T) {
	users, groups, conns := newFakeUserStore(), newFakeGroupStore(), newFakeConnStore()

	t.Run("401 tanpa cookie", func(t *testing.T) {
		r := newAdminAPITestRouter(t, stubValidator{}, users, groups, conns, &stubRevoker{}, &stubEvictor{})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/dashboard", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", rec.Code)
		}
	})

	t.Run("403 untuk non-admin", func(t *testing.T) {
		v := stubValidator{info: auth.SessionInfo{User: auth.User{ID: 2, Username: "alice", IsActive: true, IsPlatformAdmin: false}, CSRFToken: "csrf-1"}}
		r := newAdminAPITestRouter(t, v, users, groups, conns, &stubRevoker{}, &stubEvictor{})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, adminReq(http.MethodGet, "/api/admin/dashboard", "", ""))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("got %d, want 403", rec.Code)
		}
	})

	t.Run("403 mutasi tanpa CSRF valid", func(t *testing.T) {
		v := stubValidator{info: auth.SessionInfo{User: auth.User{ID: 1, Username: "admin", IsActive: true, IsPlatformAdmin: true}, CSRFToken: "csrf-ok"}}
		r := newAdminAPITestRouter(t, v, users, groups, conns, &stubRevoker{}, &stubEvictor{})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, adminReq(http.MethodPost, "/api/admin/groups", `{"name":"qa"}`, "wrong"))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("got %d, want 403", rec.Code)
		}
	})
}

func TestAdminAPIDashboardUsersGroupsConnections(t *testing.T) {
	users, groups, conns := newFakeUserStore(), newFakeGroupStore(), newFakeConnStore()
	rev, ev := &stubRevoker{}, &stubEvictor{}
	v := stubValidator{info: auth.SessionInfo{
		User:      auth.User{ID: 1, Username: "admin", IsActive: true, IsPlatformAdmin: true},
		CSRFToken: "csrf-ok",
	}}
	r := newAdminAPITestRouter(t, v, users, groups, conns, rev, ev)

	// 1. Dashboard
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, adminReq(http.MethodGet, "/api/admin/dashboard", "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard: got %d", rec.Code)
	}
	var dash adminDashboardDTO
	_ = json.NewDecoder(rec.Body).Decode(&dash)
	if dash.TotalUsers != 2 || dash.TotalGroups != 1 || dash.TotalConnections != 1 {
		t.Fatalf("unexpected dashboard stats: %+v", dash)
	}

	// 2. Promote user 2 -> ForceRotateSessionsByUser
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, adminReq(http.MethodPatch, "/api/admin/users/2", `{"fullName":"Alice Admin","email":"","isPlatformAdmin":true}`, "csrf-ok"))
	if rec.Code != http.StatusOK {
		t.Fatalf("promote user: got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rev.rotated) != 1 || rev.rotated[0] != 2 {
		t.Fatalf("expected force rotate for user 2, got %v", rev.rotated)
	}

	// 3. Tolak hapus diri sendiri
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, adminReq(http.MethodDelete, "/api/admin/users/1", "", "csrf-ok"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("self delete: got %d, want 409", rec.Code)
	}

	// 4. Update group access (view -> edit) -> ForceRotate
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, adminReq(http.MethodPut, "/api/admin/groups/1/access", `{"grants":[{"connectionId":"10","level":"edit"}]}`, "csrf-ok"))
	if rec.Code != http.StatusOK {
		t.Fatalf("update group access: got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rev.rotated) != 2 {
		t.Fatalf("expected force rotate on escalation, got %v", rev.rotated)
	}

	// 5. Connection test error redacts password
	conns.testErr = errors.New("dial failed: postgres://target:mysecret@127.0.0.1:5433/sample")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, adminReq(http.MethodPost, "/api/admin/connections/10/test", "", "csrf-ok"))
	if rec.Code != http.StatusOK {
		t.Fatalf("test connection: got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mysecret") {
		t.Fatalf("password leaked in test connection response: %s", rec.Body.String())
	}

	// 6. Delete connection -> Evict + Revoke affected users
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, adminReq(http.MethodDelete, "/api/admin/connections/10", "", "csrf-ok"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete connection: got %d", rec.Code)
	}
	if len(ev.evicted) != 1 || ev.evicted[0] != 10 {
		t.Fatalf("expected evict 10, got %v", ev.evicted)
	}
	if len(rev.revoked) == 0 || rev.revoked[len(rev.revoked)-1] != 2 {
		t.Fatalf("expected revoke user 2, got %v", rev.revoked)
	}
}

