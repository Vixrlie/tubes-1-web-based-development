package dbm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

type fakePolicy struct {
	allowed  bool
	byAction map[access.Action]bool
	err      error
	userID   int64
	connID   int64
	action   access.Action
	calls    int
}

func (p *fakePolicy) Can(ctx context.Context, userID int64, connID int64, a access.Action) (bool, error) {
	p.calls++
	p.userID = userID
	p.connID = connID
	p.action = a
	if p.byAction != nil {
		return p.byAction[a], p.err
	}
	return p.allowed, p.err
}

func TestRequireAccessAllowsWhenPolicyAllows(t *testing.T) {
	policy := &fakePolicy{allowed: true}
	r := chi.NewRouter()
	r.Route("/db/{connID}", func(r chi.Router) {
		r.Use(RequireAccess(policy, access.ActionView))
		r.Get("/browser", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/db/42/browser", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{ID: 7, IsActive: true}))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if policy.calls != 1 || policy.userID != 7 || policy.connID != 42 || policy.action != access.ActionView {
		t.Fatalf("policy got user=%d conn=%d action=%q calls=%d, want user=7 conn=42 action=view calls=1", policy.userID, policy.connID, policy.action, policy.calls)
	}
}

func TestRequireAccessRejectsDeniedPolicy(t *testing.T) {
	policy := &fakePolicy{allowed: false}
	r := chi.NewRouter()
	r.Route("/db/{connID}", func(r chi.Router) {
		r.Use(RequireAccess(policy, access.ActionEdit))
		r.Get("/browser", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/db/11/browser", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{ID: 9, IsActive: true}))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireAccessRejectsWhenPolicyReturnsError(t *testing.T) {
	policy := &fakePolicy{err: errors.New("boom")}
	r := chi.NewRouter()
	r.Route("/db/{connID}", func(r chi.Router) {
		r.Use(RequireAccess(policy, access.ActionView))
		r.Get("/browser", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/db/5/browser", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{ID: 12, IsActive: true}))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireAccessRejectsMissingUser(t *testing.T) {
	policy := &fakePolicy{allowed: true}
	r := chi.NewRouter()
	r.Route("/db/{connID}", func(r chi.Router) {
		r.Use(RequireAccess(policy, access.ActionView))
		r.Get("/browser", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/db/2/browser", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireAccessReturnsNotFoundForBadConnID(t *testing.T) {
	policy := &fakePolicy{allowed: true}
	r := chi.NewRouter()
	r.Route("/db/{connID}", func(r chi.Router) {
		r.Use(RequireAccess(policy, access.ActionView))
		r.Get("/browser", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/db/not-a-number/browser", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{ID: 4, IsActive: true}))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDBMRoutesUseExpectedActions(t *testing.T) {
	pool := testutil.NewPool(t)
	cases := []struct {
		name   string
		method string
		target string
		action access.Action
	}{
		{"browser view", http.MethodGet, "/db/42/browser", access.ActionView},
		{"query view", http.MethodGet, "/db/42/query", access.ActionView},
		{"schema view", http.MethodGet, "/db/42/schema", access.ActionView},
		{"migration list edit", http.MethodGet, "/db/42/migrations/", access.ActionEdit},
		{"migration apply edit", http.MethodPost, "/db/42/migrations/apply", access.ActionEdit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := &fakePolicy{allowed: true}
			r := chi.NewRouter()
			NewHandlers(policy, NewPoolRegistry(map[int64]*pgxpool.Pool{42: pool})).Routes(r)

			req := httptest.NewRequest(tc.method, tc.target, nil)
			req = req.WithContext(auth.WithUser(req.Context(), auth.User{ID: 7, IsActive: true}))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
			}
			if policy.calls != 1 || policy.connID != 42 || policy.action != tc.action {
				t.Fatalf("policy got conn=%d action=%q calls=%d, want conn=42 action=%q calls=1",
					policy.connID, policy.action, policy.calls, tc.action)
			}
		})
	}
}

func TestDBMMigrationsRequireEdit(t *testing.T) {
	pool := testutil.NewPool(t)
	policy := &fakePolicy{byAction: map[access.Action]bool{
		access.ActionView: true,
		access.ActionEdit: false,
	}}
	r := chi.NewRouter()
	NewHandlers(policy, NewPoolRegistry(map[int64]*pgxpool.Pool{42: pool})).Routes(r)

	for _, tc := range []struct {
		method string
		target string
	}{
		{http.MethodGet, "/db/42/migrations/"},
		{http.MethodPost, "/db/42/migrations/apply"},
	} {
		req := httptest.NewRequest(tc.method, tc.target, nil)
		req = req.WithContext(auth.WithUser(req.Context(), auth.User{ID: 7, IsActive: true}))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d, want %d", tc.method, tc.target, w.Code, http.StatusForbidden)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/db/42/browser", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{ID: 7, IsActive: true}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("view grant should allow browser, got %d", w.Code)
	}
}

func TestDBMHandlersUseLiveRegistry(t *testing.T) {
	pool := testutil.NewPool(t)
	var connID int64
	row := pool.QueryRow(context.Background(), `
		INSERT INTO db_connections (name, connection_url) VALUES ($1, $2) RETURNING id
	`, "dbm-live-test", pool.Config().ConnString())
	if err := row.Scan(&connID); err != nil {
		t.Fatalf("insert db connection: %v", err)
	}

	policy := &fakePolicy{byAction: map[access.Action]bool{
		access.ActionView: true,
		access.ActionEdit: true,
	}}
	r := chi.NewRouter()
	NewHandlers(policy, NewPGRegistry(pool)).Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/db/"+strconv.FormatInt(connID, 10)+"/browser", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{ID: 7, IsActive: true}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("browser status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "database") {
		t.Fatalf("browser response = %q, want database info", w.Body.String())
	}
}
