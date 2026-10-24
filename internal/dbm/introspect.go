package dbm

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TableInfo struct {
	Schema      string
	Name        string
	RowEstimate int64
}

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
func validateTable(ctx context.Context, pool *pgxpool.Pool, schema, table string) (bool, error) {
	var ok bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = $2 AND table_type = 'BASE TABLE'
		)`, schema, table).Scan(&ok)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func quoteTable(schema, table string) string {
	return pgx.Identifier{schema, table}.Sanitize()
}

func ListSchemasTables(ctx context.Context, pool *pgxpool.Pool) ([]TableInfo, error) {
	rows, err := pool.Query(ctx, `
		SELECT t.table_schema, t.table_name, COALESCE(pgc.reltuples::bigint, 0)
		FROM information_schema.tables t
		LEFT JOIN pg_class pgc ON pgc.relname = t.table_name
		LEFT JOIN pg_namespace pgn ON pgn.oid = pgc.relnamespace AND pgn.nspname = t.table_schema
		WHERE t.table_type = 'BASE TABLE'
		  AND t.table_schema NOT IN ('pg_catalog', 'information_schema')
		ORDER BY t.table_schema, t.table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Schema, &t.Name, &t.RowEstimate); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func ListColumns(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]ColumnInfo, error) {
	rows, err := pool.Query(ctx, `
		SELECT column_name, data_type, is_nullable = 'YES', column_default
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`, schema, table)
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

func ListConstraints(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]ConstraintInfo, error) {
	rows, err := pool.Query(ctx, `
		SELECT tc.constraint_name, tc.constraint_type, kcu.columns,
		       COALESCE(fk.ref, '') AS reference
		FROM information_schema.table_constraints tc
		LEFT JOIN (
			SELECT constraint_name, table_schema, table_name,
			       string_agg(column_name, ', ' ORDER BY ordinal_position) AS columns
			FROM information_schema.key_column_usage
			GROUP BY constraint_name, table_schema, table_name
		) kcu ON kcu.constraint_name = tc.constraint_name
		   AND kcu.table_schema = tc.table_schema
		   AND kcu.table_name = tc.table_name
		LEFT JOIN (
			SELECT con.conname, ns.nspname, cl.relname,
			       nsref.nspname || '.' || clref.relname || '(' || atref.attname || ')' AS ref
			FROM pg_constraint con
			JOIN pg_class cl ON cl.oid = con.conrelid
			JOIN pg_namespace ns ON ns.oid = cl.relnamespace
			LEFT JOIN pg_class clref ON clref.oid = con.confrelid
			LEFT JOIN pg_namespace nsref ON nsref.oid = clref.connamespace
			LEFT JOIN pg_attribute atref ON atref.attrelid = con.confrelid AND atref.attnum = ANY(con.confkey)
			WHERE con.contype = 'f'
		) fk ON fk.conname = tc.constraint_name
		   AND fk.nspname = tc.table_schema
		   AND fk.relname = tc.table_name
		WHERE tc.table_schema = $1 AND tc.table_name = $2
		ORDER BY tc.constraint_type, tc.constraint_name`, schema, table)
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

func ListIndexes(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]IndexInfo, error) {
	rows, err := pool.Query(ctx, `
		SELECT ic.relname AS name,
		       string_agg(a.attname, ', ' ORDER BY k.ordinal_position) AS columns,
		       ix.indisunique AS unique,
		       am.amname AS method
		FROM pg_index ix
		JOIN pg_class ic ON ic.oid = ix.indexrelid
		JOIN pg_class t ON t.oid = ix.indrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_am am ON am.oid = ic.relam
		JOIN LATERAL unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ordinal) ON true
		JOIN pg_attribute a ON a.attrelid = ix.indexrelid AND a.attnum = k.attnum
		WHERE n.nspname = $1 AND t.relname = $2 AND ix.indisprimary = false AND a.attnum > 0
		GROUP BY ic.indexrelid, ix.indisunique, am.amname
		ORDER BY name`, schema, table)
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

func CountRows(ctx context.Context, pool *pgxpool.Pool, schema, table string) (int64, error) {
	var n int64
	err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s`, quoteTable(schema, table))).Scan(&n)
	return n, err
}

func ListRows(ctx context.Context, pool *pgxpool.Pool, schema, table string, limit, offset int) ([]string, [][]string, error) {
	rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT * FROM %s LIMIT $1 OFFSET $2`,
		quoteTable(schema, table)), limit, offset)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	columns := make([]string, 0, len(fields))
	for _, f := range fields {
		columns = append(columns, string(f.Name))
	}

	var out [][]string
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, nil, err
		}
		line := make([]string, len(values))
		for i, v := range values {
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

func ListTableSummaries(ctx context.Context, pool *pgxpool.Pool) ([]TableSummary, error) {
	rows, err := pool.Query(ctx, `
		SELECT t.table_schema, t.table_name, COUNT(c.column_name),
		       COALESCE(pgc.reltuples, 0)::bigint
		FROM information_schema.tables t
		LEFT JOIN information_schema.columns c
		  ON c.table_schema = t.table_schema AND c.table_name = t.table_name
		LEFT JOIN pg_class pgc ON pgc.relname = t.table_name
		LEFT JOIN pg_namespace pgn ON pgn.oid = pgc.relnamespace AND pgn.nspname = t.table_schema
		WHERE t.table_type = 'BASE TABLE'
		  AND t.table_schema NOT IN ('pg_catalog', 'information_schema')
		GROUP BY t.table_schema, t.table_name, pgc.reltuples
		ORDER BY t.table_schema, t.table_name`)
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
	pkRows, err := pool.Query(ctx, `
		SELECT n.nspname, c.relname, string_agg(a.attname, ', ' ORDER BY u.ord)
		FROM pg_constraint con
		JOIN pg_class c ON c.oid = con.conrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN unnest(con.conkey) WITH ORDINALITY AS u(attnum, ord) ON true
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = u.attnum
		WHERE con.contype = 'p'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		GROUP BY n.nspname, c.relname`)
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

func ListForeignKeys(ctx context.Context, pool *pgxpool.Pool) ([]ForeignKey, error) {
	rows, err := pool.Query(ctx, `
		SELECT con.conname,
		       (n.nspname || '.' || rel.relname || '(' ||
		        (SELECT string_agg(a.attname, ', ' ORDER BY u.ord)
		           FROM unnest(con.conkey) WITH ORDINALITY AS u(attnum, ord)
		           JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = u.attnum
		        ) || ')'),
		       (rn.nspname || '.' || rrel.relname || '(' ||
		        (SELECT string_agg(a.attname, ', ' ORDER BY u.ord)
		           FROM unnest(con.confkey) WITH ORDINALITY AS u(attnum, ord)
		           JOIN pg_attribute a ON a.attrelid = con.confrelid AND a.attnum = u.attnum
		        ) || ')')
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		JOIN pg_namespace n ON n.oid = rel.relnamespace
		JOIN pg_class rrel ON rrel.oid = con.confrelid
		JOIN pg_namespace rn ON rn.oid = rrel.relnamespace
		WHERE con.contype = 'f'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY con.conname`)
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
