package activity

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	StatusSuccess = "success"
	StatusWarning = "warning"
	StatusFailed  = "failed"
)

type Event struct {
	Time   time.Time
	UserID string
	Action string
	Status string
	Detail map[string]string
	Err    error
}

type Logger struct {
	sl *slog.Logger
}

func New() *Logger {
	return NewWithWriter(os.Stdout)
}

func NewWithWriter(w io.Writer) *Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	})
	return &Logger{sl: slog.New(h)}
}

func (l *Logger) Log(ctx context.Context, e Event) {
	if l == nil || l.sl == nil {
		return
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}

	attrs := []any{
		"action", e.Action,
		"status", e.Status,
		"user_id", e.UserID,
		"request_id", RequestIDFromContext(ctx),
	}

	for k, v := range e.Detail {
		if isSensitive(k) {
			continue
		}
		attrs = append(attrs, k, v)
	}

	switch e.Status {
	case StatusFailed:
		if e.Err != nil {
			attrs = append(attrs, "error", "operation failed")
		}
		l.sl.Error("activity", attrs...)
	case StatusWarning:
		l.sl.Warn("activity", attrs...)
	default:
		l.sl.Info("activity", attrs...)
	}
}

type requestIDKey struct{}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

func isSensitive(key string) bool {
	switch strings.ToLower(key) {
	case "password", "token", "secret", "authorization", "cookie", "session_id", "session_token", "connection_url", "dsn":
		return true
	}
	return false
}
