package dbm

import (
	"strings"
	"testing"
)

func col(name, typ string, nullable bool, def *string) ColumnInfo {
	return ColumnInfo{Name: name, Type: typ, Nullable: nullable, Default: def}
}

func table(schema, name, pk string, cols ...ColumnInfo) *tableState {
	t := &tableState{Schema: schema, Name: name, PK: pk, Columns: map[string]ColumnInfo{}, Indexes: map[string]IndexInfo{}}
	for _, c := range cols {
		t.Columns[c.Name] = c
	}
	return t
}

func state(tables ...*tableState) *schemaState {
	s := &schemaState{Tables: map[string]*tableState{}, FKs: map[string]ForeignKey{}}
	for _, t := range tables {
		s.Tables[tkey(t.Schema, t.Name)] = t
	}
	return s
}

func hasItem(t *testing.T, items []DiffItem, kind, status, detailSub, sqlSub string) {
	t.Helper()
	for _, it := range items {
		if it.Kind == kind && it.Status == status &&
			strings.Contains(it.Detail, detailSub) && strings.Contains(it.SQL, sqlSub) {
			return
		}
	}
	t.Errorf("tidak ada item %s/%s (%q..%q) di %d item; isi: %+v", kind, status, detailSub, sqlSub, len(items), items)
}

func TestDiffSchemasTabelDanKolom(t *testing.T) {
	def := "now()"
	cur := state(
		table("public", "lama", "id", col("id", "integer", false, nil), col("nama", "text", true, nil)),
		table("public", "bersama", "id",
			col("id", "integer", false, nil),
			col("nilai", "integer", true, nil)),
	)
	ref := state(
		table("public", "baru", "id", col("id", "integer", false, nil)),
		table("public", "bersama", "id",
			col("id", "bigint", false, nil),
			col("nilai", "integer", false, &def),
			col("ket", "text", true, nil)),
	)
	items := DiffSchemas(cur, ref, Postgres{})

	hasItem(t, items, "table", "tambah", "tabel tidak ada di kondisi saat ini", `CREATE TABLE "public"."baru"`)
	hasItem(t, items, "table", "hapus", "tabel tidak ada di referensi", `DROP TABLE "public"."lama"`)
	hasItem(t, items, "column", "tambah", "kolom ket", `ADD COLUMN "ket" text`)
	hasItem(t, items, "column", "ubah", "tipe berbeda (integer → bigint)", `ALTER COLUMN "id"`)
	hasItem(t, items, "column", "ubah", "nullable berbeda", "SET NOT NULL")
	hasItem(t, items, "column", "ubah", "default berbeda", "SET DEFAULT now()")
}

func TestDiffSchemasIndexFKDanPK(t *testing.T) {
	cur := state(table("public", "orders", "id", col("id", "integer", false, nil)))
	cur.Tables["public.orders"].Indexes["orders_pkey"] = IndexInfo{Name: "orders_pkey", Columns: "id", Unique: true}
	cur.FKs["public.orders|orders_cust_fk"] = ForeignKey{
		Name: "orders_cust_fk", From: "public.orders (cust_id)", ToRef: "public.customers (id)"}

	ref := state(table("public", "orders", "id,cust_id",
		col("id", "integer", false, nil), col("cust_id", "integer", false, nil)))
	ref.Tables["public.orders"].Indexes["orders_pkey"] = IndexInfo{Name: "orders_pkey", Columns: "id", Unique: true}
	ref.Tables["public.orders"].Indexes["idx_cust"] = IndexInfo{Name: "idx_cust", Columns: "cust_id", Unique: false}
	ref.FKs["public.orders|orders_cust_fk"] = ForeignKey{
		Name: "orders_cust_fk", From: "public.orders (cust_id)", ToRef: "public.customers (id)"}

	items := DiffSchemas(cur, ref, Postgres{})
	hasItem(t, items, "column", "tambah", "kolom cust_id", `ADD COLUMN "cust_id" integer NOT NULL`)
	hasItem(t, items, "pk", "ubah", "primary key berbeda", "ADD PRIMARY KEY")
	hasItem(t, items, "index", "tambah", "index idx_cust", `CREATE INDEX "public"."idx_cust" ON "public"."orders" ("cust_id")`)
	// FK di kedua sisi identik → tidak ada selisih fk
	for _, it := range items {
		if it.Kind == "fk" {
			t.Errorf("FK seharusnya identik, malah muncul: %+v", it)
		}
	}
}

func TestDiffFKHilangDiReferensi(t *testing.T) {
	cur := state(table("public", "orders", "id", col("id", "integer", false, nil)))
	cur.FKs["public.orders|orders_cust_fk"] = ForeignKey{
		Name: "orders_cust_fk", From: "public.orders (cust_id)", ToRef: "public.customers (id)"}
	ref := state(table("public", "orders", "id", col("id", "integer", false, nil)))

	items := DiffSchemas(cur, ref, Postgres{})
	hasItem(t, items, "fk", "hapus", "orders_cust_fk",
		`ALTER TABLE "public"."orders" DROP CONSTRAINT IF EXISTS "orders_cust_fk"`)
}

func TestGenerateSQLUrutan(t *testing.T) {
	items := []DiffItem{
		{Kind: "fk", Status: "tambah", SQL: "ALTER TABLE fk;"},
		{Kind: "table", Status: "hapus", SQL: "DROP TABLE lama;"},
		{Kind: "column", Status: "ubah", SQL: "ALTER TABLE col;"},
		{Kind: "table", Status: "tambah", SQL: "CREATE TABLE baru;"},
	}
	sql := GenerateSQL(items)
	if strings.Index(sql, "CREATE TABLE") > strings.Index(sql, "ALTER TABLE col") ||
		strings.Index(sql, "ALTER TABLE col") > strings.Index(sql, "DROP TABLE") ||
		strings.Index(sql, "DROP TABLE") > strings.Index(sql, "ALTER TABLE fk;") {
		t.Errorf("urutan salah:\n%s", sql)
	}
}

func TestSplitFKRef(t *testing.T) {
	tableName, cols := splitFKRef("public.orders (a, b)")
	if tableName != "public.orders" || cols != "a, b" {
		t.Errorf("got %q %q", tableName, cols)
	}
	if tn, c := splitFKRef("orphan"); tn != "orphan" || c != "" {
		t.Errorf("got %q %q", tn, c)
	}
}
