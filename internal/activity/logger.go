package activity

import (
	"context"
	"log/slog"
	"os"
	"time"
)

const (
	StatusSuccess = "success"
	StatusWarning = "warning"
	StatusFailed  = "failed"
)

type Event struct {
	Time   time.Time         // diisi otomatis kalau kosong
	UserID string            // actor yang melakukan aktivitas
	Action string            // mis. "user.login", "db.update", "form.submit"
	Status string            // gunakan konstanta Status* di atas
	Detail map[string]string // info tambahan (JANGAN isi data sensitif)
	Err    error             // wajib kalau Status = StatusFailed
}

// Logger mencatat aktivitas sebagai JSON terstruktur ke stdout
// agar mudah dipantau (observability) dan ditelusuri per request.
type Logger struct {
	sl *slog.Logger
}

func New() *Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	})
	return &Logger{sl: slog.New(h)}
}

func (l *Logger) Log(ctx context.Context, e Event) {
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
		l.sl.Error("activity", append(attrs, "error", e.Err)...)
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
	switch key {
	case "password", "token", "secret", "authorization", "cookie":
		return true
	}
	return false
}
