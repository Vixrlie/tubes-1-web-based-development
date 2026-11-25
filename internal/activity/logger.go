package activity

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
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
	return NewWithWriter(defaultOutput)
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
		"action", sanitizeLogString(e.Action),
		"status", sanitizeLogString(string(e.Status)),
		"user_id", e.UserID,
		"request_id", sanitizeLogString(RequestIDFromContext(ctx)),
	}
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		attrs = append(attrs, "trace_id", span.TraceID().String(), "span_id", span.SpanID().String())
	}

	for k, v := range e.Detail {
		if isSensitive(k) {
			continue
		}
		attrs = append(attrs, sanitizeLogString(k), sanitizeLogValue(v))
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

var logLineReplacer = strings.NewReplacer("\r", "", "\n", "")

func sanitizeLogString(s string) string {
	return logLineReplacer.Replace(s)
}

func sanitizeLogValue(v any) any {
	if s, ok := v.(string); ok {
		return logLineReplacer.Replace(s)
	}
	return v
}
