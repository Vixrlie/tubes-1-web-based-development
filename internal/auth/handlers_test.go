package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
)

type handlerTestStore struct {
	account Account
	err     error
}

func (s handlerTestStore) GetByUsername(context.Context, string) (Account, error) {
	return s.account, s.err
}
func (s handlerTestStore) GetByID(context.Context, int64) (Account, error) {
	return s.account, s.err
}
func (s handlerTestStore) Create(context.Context, NewAccount) (Account, error) {
	return s.account, s.err
}

type handlerTestSessions struct {
	createToken string
	createErr   error
	revokeErr   error
}

func (s *handlerTestSessions) Create(context.Context, int64) (string, error) {
	return s.createToken, s.createErr
}
func (s *handlerTestSessions) Revoke(context.Context, string) error { return s.revokeErr }

type handlerTestRenderer struct{}

func (handlerTestRenderer) Render(http.ResponseWriter, string, any) {}

func newTestAuthHandlers(t *testing.T, store Store, sessions *handlerTestSessions) (*Handlers, *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	h := NewHandlers(store, sessions, handlerTestRenderer{}, false)
	h.SetLogger(activity.NewWithWriter(&output))
	return h, &output
}

func parseActivityLog(t *testing.T, output string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	logs := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON log %q: %v", line, err)
		}
		logs = append(logs, entry)
	}
	return logs
}

func TestLoginLogsSuccessWithoutCredentials(t *testing.T) {
	passwordHash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	store := handlerTestStore{account: Account{
		User:         User{ID: 42, Username: "arina", IsActive: true},
		PasswordHash: passwordHash,
	}}
	sessions := &handlerTestSessions{createToken: "session-secret-token"}
	h, output := newTestAuthHandlers(t, store, sessions)

	form := url.Values{"username": {"arina"}, "password": {"correct-password"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(activity.WithRequestID(req.Context(), "req-login-success"))
	res := httptest.NewRecorder()
	h.login(res, req)

	if res.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	logs := parseActivityLog(t, output.String())
	if len(logs) != 1 {
		t.Fatalf("got %d log entries, want 1", len(logs))
	}
	assertLogField(t, logs[0], "action", "user.login")
	assertLogField(t, logs[0], "status", activity.StatusSuccess)
	assertLogField(t, logs[0], "request_id", "req-login-success")
	if strings.Contains(output.String(), "correct-password") || strings.Contains(output.String(), "session-secret-token") {
		t.Fatal("log contains a password or session token")
	}
}

func TestLoginLogsFailure(t *testing.T) {
	passwordHash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	store := handlerTestStore{account: Account{
		User:         User{ID: 42, Username: "arina", IsActive: true},
		PasswordHash: passwordHash,
	}}
	h, output := newTestAuthHandlers(t, store, &handlerTestSessions{})

	form := url.Values{"username": {"arina"}, "password": {"wrong-password"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	h.login(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusUnauthorized)
	}
	logs := parseActivityLog(t, output.String())
	if len(logs) != 1 {
		t.Fatalf("got %d log entries, want 1", len(logs))
	}
	assertLogField(t, logs[0], "action", "user.login")
	assertLogField(t, logs[0], "status", activity.StatusFailed)
	if strings.Contains(output.String(), "wrong-password") {
		t.Fatal("log contains submitted password")
	}
}

func TestLoginLogsSessionCreationFailure(t *testing.T) {
	passwordHash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	store := handlerTestStore{account: Account{
		User:         User{ID: 42, Username: "arina", IsActive: true},
		PasswordHash: passwordHash,
	}}
	h, output := newTestAuthHandlers(t, store, &handlerTestSessions{
		createErr: errors.New("session creation failed"),
	})

	form := url.Values{"username": {"arina"}, "password": {"correct-password"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	h.login(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusInternalServerError)
	}
	logs := parseActivityLog(t, output.String())
	if len(logs) != 1 {
		t.Fatalf("got %d log entries, want 1", len(logs))
	}
	assertLogField(t, logs[0], "action", "user.login")
	assertLogField(t, logs[0], "status", activity.StatusFailed)
	if strings.Contains(output.String(), "correct-password") {
		t.Fatal("log contains submitted password")
	}
}

func TestLogoutLogsSuccess(t *testing.T) {
	sessions := &handlerTestSessions{}
	h, output := newTestAuthHandlers(t, handlerTestStore{}, sessions)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session-secret-token"})
	res := httptest.NewRecorder()
	h.logout(res, req)

	if res.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	logs := parseActivityLog(t, output.String())
	if len(logs) != 1 {
		t.Fatalf("got %d log entries, want 1", len(logs))
	}
	assertLogField(t, logs[0], "action", "user.logout")
	assertLogField(t, logs[0], "status", activity.StatusSuccess)
	if strings.Contains(output.String(), "session-secret-token") {
		t.Fatal("log contains session token")
	}
}

func TestLogoutLogsRevokeFailure(t *testing.T) {
	sessions := &handlerTestSessions{revokeErr: errors.New("revoke failed")}
	h, output := newTestAuthHandlers(t, handlerTestStore{}, sessions)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session-secret-token"})
	res := httptest.NewRecorder()
	h.logout(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusInternalServerError)
	}
	logs := parseActivityLog(t, output.String())
	if len(logs) != 1 {
		t.Fatalf("got %d log entries, want 1", len(logs))
	}
	assertLogField(t, logs[0], "action", "user.logout")
	assertLogField(t, logs[0], "status", activity.StatusFailed)
	if strings.Contains(output.String(), "session-secret-token") {
		t.Fatal("log contains session token")
	}
}

func assertLogField(t *testing.T, entry map[string]any, key string, want any) {
	t.Helper()
	if got := entry[key]; got != want {
		t.Fatalf("log field %q = %#v, want %#v", key, got, want)
	}
}
