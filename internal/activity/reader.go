package activity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrActivityNotFound = errors.New("activity not found")
var ErrActivityUnavailable = errors.New("activity unavailable")
var eventIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,18}-[a-f0-9]{16}$`)

const activityLimit = 5000

// Entry exposes only the structured, redacted fields used by the SPA.
type Entry struct {
	ID          string            `json:"id"`
	Time        string            `json:"time"`
	Action      string            `json:"action"`
	Status      string            `json:"status"`
	UserID      string            `json:"userId"`
	RequestID   string            `json:"requestId"`
	TraceID     string            `json:"traceId,omitempty"`
	SpanID      string            `json:"spanId,omitempty"`
	Database    string            `json:"database,omitempty"`
	MigrationID string            `json:"migrationId,omitempty"`
	Detail      map[string]string `json:"detail"`
}

type ActivityFilter struct {
	Search, Status, Category string
	Start, End               time.Time
	Page, PageSize           int
}
type ActivityPage struct {
	Items    []Entry `json:"items"`
	Total    int     `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
	Limited  bool    `json:"limited"`
	From     string  `json:"from"`
	To       string  `json:"to"`
}
type ActivityReader interface {
	List(context.Context, ActivityFilter) (ActivityPage, error)
	Get(context.Context, string) (Entry, error)
}

type LokiReader struct {
	base   string
	client *http.Client
}

func NewLokiReader(base string) (*LokiReader, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("LOKI_URL harus berupa URL HTTP(S) tanpa credentials, query atau fragment")
	}
	return &LokiReader{base: strings.TrimRight(base, "/"), client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func activityQuery(f ActivityFilter) string {
	query := `{service_name="pathgres"} | json | __error__="" | action!=""`
	if f.Status != "" && f.Status != "all" {
		query += " | status=" + strconv.Quote(f.Status)
	}
	if f.Category != "" && f.Category != "all" {
		query += " | action=~" + strconv.Quote(regexp.QuoteMeta(f.Category)+`\..*`)
	}
	if f.Search != "" {
		query += " |~ " + strconv.Quote("(?i)"+regexp.QuoteMeta(f.Search))
	}
	return query
}

func (l *LokiReader) entries(ctx context.Context, f ActivityFilter) ([]Entry, bool, error) {
	q := url.Values{"query": {activityQuery(f)}, "start": {strconv.FormatInt(f.Start.UnixNano(), 10)}, "end": {strconv.FormatInt(f.End.UnixNano(), 10)}, "direction": {"backward"}, "limit": {strconv.Itoa(activityLimit)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.base+"/loki/api/v1/query_range?"+q.Encode(), nil)
	if err != nil {
		return nil, false, ErrActivityUnavailable
	}
	res, err := l.client.Do(req)
	if err != nil {
		return nil, false, ErrActivityUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, false, ErrActivityUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 16*1024*1024+1))
	if err != nil || len(body) > 16*1024*1024 {
		return nil, false, ErrActivityUnavailable
	}
	var response struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Values [][]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &response) != nil || response.Status != "success" || response.Data.ResultType != "streams" {
		return nil, false, ErrActivityUnavailable
	}
	rows := make([]Entry, 0)
	seen := make(map[string]bool)
	count := 0
	for _, stream := range response.Data.Result {
		for _, value := range stream.Values {
			count++
			if count > activityLimit || len(value) != 2 {
				return nil, false, ErrActivityUnavailable
			}
			entry, ok := parseLogEntry(value[0], value[1])
			if ok && !seen[entry.ID] {
				rows = append(rows, entry)
				seen[entry.ID] = true
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Time == rows[j].Time {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].Time > rows[j].Time
	})
	return rows, count == activityLimit, nil
}

func (l *LokiReader) List(ctx context.Context, f ActivityFilter) (ActivityPage, error) {
	rows, limited, err := l.entries(ctx, f)
	if err != nil {
		return ActivityPage{}, err
	}
	if f.PageSize < 1 || f.PageSize > 100 || f.Page < 1 {
		return ActivityPage{}, errors.New("invalid pagination")
	}
	pages := (len(rows) + f.PageSize - 1) / f.PageSize
	if pages < 1 {
		pages = 1
	}
	if f.Page > pages {
		f.Page = pages
	}
	start := (f.Page - 1) * f.PageSize
	end := start + f.PageSize
	if end > len(rows) {
		end = len(rows)
	}
	return ActivityPage{Items: rows[start:end], Total: len(rows), Page: f.Page, PageSize: f.PageSize, Limited: limited, From: f.Start.Format(time.RFC3339Nano), To: f.End.Format(time.RFC3339Nano)}, nil
}

func (l *LokiReader) Get(ctx context.Context, id string) (Entry, error) {
	if !eventIDPattern.MatchString(id) {
		return Entry{}, ErrActivityNotFound
	}
	stamp, err := strconv.ParseInt(strings.SplitN(id, "-", 2)[0], 10, 64)
	if err != nil || stamp <= 0 || stamp == int64(^uint64(0)>>1) {
		return Entry{}, ErrActivityNotFound
	}
	rows, _, err := l.entries(ctx, ActivityFilter{Start: time.Unix(0, stamp), End: time.Unix(0, stamp+1)})
	if err != nil {
		return Entry{}, err
	}
	for _, row := range rows {
		if row.ID == id {
			return row, nil
		}
	}
	return Entry{}, ErrActivityNotFound
}

func parseLogEntry(stamp, line string) (Entry, bool) {
	ns, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || ns <= 0 {
		return Entry{}, false
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(line), &raw) != nil {
		return Entry{}, false
	}
	str := func(key string) string {
		var s string
		_ = json.Unmarshal(raw[key], &s)
		if len(s) > 2048 {
			return s[:2048]
		}
		return s
	}
	status := str("status")
	if str("action") == "" || (status != "success" && status != "warning" && status != "failed" && status != "started") {
		return Entry{}, false
	}
	hash := sha256.Sum256([]byte(line))
	e := Entry{ID: fmt.Sprintf("%s-%s", stamp, hex.EncodeToString(hash[:8])), Time: time.Unix(0, ns).UTC().Format("2006-01-02T15:04:05.000000000Z"), Action: str("action"), Status: status, UserID: str("user_id"), RequestID: str("request_id"), TraceID: str("trace_id"), SpanID: str("span_id"), Database: str("database"), MigrationID: str("migration_id"), Detail: map[string]string{}}
	for _, key := range []string{"method", "route", "http_status", "resource_id", "target_user_id", "query_fingerprint", "parameter_count", "duration_us", "rows", "command", "sqlstate", "batch_position", "reported_queries", "duration"} {
		if value := str(key); value != "" {
			e.Detail[key] = value
		}
	}
	if _, ok := raw["parameters"]; ok {
		e.Detail["parameters"] = "[redacted]"
	}
	if _, ok := raw["query"]; ok {
		op := str("query")
		switch op {
		case "SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "ALTER", "DROP", "BEGIN", "COMMIT", "ROLLBACK", "EXPLAIN", "WITH", "COPY", "SQL":
			e.Detail["query"] = op
		default:
			e.Detail["query"] = "[redacted]"
		}
	}
	if _, ok := raw["error"]; ok {
		e.Detail["error"] = "operation failed"
	}
	return e, true
}
