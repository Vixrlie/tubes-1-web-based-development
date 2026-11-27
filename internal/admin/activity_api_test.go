package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/go-chi/chi/v5"
)

type activityValidator func(context.Context, string) (auth.SessionInfo, error)

func (v activityValidator) Validate(ctx context.Context, token string) (auth.SessionInfo, error) {
	return v(ctx, token)
}

type activityReaderStub struct {
	called bool
	err    error
}

func (s *activityReaderStub) List(context.Context, activity.ActivityFilter) (activity.ActivityPage, error) {
	s.called = true
	return activity.ActivityPage{Items: []activity.Entry{}, Page: 1, PageSize: 8}, s.err
}
func (s *activityReaderStub) Get(context.Context, string) (activity.Entry, error) {
	s.called = true
	return activity.Entry{}, activity.ErrActivityNotFound
}

func TestActivityJSONPermissionsAndAvailability(t *testing.T) {
	for _, tc := range []struct {
		name, token                string
		user                       auth.User
		validationErr, errorSource error
		want                       int
	}{
		{name: "anonymous", want: 401},
		{name: "inactive", token: "session", user: auth.User{ID: 1}, want: 401},
		{name: "nonadmin", token: "session", user: auth.User{ID: 1, IsActive: true}, want: 403},
		{name: "validation unavailable", token: "session", validationErr: errors.New("offline"), want: 503},
		{name: "admin", token: "session", user: auth.User{ID: 1, IsActive: true, IsPlatformAdmin: true}, want: 200},
		{name: "loki unavailable", token: "session", user: auth.User{ID: 1, IsActive: true, IsPlatformAdmin: true}, errorSource: activity.ErrActivityUnavailable, want: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &activityReaderStub{err: tc.errorSource}
			r := chi.NewRouter()
			ActivityJSONRoutes(r, source, activityValidator(func(context.Context, string) (auth.SessionInfo, error) {
				return auth.SessionInfo{User: tc.user}, tc.validationErr
			}), false)
			req := httptest.NewRequest("GET", "/api/admin/activity-logs", nil)
			if tc.token != "" {
				req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: tc.token})
			}
			res := httptest.NewRecorder()
			r.ServeHTTP(res, req)
			if res.Code != tc.want || res.Header().Get("Content-Type") != "application/json; charset=utf-8" || res.Header().Get("Location") != "" {
				t.Fatalf("status=%d header=%v body=%s", res.Code, res.Header(), res.Body.String())
			}
			if tc.want == 401 || tc.want == 403 || tc.validationErr != nil {
				if source.called {
					t.Fatal("unauthorized query reached Loki")
				}
			}
		})
	}
}

func TestActivityDateRangeAndFilterValidation(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	f, err := activityFilters(httptest.NewRequest("GET", "/?from=2026-11-22&to=2026-11-22&page=2", nil), now)
	if err != nil || f.Start.Hour() != 0 || f.End.Sub(f.Start) != 24*time.Hour || f.Start.UTC().Hour() != 17 || f.Page != 2 {
		t.Fatalf("filter=%+v error=%v", f, err)
	}
	for _, query := range []string{"status=unknown", "category=invalid", "from=bad", "from=2026-11-23&to=2026-11-22", "page=0", "pageSize=101", "from=2026-01-01&to=2026-11-22"} {
		if _, err := activityFilters(httptest.NewRequest("GET", "/?"+query, nil), now); err == nil {
			t.Errorf("accepted %s", query)
		}
	}
}
