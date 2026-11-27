package dbm

import "testing"

func TestSplitStatements(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"sederhana", "SELECT 1; SELECT 2;", []string{"SELECT 1", "SELECT 2"}},
		{"titik-koma dalam string", `SELECT 'a;b'; SELECT 2`, []string{"SELECT 'a;b'", "SELECT 2"}},
		{"escape string", `SELECT 'it''s; ok'; SELECT 2`, []string{"SELECT 'it''s; ok'", "SELECT 2"}},
		{"komentar baris", "-- pakai ;\nSELECT 1;", []string{"-- pakai ;\nSELECT 1"}},
		{"komentar blok", "/* ; ; */SELECT 1", []string{"/* ; ; */SELECT 1"}},
		{"dollar quoting", "CREATE FUNCTION f() RETURNS int AS $$ BEGIN; RETURN 1; END; $$ LANGUAGE plpgsql; SELECT 1",
			[]string{"CREATE FUNCTION f() RETURNS int AS $$ BEGIN; RETURN 1; END; $$ LANGUAGE plpgsql", "SELECT 1"}},
		{"tag dollar", "DO $x$ BEGIN; END; $x$; SELECT 2", []string{"DO $x$ BEGIN; END; $x$", "SELECT 2"}},
		{"parameter $1 bukan tag", "SELECT $1; SELECT 2", []string{"SELECT $1", "SELECT 2"}},
		{"statement kosong dibuang", "SELECT 1;; ;SELECT 2", []string{"SELECT 1", "SELECT 2"}},
		{"hanya komentar dibuang", "-- saja\n/* juga */", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SplitStatements(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("got %d %q, want %d %q", len(got), got, len(c.want), c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("stmt[%d] = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestStmtKind(t *testing.T) {
	cases := map[string]string{
		"SELECT * FROM t":                       "query",
		"  with x as (select 1) select 2":       "query",
		"INSERT INTO t VALUES (1)":              "query",
		"UPDATE t SET a = 1":                    "query",
		"CREATE TABLE t (id int)":               "ddl",
		"-- komentar\nALTER TABLE t ADD c text": "ddl",
		"GRANT SELECT ON t TO u":                "ddl",
		"EXPLAIN SOMETHING":                     "lainnya",
	}
	for in, want := range cases {
		if got := stmtKind(in); got != want {
			t.Errorf("stmtKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortStmt(t *testing.T) {
	if s := shortStmt("SELECT   *\n  FROM t"); s != "SELECT * FROM t" {
		t.Errorf("shortStmt = %q", s)
	}
	long := shortStmt(makeLong(200))
	if len([]rune(long)) > 73 {
		t.Errorf("terlalu panjang: %d", len([]rune(long)))
	}
}

func makeLong(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
