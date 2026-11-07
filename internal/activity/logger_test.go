package activity

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestIDContextRoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-123")
	if got := RequestIDFromContext(ctx); got != "req-123" {
		t.Fatalf("RequestIDFromContext() = %q, want %q", got, "req-123")
	}
}

func TestRequestIDFromContextWithoutID(t *testing.T) {
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Fatalf("RequestIDFromContext() = %q, want empty string", got)
	}
	if got := RequestIDFromContext(nil); got != "" {
		t.Fatalf("RequestIDFromContext(nil) = %q, want empty string", got)
	}
}

func TestIsSensitive(t *testing.T) {
	for _, key := range []string{"password", "token", "secret", "authorization", "cookie"} {
		if !isSensitive(key) {
			t.Errorf("isSensitive(%q) = false, want true", key)
		}
	}
	for _, key := range []string{"method", "path", "resource_id"} {
		if isSensitive(key) {
			t.Errorf("isSensitive(%q) = true, want false", key)
		}
	}
}

func TestLoggerWritesStructuredJSONAndRedactsSensitiveDetails(t *testing.T) {
	var output strings.Builder
	logger := NewWithWriter(&output)
	ctx := WithRequestID(context.Background(), "req-test")

	logger.Log(ctx, Event{
		UserID: "arina",
		Action: "user.login",
		Status: StatusSuccess,
		Detail: map[string]string{
			"method":   "POST",
			"password": "must-not-be-logged",
			"token":    "also-must-not-be-logged",
		},
	})

	if strings.Contains(output.String(), "must-not-be-logged") || strings.Contains(output.String(), "also-must-not-be-logged") {
		t.Fatal("log output contains sensitive detail")
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(output.String()), &entry); err != nil {
		t.Fatalf("log is not valid JSON: %v", err)
	}
	for key, want := range map[string]string{
		"action":     "user.login",
		"status":     StatusSuccess,
		"user_id":    "arina",
		"request_id": "req-test",
		"method":     "POST",
	} {
		if got := entry[key]; got != want {
			t.Errorf("log field %q = %#v, want %q", key, got, want)
		}
	}
	if _, exists := entry["password"]; exists {
		t.Error("log contains password field")
	}
	if _, exists := entry["token"]; exists {
		t.Error("log contains token field")
	}
}

func TestLoggerUsesErrorLevelForFailure(t *testing.T) {
	var output strings.Builder
	logger := NewWithWriter(&output)
	logger.Log(context.Background(), Event{
		Action: "user.login",
		Status: StatusFailed,
	})

	var entry map[string]any
	if err := json.Unmarshal([]byte(output.String()), &entry); err != nil {
		t.Fatalf("log is not valid JSON: %v", err)
	}
	if got := entry["level"]; got != "ERROR" {
		t.Errorf("log level = %#v, want ERROR", got)
	}
}

func TestLoggerUsesWarningLevel(t *testing.T) {
	var output strings.Builder
	logger := NewWithWriter(&output)
	logger.Log(context.Background(), Event{
		Action: "user.login",
		Status: StatusWarning,
	})

	var entry map[string]any
	if err := json.Unmarshal([]byte(output.String()), &entry); err != nil {
		t.Fatalf("log is not valid JSON: %v", err)
	}
	if got := entry["level"]; got != "WARN" {
		t.Errorf("log level = %#v, want WARN", got)
	}
}
