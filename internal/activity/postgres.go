package activity

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type sqlActorKey struct{}
type migrationIDKey struct{}
type sqlExecutionKey struct{}

func WithActor(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, sqlActorKey{}, strconv.FormatInt(id, 10))
}

func WithMigrationID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, migrationIDKey{}, strconv.FormatInt(id, 10))
}

// PostgresTracer records driver executions, including transaction commands.
type PostgresTracer struct {
	Logger   *Logger
	Database string
}

type sqlExecution struct {
	start  time.Time
	detail map[string]string
}

func (t PostgresTracer) execution(ctx context.Context, query string, args int) sqlExecution {
	// Arbitrary SQL can contain credentials in literals, identifiers, and
	// comments. Keep only a known operation and a query fingerprint.
	operation := "SQL"
	if words := strings.Fields(query); len(words) > 0 {
		switch word := strings.ToUpper(words[0]); word {
		case "SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "ALTER", "DROP",
			"BEGIN", "COMMIT", "ROLLBACK", "EXPLAIN", "WITH", "COPY":
			operation = word
		}
	}
	detail := map[string]string{
		"database":          t.Database,
		"query":             operation,
		"query_fingerprint": fmt.Sprintf("%x", sha256.Sum256([]byte(query))),
		"parameter_count":   strconv.Itoa(args),
		"parameters":        "[redacted]",
	}
	if id, ok := ctx.Value(migrationIDKey{}).(string); ok {
		detail["migration_id"] = id
	}
	return sqlExecution{start: time.Now(), detail: detail}
}

func (t PostgresTracer) log(ctx context.Context, action, status string, detail map[string]string, err error) {
	actor, _ := ctx.Value(sqlActorKey{}).(string)
	if actor == "" {
		actor = "unattributed"
	}
	t.Logger.Log(ctx, Event{UserID: actor, Action: action, Status: status, Detail: detail, Err: err})
}

func (t PostgresTracer) finish(ctx context.Context, execution sqlExecution, action string, tag pgconn.CommandTag, err error) {
	detail := execution.detail
	detail["duration_us"] = strconv.FormatInt(time.Since(execution.start).Microseconds(), 10)
	if tag.String() != "" {
		detail["command"] = strings.Fields(tag.String())[0]
		detail["rows"] = strconv.FormatInt(tag.RowsAffected(), 10)
	}
	status := StatusSuccess
	var safeErr error
	if err != nil {
		status = StatusFailed
		safeErr = errors.New("database execution failed")
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			detail["sqlstate"] = pgErr.SQLState()
		}
	}
	t.log(ctx, action, status, detail, safeErr)
}

func (t PostgresTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	execution := t.execution(ctx, data.SQL, len(data.Args))
	ctx, span := StartSpan(ctx, "db."+execution.detail["query"])
	span.SetAttributes(attribute.String("db.namespace", t.Database), attribute.String("db.operation.name", execution.detail["query"]))
	t.log(ctx, "sql.start", "started", execution.detail, nil)
	return context.WithValue(ctx, sqlExecutionKey{}, execution)
}

func (t PostgresTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	defer EndSpan(trace.SpanFromContext(ctx), data.Err != nil)
	execution := ctx.Value(sqlExecutionKey{}).(sqlExecution)
	action := "sql.result"
	switch execution.detail["query"] {
	case "COMMIT":
		action = "sql.commit"
	case "ROLLBACK":
		action = "sql.rollback"
	}
	t.finish(ctx, execution, action, data.CommandTag, data.Err)
}

type sqlBatchKey struct{}
type sqlBatch struct {
	start time.Time
	count int
}

func (t PostgresTracer) TraceBatchStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchStartData) context.Context {
	ctx, span := StartSpan(ctx, "db.batch")
	span.SetAttributes(attribute.String("db.namespace", t.Database))
	start := time.Now()
	for index, query := range data.Batch.QueuedQueries {
		execution := t.execution(ctx, query.SQL, len(query.Arguments))
		execution.detail["batch_position"] = strconv.Itoa(index + 1)
		t.log(ctx, "sql.batch.queued", "started", execution.detail, nil)
	}
	return context.WithValue(ctx, sqlBatchKey{}, &sqlBatch{start: start})
}

func (t PostgresTracer) TraceBatchQuery(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchQueryData) {
	batch := ctx.Value(sqlBatchKey{}).(*sqlBatch)
	batch.count++
	execution := t.execution(ctx, data.SQL, len(data.Args))
	execution.start = batch.start
	execution.detail["batch_position"] = strconv.Itoa(batch.count)
	t.finish(ctx, execution, "sql.batch.query_result", data.CommandTag, data.Err)
}

func (t PostgresTracer) TraceBatchEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchEndData) {
	defer EndSpan(trace.SpanFromContext(ctx), data.Err != nil)
	batch := ctx.Value(sqlBatchKey{}).(*sqlBatch)
	execution := sqlExecution{start: batch.start, detail: map[string]string{
		"database":         t.Database,
		"reported_queries": strconv.Itoa(batch.count),
	}}
	t.finish(ctx, execution, "sql.batch.result", pgconn.CommandTag{}, data.Err)
}
