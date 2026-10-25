package dbm

// Target = koneksi ke satu database target berbagai DBMS. DB memakai
// database/sql (driver "pgx" stdlib untuk PostgreSQL, go-sql-driver/mysql
// untuk MySQL/MariaDB); selisih SQL antar DBMS (pengutipan, kueri
// introspeksi, kunci transaksi, EXPLAIN) terkapsulasi di Dialect.

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	gomysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx" untuk database/sql
)

// Target = pasangan koneksi + dialect untuk satu database target.
type Target struct {
	DB      *sql.DB
	Dialect Dialect
}

// Close menutup seluruh koneksi target.
func (t *Target) Close() {
	if t != nil && t.DB != nil {
		t.DB.Close()
	}
}

// QuoteTable mengutip "schema.tabel" sesuai dialect.
func (t *Target) QuoteTable(schema, table string) string {
	return t.Dialect.QuoteIdent(schema, table)
}

// Dialect = perbedaan SQL antar DBMS target.
type Dialect interface {
	// Name mengembalikan "postgres" | "mysql".
	Name() string
	// OpenArgs mengubah URL koneksi menjadi (nama driver, DSN) database/sql.
	OpenArgs(rawURL string) (driver, dsn string, err error)
	// QuoteIdent mengutip identifier ("a"."b" vs `a`.`b`).
	QuoteIdent(parts ...string) string
	// LockSQL/UnlockSQL = kunci per-koneksi setara advisory lock ("" bila tidak ada).
	LockSQL(connID int64) string
	UnlockSQL(connID int64) string
	// Explain membangun perintah EXPLAIN untuk satu statement.
	Explain(analyze bool, stmt string) string
	// ExplainIsJSON: true bila keluaran EXPLAIN berupa JSON yang bisa diparse
	// menjadi ringkasan; false = tampilkan keluaran mentah.
	ExplainIsJSON(analyze bool) bool
	// Queries berisi teks kueri introspeksi (bentuk hasil scan identik antar dialect).
	Queries() Queries
}

// Queries = kueri introspeksi per dialect. Placeholder mengikuti driver:
// "$n" untuk PostgreSQL, "?" untuk MySQL.
type Queries struct {
	Summaries   string // (schema, tabel, jumlah kolom, perkiraan baris)
	PKs         string // (schema, tabel, kolom PK)
	Columns     string // arg: schema, tabel
	Indexes     string // arg: schema, tabel
	Constraints string // arg: schema, tabel
	ForeignKeys string // tanpa argumen
	Validate    string // arg: schema, tabel -> EXISTS (bool)
	CountTables string // jumlah tabel BASE (tanpa argumen)
	Rows        string // "SELECT * FROM %s LIMIT .." (%s = tabel terutip)
}

// OpenTarget membuka koneksi target dari URL tersimpan (db_connections.connection_url).
// Skema URL menentukan DBMS: postgres:// atau mysql://.
func OpenTarget(ctx context.Context, rawURL string) (*Target, error) {
	if rawURL == "" {
		return nil, ErrConnNotFound
	}
	var d Dialect
	switch urlScheme(rawURL) {
	case "postgres", "postgresql":
		d = Postgres{}
	case "mysql":
		d = MySQL{}
	default:
		return nil, fmt.Errorf("skema database belum didukung: %q", urlScheme(rawURL))
	}
	driver, dsn, err := d.OpenArgs(rawURL)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, err
	}
	return &Target{DB: db, Dialect: d}, nil
}

func urlScheme(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme)
}

// --- PostgreSQL -------------------------------------------------------------

// Postgres = dialect PostgreSQL. Kueri dipertahankan seperti versi pgx
// sebelum refactor multi-DBMS.
type Postgres struct{}

func (Postgres) Name() string { return "postgres" }

func (Postgres) OpenArgs(rawURL string) (string, string, error) {
	return "pgx", rawURL, nil
}

func (Postgres) QuoteIdent(parts ...string) string {
	return pgx.Identifier(parts).Sanitize()
}

func (Postgres) LockSQL(connID int64) string {
	return fmt.Sprintf("SELECT pg_advisory_xact_lock(%d)", connID)
}

func (Postgres) UnlockSQL(int64) string { return "" }

func (Postgres) Explain(analyze bool, stmt string) string {
	if analyze {
		return "EXPLAIN (ANALYZE, FORMAT JSON) " + stmt
	}
	return "EXPLAIN (FORMAT JSON) " + stmt
}

