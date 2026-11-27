package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresTracerRedactsSQLAndPreservesCorrelation(t *testing.T) {
	var output bytes.Buffer
	tracer := PostgresTracer{Logger: NewWithWriter(&output), Database: "target"}
	ctx := WithMigrationID(WithActor(WithRequestID(context.Background(), "req-1"), 42), 7)
	ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{
		SQL:  "SELECT 'secret-literal', $1",
		Args: []any{"secret-argument"},
	})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{
		Err: &pgconn.PgError{Code: "22000", Message: "secret-error"},
	})
	for _, secret := range []string{"secret-literal", "secret-argument", "secret-error"} {
		if bytes.Contains(output.Bytes(), []byte(secret)) {
			t.Fatalf("log exposed %q", secret)
		}
	}
	decoder := json.NewDecoder(&output)
	for _, action := range []string{"sql.start", "sql.result"} {
		var entry map[string]any
		if err := decoder.Decode(&entry); err != nil {
			t.Fatal(err)
		}
		for field, want := range map[string]string{
			"action": action, "request_id": "req-1", "user_id": "42",
			"migration_id": "7", "database": "target", "query": "SELECT",
		} {
			if entry[field] != want {
				t.Errorf("%s = %v, want %q", field, entry[field], want)
			}
		}
		if action == "sql.result" && (entry["status"] != StatusFailed || entry["sqlstate"] != "22000") {
			t.Errorf("unexpected failure event: %v", entry)
		}
	}
}
