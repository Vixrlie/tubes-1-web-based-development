package dbm

// Analisa rencana eksekusi (EXPLAIN) untuk halaman Preview migration.
// EXPLAIN biasa tidak mengeksekusi statement; EXPLAIN ANALYZE mengeksekusi,
// jadi selalu dijalankan di dalam transaksi yang di-ROLLBACK sehingga tidak
// ada perubahan permanen pada database target.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

)

// ExplainItem = hasil analisa satu statement.
type ExplainItem struct {
	N       int        
	Stmt    string     
	Kind    string     
	Note    string     
	Summary PlanSummary
	Tree    string     
	Analyze bool       
}

// PlanSummary = ringkasan execution plan
type PlanSummary struct {
	Scan       string
	Index      string
	Join       string
	Cost       string
	ActualTime string
	Rows       string
}

const ddlNoPlanNote = "Statement DDL/utility tidak memiliki execution plan."

// ExplainAll menganalisa seluruh statement dalam satu SQL migration.
func ExplainAll(ctx context.Context, pool *Target, sqlText string, analyze bool) []ExplainItem {
	stmts := SplitStatements(sqlText)
	items := make([]ExplainItem, 0, len(stmts))
	for i, stmt := range stmts {
		items = append(items, Explain(ctx, pool, i+1, stmt, analyze))
	}
	return items
}

// Explain menganalisa satu statement. DDL dilewati tanpa query (memang tidak
// punya plan), statement lain dijelaskan lewat EXPLAIN [ANALYZE] FORMAT JSON.
func Explain(ctx context.Context, pool *Target, n int, stmt string, analyze bool) ExplainItem {
	item := ExplainItem{N: n, Stmt: shortStmt(stmt), Kind: stmtKind(stmt), Analyze: analyze}
	if item.Kind == "ddl" {
		item.Note = ddlNoPlanNote
		return item
	}

	query := pool.Dialect.Explain(analyze, stmt)

	var raw []byte
	var err error
	if analyze {
		// ANALYZE benar-benar eksekusi statement dmn transaksi dibuka hanya
		// untuk ini lalu selalu di-ROLLBACK (defer), termasuk bila gagal.
		tx, txErr := pool.DB.BeginTx(ctx, nil)
		if txErr != nil {
			item.Note = "gagal memulai transaksi: " + sanitizePGError(txErr)
			return item
		}
		defer tx.Rollback()
		err = tx.QueryRowContext(ctx, query).Scan(&raw)
	} else {
		err = pool.DB.QueryRowContext(ctx, query).Scan(&raw)
	}
	if err != nil {
		item.Note = "tidak memiliki execution plan: " + sanitizePGError(err)
		return item
	}

	var payload []struct {
		Plan map[string]any `json:"Plan"`
	}
	if !pool.Dialect.ExplainIsJSON(analyze) ||
		json.Unmarshal(raw, &payload) != nil ||
		len(payload) == 0 || payload[0].Plan == nil {
		// Output tidak dikenali (mis. EXPLAIN ANALYZE MySQL yang
		// berupa pohon teks), tampilkan apa adanya, jangan mengarang ringkasan.
		item.Tree = strings.TrimSpace(string(raw)) + "\n"
		item.Note = "Ringkasan otomatis tidak tersedia untuk DBMS ini; ditampilkan keluaran mentah EXPLAIN."
		return item
	}
	root := payload[0].Plan
	item.Tree = formatPlan(root, 0)
	item.Summary = extractSummary(root, analyze)
	return item
}

