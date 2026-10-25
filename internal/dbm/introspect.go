package dbm

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

type ColumnInfo struct {
	Name     string
	Type     string
	Nullable bool
	Default  *string
}

type ConstraintInfo struct {
	Name      string
	Type      string
	Columns   string
	Reference string
}

type IndexInfo struct {
	Name    string
	Columns string
	Unique  bool
	Method  string
}

// validasi tabel mencegah injection via nama skema/tabel dari URL.
func validateTable(ctx context.Context, t *Target, schema, table string) (bool, error) {
	var ok bool
	err := t.DB.QueryRowContext(ctx, t.Dialect.Queries().Validate, schema, table).Scan(&ok)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func ListColumns(ctx context.Context, t *Target, schema, table string) ([]ColumnInfo, error) {
	rows, err := t.DB.QueryContext(ctx, t.Dialect.Queries().Columns, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ColumnInfo
	for rows.Next() {
		var c ColumnInfo
		var def sql.NullString
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &def); err != nil {
			return nil, err
		}
		if def.Valid {
			c.Default = &def.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func ListConstraints(ctx context.Context, t *Target, schema, table string) ([]ConstraintInfo, error) {
	rows, err := t.DB.QueryContext(ctx, t.Dialect.Queries().Constraints, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ConstraintInfo
	for rows.Next() {
		var c ConstraintInfo
		if err := rows.Scan(&c.Name, &c.Type, &c.Columns, &c.Reference); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func ListIndexes(ctx context.Context, t *Target, schema, table string) ([]IndexInfo, error) {
	rows, err := t.DB.QueryContext(ctx, t.Dialect.Queries().Indexes, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []IndexInfo
	for rows.Next() {
		var i IndexInfo
		if err := rows.Scan(&i.Name, &i.Columns, &i.Unique, &i.Method); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func CountRows(ctx context.Context, t *Target, schema, table string) (int64, error) {
	var n int64
	err := t.DB.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %s`, t.QuoteTable(schema, table))).Scan(&n)
	return n, err
}

func ListRows(ctx context.Context, t *Target, schema, table string, limit, offset int) ([]string, [][]string, error) {
	rows, err := t.DB.QueryContext(ctx,
		fmt.Sprintf(t.Dialect.Queries().Rows, t.QuoteTable(schema, table)), limit, offset)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	var out [][]string
	for rows.Next() {
		// Nilai di-scan ke any; buffer []byte driver disalin dulu supaya
		// tidak berubah saat baris berikutnya dibaca.
		vals := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		line := make([]string, len(columns))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			line[i] = formatValue(v)
		}
		out = append(out, line)
	}
	return columns, out, rows.Err()
}

func formatValue(v any) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case []byte:
		return string(val)
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case int32:
		return strconv.Itoa(int(val))
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// TableSummary = ringkasan satu tabel untuk halaman schema
// (nama, jumlah kolom, perkiraan baris, primary key).
type TableSummary struct {
	Schema      string
	Name        string
	Columns     int
	RowEstimate int64
	PK          string // kolom primary key, "" bila tidak ada
}

func ListTableSummaries(ctx context.Context, t *Target) ([]TableSummary, error) {
	q := t.Dialect.Queries()
	rows, err := t.DB.QueryContext(ctx, q.Summaries)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TableSummary
	for rows.Next() {
		var s TableSummary
		if err := rows.Scan(&s.Schema, &s.Name, &s.Columns, &s.RowEstimate); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Primary key per tabel (query terpisah; hasil digabung ke ringkasan).
	pkRows, err := t.DB.QueryContext(ctx, q.PKs)
	if err != nil {
		return out, nil // ringkasan tetap dipakai walau PK gagal
	}
	defer pkRows.Close()

	pk := map[string]string{}
	for pkRows.Next() {
		var schema, table, cols string
		if err := pkRows.Scan(&schema, &table, &cols); err != nil {
			break
		}
		pk[schema+"."+table] = cols
	}
	for i := range out {
		out[i].PK = pk[out[i].Schema+"."+out[i].Name]
	}
	return out, nil
}

// ForeignKey = satu relasi foreign key untuk kartu "Relasi (Foreign Key)".
type ForeignKey struct {
	Name   string
	From   string // tabel.asal (kolom)
	ToRef  string // skema.tujuan (kolom)
}

func ListForeignKeys(ctx context.Context, t *Target) ([]ForeignKey, error) {
	rows, err := t.DB.QueryContext(ctx, t.Dialect.Queries().ForeignKeys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ForeignKey
	for rows.Next() {
		var f ForeignKey
		if err := rows.Scan(&f.Name, &f.From, &f.ToRef); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