func (Postgres) ExplainIsJSON(bool) bool { return true }

func (Postgres) Queries() Queries {
	return Queries{
		Summaries: `
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
		ORDER BY t.table_schema, t.table_name`,
		PKs: `
		SELECT n.nspname, c.relname, string_agg(a.attname, ', ' ORDER BY u.ord)
		FROM pg_constraint con
		JOIN pg_class c ON c.oid = con.conrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN unnest(con.conkey) WITH ORDINALITY AS u(attnum, ord) ON true
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = u.attnum
		WHERE con.contype = 'p'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		GROUP BY n.nspname, c.relname`,
		Columns: `
		SELECT column_name, data_type, is_nullable = 'YES', column_default
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`,
		Indexes: `
		SELECT ic.relname AS name,
		       string_agg(a.attname, ', ' ORDER BY k.ordinal) AS columns,
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
		GROUP BY ic.oid, ic.relname, ix.indisunique, am.amname
		ORDER BY name`,
		Constraints: `
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
		ORDER BY tc.constraint_type, tc.constraint_name`,
		ForeignKeys: `
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
		ORDER BY con.conname`,
		Validate: `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = $2 AND table_type = 'BASE TABLE'
		)`,
		CountTables: `
		SELECT COUNT(*) FROM information_schema.tables
		  WHERE table_type = 'BASE TABLE'
		    AND table_schema NOT IN ('pg_catalog', 'information_schema')`,
		Rows: `SELECT * FROM %s LIMIT $1 OFFSET $2`,
	}
}

// --- MySQL / MariaDB --------------------------------------------------------

// MySQL = dialect MySQL/MariaDB lewat information_schema. Skema = nama
// database yang tersambung (TABLE_SCHEMA = DATABASE() bagi query global).
type MySQL struct{}

func (MySQL) Name() string { return "mysql" }

func (MySQL) OpenArgs(rawURL string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	pass, _ := u.User.Password()
	cfg := gomysql.NewConfig()
	cfg.User = u.User.Username()
	cfg.Passwd = pass
	cfg.Net = "tcp"
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "3306"
	}
	cfg.Addr = host + ":" + port
	cfg.DBName = strings.TrimPrefix(u.Path, "/")
	cfg.ParseTime = true
	// Satu eksekusi migration boleh berisi banyak statement (satu paket SQL),
	// sama seperti perilaku pgx di PostgreSQL.
	cfg.Params = map[string]string{"multiStatements": "true"}
	return "mysql", cfg.FormatDSN(), nil
}

func (MySQL) QuoteIdent(parts ...string) string {
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = "`" + strings.ReplaceAll(p, "`", "``") + "`"
	}
	return strings.Join(quoted, ".")
}

// GET_LOCK bersifat per-sesi; dilepas oleh UnlockSQL atau saat koneksi ditutup.
func (MySQL) LockSQL(connID int64) string {
	return fmt.Sprintf("SELECT GET_LOCK('pathgres-%d', 30)", connID)
}

func (MySQL) UnlockSQL(connID int64) string {
	return fmt.Sprintf("SELECT RELEASE_LOCK('pathgres-%d')", connID)
}

func (MySQL) Explain(analyze bool, stmt string) string {
	if analyze {
		// EXPLAIN ANALYZE MySQL menghasilkan pohon teks, bukan JSON.
		return "EXPLAIN ANALYZE " + stmt
	}
	return "EXPLAIN FORMAT=JSON " + stmt
}

func (MySQL) ExplainIsJSON(analyze bool) bool { return !analyze }

