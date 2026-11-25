package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

func TestAdminAPIGroupLeader(t *testing.T) {
	ctx := context.Background()
	pool := testutil.NewPool(t)
	store := NewPGGroupStore(pool)
	grp, err := store.CreateGroup(ctx, "tim-leader")
	if err != nil {
		t.Fatal(err)
	}
	var member, outsider int64
	for name, dst := range map[string]*int64{"anggota": &member, "luar": &outsider} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (username, password_hash) VALUES ($1, 'x') RETURNING id`, name).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.AddMember(ctx, grp.ID, member); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	RegisterAdminJSONRoutes(r, AdminAPIConfig{
		Groups: store,
		Validator: stubValidator{info: auth.SessionInfo{
			User:      auth.User{ID: member, Username: "admin", IsActive: true, IsPlatformAdmin: true},
			CSRFToken: "csrf-ok",
		}},
		Revoker: &stubRevoker{},
		Evictor: &stubEvictor{},
	})
	do := func(method, path, body string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, adminReq(method, path, body, "csrf-ok"))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	base := fmt.Sprintf("/api/admin/groups/%d", grp.ID)

	if code, body := do(http.MethodGet, base, ""); code != http.StatusOK || body["leaderId"] != nil {
		t.Fatalf("awal tanpa leader: %d %v", code, body)
	}
	if code, body := do(http.MethodPut, base+"/leader", fmt.Sprintf(`{"userId":"%d"}`, outsider)); code != http.StatusUnprocessableEntity || body["error"].(map[string]any)["code"] != "LEADER_NOT_MEMBER" {
		t.Fatalf("bukan anggota: %d %v", code, body)
	}
	if code, body := do(http.MethodPut, base+"/leader", fmt.Sprintf(`{"userId":"%d"}`, member)); code != http.StatusOK || body["leaderId"] != fmt.Sprint(member) {
		t.Fatalf("set leader: %d %v", code, body)
	}
	if code, body := do(http.MethodGet, base, ""); code != http.StatusOK || body["leaderId"] != fmt.Sprint(member) {
		t.Fatalf("detail memuat leader: %d %v", code, body)
	}
	if code, body := do(http.MethodPut, base+"/leader", `{"userId":null}`); code != http.StatusOK || body["leaderId"] != nil {
		t.Fatalf("kosongkan leader: %d %v", code, body)
	}
	if code, _ := do(http.MethodPut, fmt.Sprintf("/api/admin/groups/%d/leader", grp.ID+999), `{"userId":null}`); code != http.StatusNotFound {
		t.Fatalf("grup tidak ada: %d", code)
	}
	if code, _ := do(http.MethodPut, base+"/leader", `{"userId":"abc"}`); code != http.StatusBadRequest {
		t.Fatalf("id tidak valid: %d", code)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, adminReq(http.MethodPut, base+"/leader", `{"userId":null}`, ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tanpa CSRF harus 403, got %d", rec.Code)
	}
}
