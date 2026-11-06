package activity

import (
	"context"
	"log/slog"
	"os"
	"time"
)

type Event struct {
	Time   time.Time
	UserID string
	Action string // mis. "user.login", "db.update"
	Status string // "success" | "failed"
	Detail map[string]string
	Err    error
}

type Logger struct {
	sl *slog.Logger
}

func New() *Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return &Logger{sl: slog.New(h)}
}

// Log mencatat satu aktivitas
func (l *Logger) Log(ctx context.Context, e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	attrs := []any{
		"action", e.Action,
		"status", e.Status,
		"user_id", e.UserID,
		"request_id", ctx.Value(requestIDKey{}),
	}
	for k, v := range e.Detail {
		if isSensitive(k) {
			continue
		}
		attrs = append(attrs, k, v)
	}
	switch e.Status {
	case "failed":
		l.sl.Error("activity", append(attrs, "error", e.Err)...)
	case "warning":
		l.sl.Warn("activity", attrs...)
	default:
		l.sl.Info("activity", attrs...)
	}
}

type requestIDKey struct{}

func isSensitive(key string) bool {
	switch key {
	case "password", "token", "secret", "authorization", "cookie":
		return true
	}
	return false
}
