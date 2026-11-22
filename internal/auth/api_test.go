package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

type jsonTestValidator func(context.Context, string) (SessionInfo, error)

func (v jsonTestValidator) Validate(ctx context.Context, token string) (SessionInfo, error) {
	return v(ctx, token)
}

type jsonTestSessions struct {
	revoked   bool
	revokeErr error
}

func (s *jsonTestSessions) Create(context.Context, int64) (string, error) {
	s.revoked = false
	return "private-session-token", nil
}
func (s *jsonTestSessions) Revoke(context.Context, string) error {
	s.revoked = true
	return s.revokeErr
}

func TestJSONAuthJourney(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	user := User{ID: 42, Username: "arina", IsActive: true, IsPlatformAdmin: true}
	sessions := &jsonTestSessions{}
	h := NewHandlers(handlerTestStore{account: Account{User: user, PasswordHash: hash}}, sessions, handlerTestRenderer{}, false)
	router := chi.NewRouter()
	h.JSONRoutes(router, jsonTestValidator(func(_ context.Context, token string) (SessionInfo, error) {
		if token != "private-session-token" || sessions.revoked {
			return SessionInfo{}, ErrInvalidSession
		}
		return SessionInfo{User: user, CSRFToken: "private-csrf"}, nil
	}))
	request := func(method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if res := request("GET", "/api/auth/session", "", nil, ""); res.Code != 401 || res.Header().Get("Location") != "" {
		t.Fatalf("missing cookie: %d %s", res.Code, res.Body)
	}
	login := request("POST", "/api/auth/login", `{"username":"arina","password":"correct-password"}`, nil, "")
	if login.Code != 200 {
		t.Fatalf("login: %d %s", login.Code, login.Body)
	}
	if strings.Contains(login.Body.String(), "private-session-token") || strings.Contains(login.Body.String(), hash) {
		t.Fatal("credentials leaked into JSON")
	}
	var dto struct {
		User struct {
			ID              string
			Username        string
			IsPlatformAdmin bool
		}
		CSRFToken string
	}
	if err := json.Unmarshal(login.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.User.ID != "42" || !dto.User.IsPlatformAdmin || dto.CSRFToken != "private-csrf" {
		t.Fatalf("unexpected DTO: %+v", dto)
	}
	cookie := login.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("insecure cookie: %+v", cookie)
	}
	if res := request("GET", "/api/auth/session", "", cookie, ""); res.Code != 200 || res.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("restore: %d %s", res.Code, res.Body)
	}
	if res := request("POST", "/api/auth/logout", "", cookie, "wrong"); res.Code != 403 || sessions.revoked {
		t.Fatalf("CSRF: %d", res.Code)
	}
	logout := request("POST", "/api/auth/logout", "", cookie, "private-csrf")
	if logout.Code != 204 || !sessions.revoked || logout.Result().Cookies()[0].MaxAge != -1 {
		t.Fatalf("logout: %d %s", logout.Code, logout.Body)
	}
	if res := request("GET", "/api/auth/session", "", cookie, ""); res.Code != 401 {
		t.Fatalf("revoked session accepted: %d", res.Code)
	}
}

func TestJSONLoginRejectsInvalidRequests(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body, contentType, site string
		active                        bool
		want                          int
	}{
		{"wrong password", `{"username":"arina","password":"wrong"}`, "application/json", "", true, 401},
		{"inactive account", `{"username":"arina","password":"correct-password"}`, "application/json", "", false, 401},
		{"missing password", `{"username":"arina"}`, "application/json", "", true, 400},
		{"unknown property", `{"username":"arina","password":"correct-password","admin":true}`, "application/json", "", true, 400},
		{"trailing JSON", `{"username":"arina","password":"correct-password"} {}`, "application/json", "", true, 400},
		{"oversized body", `{"username":"` + strings.Repeat("x", maxLoginBody) + `","password":"correct-password"}`, "application/json", "", true, 400},
		{"form body", "username=arina&password=correct-password", "application/x-www-form-urlencoded", "", true, 415},
		{"cross site", `{"username":"arina","password":"correct-password"}`, "application/json", "cross-site", true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandlers(handlerTestStore{account: Account{User: User{ID: 42, Username: "arina", IsActive: tc.active}, PasswordHash: hash}}, &handlerTestSessions{}, handlerTestRenderer{}, false)
			router := chi.NewRouter()
			h.JSONRoutes(router, jsonTestValidator(func(context.Context, string) (SessionInfo, error) {
				t.Fatal("must not create a session")
				return SessionInfo{}, nil
			}))
			r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			if tc.site != "" {
				r.Header.Set("Sec-Fetch-Site", tc.site)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != tc.want || len(w.Result().Cookies()) != 0 || w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("got %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestJSONSessionErrorsRotationAndLogoutFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		info SessionInfo
		err  error
		want int
	}{
		{"expired", SessionInfo{}, ErrInvalidSession, 401},
		{"disabled", SessionInfo{User: User{ID: 42, IsActive: false}}, nil, 401},
		{"infrastructure", SessionInfo{}, errors.New("private database error"), 503},
		{"rotated", SessionInfo{User: User{ID: 42, Username: "arina", IsActive: true}, CSRFToken: "csrf", NewToken: "rotated-secret"}, nil, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandlers(handlerTestStore{}, &handlerTestSessions{}, handlerTestRenderer{}, true)
			router := chi.NewRouter()
			h.JSONRoutes(router, jsonTestValidator(func(context.Context, string) (SessionInfo, error) { return tc.info, tc.err }))
			r := httptest.NewRequest("GET", "/api/auth/session", nil)
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "old-secret"})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != tc.want || strings.Contains(w.Body.String(), "private database") || strings.Contains(w.Body.String(), "rotated-secret") {
				t.Fatalf("response: %d %s", w.Code, w.Body)
			}
			if tc.info.NewToken != "" {
				cookie := w.Result().Cookies()[0]
				if cookie.Value != tc.info.NewToken || !cookie.Secure {
					t.Fatal("rotation cookie missing")
				}
			}
		})
	}
	h := NewHandlers(handlerTestStore{}, &handlerTestSessions{revokeErr: errors.New("private error")}, handlerTestRenderer{}, false)
	router := chi.NewRouter()
	h.JSONRoutes(router, jsonTestValidator(func(context.Context, string) (SessionInfo, error) {
		return SessionInfo{User: User{ID: 42, IsActive: true}, CSRFToken: "csrf"}, nil
	}))
	r := httptest.NewRequest("POST", "/api/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "secret"})
	r.Header.Set("X-CSRF-Token", "csrf")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 500 || w.Result().Cookies()[0].MaxAge != -1 || !strings.Contains(w.Body.String(), "LOGOUT_FAILED") {
		t.Fatalf("failed revocation: %d %s", w.Code, w.Body)
	}
}