// SplitStatements memecah SQL per ';' yang sadar string tunggal,
// dollar-quoting, dan komentar supaya ';' di dalam literal tidak ikut membelah statement.
func SplitStatements(sqlText string) []string {
	var out []string
	var b strings.Builder
	var inSingle, inDouble, inLine, inBlock bool
	dollarTag := ""

	flush := func() {
		if stmt := strings.TrimSpace(b.String()); stmt != "" && hasSQL(stmt) {
			out = append(out, stmt)
		}
		b.Reset()
	}

	for i := 0; i < len(sqlText); i++ {
		c := sqlText[i]

		if dollarTag != "" {
			if strings.HasPrefix(sqlText[i:], dollarTag) {
				b.WriteString(dollarTag)
				i += len(dollarTag) - 1
				dollarTag = ""
				continue
			}
			b.WriteByte(c)
			continue
		}
		if inLine {
			b.WriteByte(c)
			if c == '\n' {
				inLine = false
			}
			continue
		}
		if inBlock {
			b.WriteByte(c)
			if c == '*' && i+1 < len(sqlText) && sqlText[i+1] == '/' {
				b.WriteByte('/')
				i++
				inBlock = false
			}
			continue
		}
		if inSingle {
			b.WriteByte(c)
			if c == '\'' {
				if i+1 < len(sqlText) && sqlText[i+1] == '\'' {
					b.WriteByte('\'')
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			b.WriteByte(c)
			if c == '"' {
				inDouble = false
			}
			continue
		}

		switch {
		case c == '-' && i+1 < len(sqlText) && sqlText[i+1] == '-':
			b.WriteString("--")
			i++
			inLine = true
		case c == '/' && i+1 < len(sqlText) && sqlText[i+1] == '*':
			b.WriteString("/*")
			i++
			inBlock = true
		case c == '\'':
			b.WriteByte(c)
			inSingle = true
		case c == '"':
			b.WriteByte(c)
			inDouble = true
		case c == '$':
			if tag := dollarTagAt(sqlText, i); tag != "" {
				b.WriteString(tag)
				i += len(tag) - 1
				dollarTag = tag
			} else {
				b.WriteByte(c)
			}
		case c == ';':
			flush()
		default:
			b.WriteByte(c)
		}
	}
	flush()
	return out
}

// dollarTagAt mengembalikan tag dollar-quoting ("$tag$", "$$") pada posisi i,
// atau "" bila bukan (mis. parameter $1).
func dollarTagAt(s string, i int) string {
	for j := i + 1; j < len(s); j++ {
		if s[j] == '$' {
			return s[i : j+1]
		}
		if !(unicode.IsLetter(rune(s[j])) || unicode.IsDigit(rune(s[j])) || s[j] == '_') {
			return ""
		}
	}
	return ""
}

// hasSQL: true bila masih ada isi statement setelah komentar & spasi dibuang.
func hasSQL(stmt string) bool {
	for i := 0; i < len(stmt); i++ {
		c := stmt[i]
		if c == '-' && i+1 < len(stmt) && stmt[i+1] == '-' {
			for i < len(stmt) && stmt[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(stmt) && stmt[i+1] == '*' {
			if j := strings.Index(stmt[i+2:], "*/"); j >= 0 {
				i += j + 3
				continue
			}
			return false
		}
		if !unicode.IsSpace(rune(c)) {
			return true
		}
	}
	return false
}

// leadingKeyword = kata pertama statement dalam huruf besar (lewati komentar).
func leadingKeyword(stmt string) string {
	i := 0
	for i < len(stmt) {
		c := stmt[i]
		if c == '-' && i+1 < len(stmt) && stmt[i+1] == '-' {
			for i < len(stmt) && stmt[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(stmt) && stmt[i+1] == '*' {
			j := strings.Index(stmt[i+2:], "*/")
			if j < 0 {
				return ""
			}
			i += j + 4
			continue
		}
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}
		break
	}
	j := i
	for j < len(stmt) && unicode.IsLetter(rune(stmt[j])) {
		j++
	}
	return strings.ToUpper(stmt[i:j])
}

// stmtKind mengelompokkan statement: "query" (boleh di-EXPLAIN), "ddl"
// (utility, tanpa plan, dilewati), "lainnya" (dicoba, galat ditampilkan jujur).
func stmtKind(stmt string) string {
	switch leadingKeyword(stmt) {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "WITH", "VALUES", "TABLE", "MERGE":
		return "query"
	case "CREATE", "ALTER", "DROP", "TRUNCATE", "COMMENT", "GRANT", "REVOKE",
		"SET", "SHOW", "BEGIN", "START", "COMMIT", "ROLLBACK", "SAVEPOINT",
		"RELEASE", "VACUUM", "ANALYZE", "REINDEX", "CLUSTER", "REFRESH", "COPY",
		"LISTEN", "NOTIFY", "DISCARD", "LOCK", "PREPARE", "EXECUTE", "DEALLOCATE",
		"CALL", "DO":
		return "ddl"
	default:
		return "lainnya"
	}
}

// shortStmt memadatkan statement untuk tampilan tabel (maks 72 karakter).
func shortStmt(stmt string) string {
	flat := strings.Join(strings.Fields(stmt), " ")
	if len(flat) > 72 {
		return flat[:72] + "…"
	}
	return flat
}

// extractSummary mengambil info dari akar plan + satu kali telusur.
func extractSummary(root map[string]any, analyze bool) PlanSummary {
	s := PlanSummary{
		Scan:  "-",
		Index: "-",
		Join:  "-",
		Cost:  fmt.Sprintf("%.2f", numField(root["Total Cost"])),
		Rows:  fmt.Sprintf("%d (est)", int64(numField(root["Plan Rows"]))),
	}
	if analyze {
		s.ActualTime = fmt.Sprintf("%.2f", numField(root["Actual Total Time"]))
		s.Rows = fmt.Sprintf("%d est / %d akt",
			int64(numField(root["Plan Rows"])), int64(numField(root["Actual Rows"])))
	}
	var walk func(n map[string]any)
	walk = func(n map[string]any) {
		node := strField(n["Node Type"])
		if s.Scan == "-" && strings.Contains(node, "Scan") {
			s.Scan = node
		}
		if s.Index == "-" {
			if idx := strField(n["Index Name"]); idx != "" {
				s.Index = idx
			}
		}
		if s.Join == "-" && strings.Contains(node, "Join") {
			s.Join = node
		}
		if children, ok := n["Plans"].([]any); ok {
			for _, child := range children {
				if m, ok := child.((map[string]any)); ok {
					walk(m)
				}
			}
		}
	}
	walk(root)
	return s
}

// formatPlan menyusun teks plan bergaya EXPLAIN teks bawaan PostgreSQL.
func formatPlan(n map[string]any, depth int) string {
	var b strings.Builder
	formatPlanInto(&b, n, depth)
	return b.String()
}

func formatPlanInto(b *strings.Builder, n map[string]any, depth int) {
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString(strField(n["Node Type"]))
	if rel := strField(n["Relation Name"]); rel != "" {
		b.WriteString(" on " + rel)
	}
	if idx := strField(n["Index Name"]); idx != "" {
		b.WriteString(" using " + idx)
	}
	b.WriteString(fmt.Sprintf("  (cost=%.2f..%.2f rows=%d width=%d",
		numField(n["Startup Cost"]), numField(n["Total Cost"]),
		int64(numField(n["Plan Rows"])), int64(numField(n["Plan Width"]))))
	if actual, ok := n["Actual Total Time"]; ok && actual != nil {
		b.WriteString(fmt.Sprintf(" actual time=%.2f..%.2f rows=%d loops=%d",
			numField(n["Actual Startup Time"]), numField(n["Actual Total Time"]),
			int64(numField(n["Actual Rows"])), int64(numField(n["Actual Loops"]))))
	}
	if join := strField(n["Join Type"]); join != "" {
		b.WriteString(" " + join)
	}
	b.WriteString(")\n")

	if children, ok := n["Plans"].([]any); ok {
		for _, child := range children {
			if m, ok := child.(map[string]any); ok {
				formatPlanInto(b, m, depth+1)
			}
		}
	}
}

func numField(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	}
	return 0
}

func strField(v any) string {
	s, _ := v.(string)
	return s
}