func (MySQL) Queries() Queries {
	return Queries{
		Summaries: `
		SELECT t.TABLE_SCHEMA, t.TABLE_NAME,
		       (SELECT COUNT(*) FROM information_schema.COLUMNS c
		         WHERE c.TABLE_SCHEMA = t.TABLE_SCHEMA AND c.TABLE_NAME = t.TABLE_NAME),
		       COALESCE(t.TABLE_ROWS, 0)
		FROM information_schema.TABLES t
		WHERE t.TABLE_TYPE = 'BASE TABLE'
		  AND t.TABLE_SCHEMA NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')
		ORDER BY t.TABLE_SCHEMA, t.TABLE_NAME`,
		PKs: `
		SELECT TABLE_SCHEMA, TABLE_NAME,
		       GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ', ')
		FROM information_schema.STATISTICS
		WHERE INDEX_NAME = 'PRIMARY'
		  AND TABLE_SCHEMA NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')
		GROUP BY TABLE_SCHEMA, TABLE_NAME`,
		Columns: `
		SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE = 'YES', COLUMN_DEFAULT
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION`,
		Indexes: `
		SELECT INDEX_NAME,
		       GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ', '),
		       MAX(NON_UNIQUE) = 0,
		       'BTREE'
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME <> 'PRIMARY'
		GROUP BY INDEX_NAME
		ORDER BY INDEX_NAME`,
		Constraints: `
		SELECT tc.CONSTRAINT_NAME, tc.CONSTRAINT_TYPE,
		       COALESCE(kcu.columns, ''),
		       COALESCE(fk.ref, '')
		FROM information_schema.TABLE_CONSTRAINTS tc
		LEFT JOIN (
			SELECT CONSTRAINT_NAME, TABLE_SCHEMA, TABLE_NAME,
			       GROUP_CONCAT(COLUMN_NAME ORDER BY ORDINAL_POSITION SEPARATOR ', ') AS columns
			FROM information_schema.KEY_COLUMN_USAGE
			GROUP BY CONSTRAINT_NAME, TABLE_SCHEMA, TABLE_NAME
		) kcu ON kcu.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
		   AND kcu.TABLE_SCHEMA = tc.TABLE_SCHEMA
		   AND kcu.TABLE_NAME = tc.TABLE_NAME
		LEFT JOIN (
			SELECT k.CONSTRAINT_NAME, k.TABLE_SCHEMA, k.TABLE_NAME,
			       CONCAT(k.REFERENCED_TABLE_SCHEMA, '.', k.REFERENCED_TABLE_NAME, '(',
			              GROUP_CONCAT(k.REFERENCED_COLUMN_NAME ORDER BY k.ORDINAL_POSITION SEPARATOR ', '),
			              ')') AS ref
			FROM information_schema.KEY_COLUMN_USAGE k
			WHERE k.REFERENCED_TABLE_NAME IS NOT NULL
			GROUP BY k.CONSTRAINT_NAME, k.TABLE_SCHEMA, k.TABLE_NAME,
			         k.REFERENCED_TABLE_SCHEMA, k.REFERENCED_TABLE_NAME
		) fk ON fk.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
		   AND fk.TABLE_SCHEMA = tc.TABLE_SCHEMA
		   AND fk.TABLE_NAME = tc.TABLE_NAME
		WHERE tc.TABLE_SCHEMA = ? AND tc.TABLE_NAME = ?
		ORDER BY tc.CONSTRAINT_TYPE, tc.CONSTRAINT_NAME`,
		ForeignKeys: `
		SELECT k.CONSTRAINT_NAME,
		       CONCAT(k.TABLE_SCHEMA, '.', k.TABLE_NAME, '(',
		              GROUP_CONCAT(k.COLUMN_NAME ORDER BY k.ORDINAL_POSITION SEPARATOR ', '), ')'),
		       CONCAT(k.REFERENCED_TABLE_SCHEMA, '.', k.REFERENCED_TABLE_NAME, '(',
		              GROUP_CONCAT(k.REFERENCED_COLUMN_NAME ORDER BY k.POSITION_IN_UNIQUE_CONSTRAINT SEPARATOR ', '), ')')
		FROM information_schema.KEY_COLUMN_USAGE k
		WHERE k.REFERENCED_TABLE_NAME IS NOT NULL
		  AND k.TABLE_SCHEMA NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')
		GROUP BY k.CONSTRAINT_NAME, k.TABLE_SCHEMA, k.TABLE_NAME,
		         k.REFERENCED_TABLE_SCHEMA, k.REFERENCED_TABLE_NAME
		ORDER BY k.CONSTRAINT_NAME`,
		Validate: `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.TABLES
			WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND TABLE_TYPE = 'BASE TABLE'
		)`,
		CountTables: `
		SELECT COUNT(*) FROM information_schema.TABLES
		  WHERE TABLE_TYPE = 'BASE TABLE'
		    AND TABLE_SCHEMA NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')`,
		Rows: `SELECT * FROM %s LIMIT ? OFFSET ?`,
	}
}
