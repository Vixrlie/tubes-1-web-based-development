package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSQLSpanCorrelatesWithActivityLog(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	ctx, parent := StartSpan(context.Background(), "migration.execute")
	var output bytes.Buffer
	tracer := PostgresTracer{Logger: NewWithWriter(&output), Database: "target"}
	queryCtx := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	tracer.TraceQueryEnd(queryCtx, nil, pgx.TraceQueryEndData{Err: context.Canceled})
	parent.End()

	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("ended spans = %d, want 2", len(spans))
	}
	query := spans[0]
	if query.Parent().SpanID() != parent.SpanContext().SpanID() || query.Status().Code != codes.Error {
		t.Fatal("SQL span must be a failed child of the migration span")
	}
	decoder := json.NewDecoder(&output)
	for range 2 {
		var entry map[string]any
		if err := decoder.Decode(&entry); err != nil {
			t.Fatal(err)
		}
		if entry["trace_id"] != query.SpanContext().TraceID().String() || entry["span_id"] != query.SpanContext().SpanID().String() {
			t.Fatalf("activity log does not identify its SQL span: %v", entry)
		}
	}
}
