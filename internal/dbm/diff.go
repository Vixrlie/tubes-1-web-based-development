package dbm

// Schema diff & generator kandidat SQL migration (BON-02/03).
// Sisi A = kondisi saat ini (koneksi yang dibuka), sisi B = referensi
// (koneksi pembanding). Kandidat SQL selalu ditinjau/diubah pengguna
// lewat halaman editor sebelum dijalankan.

import (
	"context"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DiffItem = satu perbedaan; SQL = kandidat perubahan pada kondisi saat ini.
type DiffItem struct {
	Schema string // schema tabel terkait
	Table  string // nama tabel (tanpa schema)
	Kind   string // "table" | "column" | "pk" | "index" | "fk"
	Status string // "tambah" | "ubah" | "hapus" (arah perubahan pada kondisi saat ini)
	Detail string // deskripsi manusia
	SQL    string // kandidat SQL ("" bila tidak dihasilkan)
}

// Qualified = nama tabel "schema.tabel" untuk tampilan & SQL.
func (d DiffItem) Qualified() string {
	return d.Schema + "." + d.Table
}

type tableState struct {
	Schema  string
	Name    string
	PK      string
	Columns map[string]ColumnInfo
	Indexes map[string]IndexInfo
}

type schemaState struct {
	Tables map[string]*tableState // key: "schema.tabel"
	// FKs key: "schema.tabel|nama_constraint" (dari sisi pemilik).
	FKs map[string]ForeignKey
}

func tkey(schema, table string) string { return schema + "." + table }

// loadSchemaState memuat kondisi schema satu database target: tabel + kolom
// per tabel, index per tabel, dan seluruh foreign key.
func loadSchemaState(ctx context.Context, pool *pgxpool.Pool) (*schemaState, error) {
	st := &schemaState{
		Tables: map[string]*tableState{},
		FKs:    map[string]ForeignKey{},
	}
	sums, err := ListTableSummaries(ctx, pool)
	if err != nil {
		return nil, err
	}
	for _, s := range sums {
		ts := &tableState{
			Schema:  s.Schema,
			Name:    s.Name,
			PK:      s.PK,
			Columns: map[string]ColumnInfo{},
			Indexes: map[string]IndexInfo{},
		}
		cols, err := ListColumns(ctx, pool, s.Schema, s.Name)
		if err != nil {
			return nil, err
		}
		for _, c := range cols {
			ts.Columns[c.Name] = c
		}
		idxs, err := ListIndexes(ctx, pool, s.Schema, s.Name)
		if err != nil {
			return nil, err
		}
		for _, ix := range idxs {
			ts.Indexes[ix.Name] = ix
		}
		st.Tables[tkey(s.Schema, s.Name)] = ts
	}
	fks, err := ListForeignKeys(ctx, pool)
	if err != nil {
		return nil, err
	}
	for _, f := range fks {
		owner, _ := splitFKRef(f.From)
		st.FKs[owner+"|"+f.Name] = f
	}
	return st, nil
}

// DiffSchemas membandingkan kondisi saat ini (cur) dengan referensi (ref).
func DiffSchemas(cur, ref *schemaState) []DiffItem {
	var items []DiffItem

	keys := map[string]bool{}
	for k := range cur.Tables {
		keys[k] = true
	}
	for k := range ref.Tables {
		keys[k] = true
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)

	for _, k := range ordered {
		a, inA := cur.Tables[k]
		b, inB := ref.Tables[k]

		switch {
		case !inA && inB: // tabel ada di referensi, belum ada di sini
			items = append(items, DiffItem{
				Schema: b.Schema, Table: b.Name, Kind: "table", Status: "tambah",
				Detail: "tabel tidak ada di kondisi saat ini",
				SQL:    generateCreate(b),
			})
		case inA && !inB: // tabel berlebih di sini
			items = append(items, DiffItem{
				Schema: a.Schema, Table: a.Name, Kind: "table", Status: "hapus",
				Detail: "tabel tidak ada di referensi",
				SQL:    "DROP TABLE " + quoteIdent(a.Schema, a.Name) + ";",
			})
		default:
			items = append(items, diffTable(a, b)...)
		}
	}

	items = append(items, diffFKs(cur, ref)...)
	return items
}

// diffTable membandingkan isi satu tabel yang ada di kedua sisi.
func diffTable(a, b *tableState) []DiffItem {
	var items []DiffItem
	schema, name := a.Schema, a.Name
	q := quoteIdent(schema, name)

	cols := map[string]bool{}
	for c := range a.Columns {
		cols[c] = true
	}
	for c := range b.Columns {
		cols[c] = true
	}
	ordered := make([]string, 0, len(cols))
	for c := range cols {
		ordered = append(ordered, c)
	}
	sort.Strings(ordered)

	for _, cname := range ordered {
		ca, inA := a.Columns[cname]
		cb, inB := b.Columns[cname]
		switch {
		case !inA && inB:
			items = append(items, DiffItem{
				Schema: schema, Table: name, Kind: "column", Status: "tambah",
				Detail: "kolom " + cname + " tidak ada di kondisi saat ini",
				SQL:    "ALTER TABLE " + q + " ADD COLUMN " + columnDef(cb) + ";",
			})
		case inA && !inB:
			items = append(items, DiffItem{
				Schema: schema, Table: name, Kind: "column", Status: "hapus",
				Detail: "kolom " + cname + " tidak ada di referensi",
				SQL:    "ALTER TABLE " + q + " DROP COLUMN " + quoteIdent(cname) + ";",
			})
		default:
			qc := quoteIdent(cname)
			if !strings.EqualFold(ca.Type, cb.Type) {
				items = append(items, DiffItem{
					Schema: schema, Table: name, Kind: "column", Status: "ubah",
					Detail: "kolom " + cname + ": tipe berbeda (" + ca.Type + " → " + cb.Type + "); mungkin perlu USING bila konversi tidak implisit",
					SQL:    "ALTER TABLE " + q + " ALTER COLUMN " + qc + " TYPE " + cb.Type + ";",
				})
			}
			if ca.Nullable != cb.Nullable {
				verb := "SET NOT NULL"
				if cb.Nullable {
					verb = "DROP NOT NULL"
				}
				items = append(items, DiffItem{
					Schema: schema, Table: name, Kind: "column", Status: "ubah",
					Detail: "kolom " + cname + ": nullable berbeda",
					SQL:    "ALTER TABLE " + q + " ALTER COLUMN " + qc + " " + verb + ";",
				})
			}
			if defaultOf(ca) != defaultOf(cb) {
				verb := "DROP DEFAULT"
				if cb.Default != nil && *cb.Default != "" {
					verb = "SET DEFAULT " + *cb.Default
				}
				items = append(items, DiffItem{
					Schema: schema, Table: name, Kind: "column", Status: "ubah",
					Detail: "kolom " + cname + ": default berbeda (" + defaultOf(ca) + " → " + defaultOf(cb) + ")",
					SQL:    "ALTER TABLE " + q + " ALTER COLUMN " + qc + " " + verb + ";",
				})
			}
		}
	}

	// Primary key
	if a.PK != b.PK {
		detail := "primary key berbeda (" + labelPK(a.PK) + " → " + labelPK(b.PK) + ")"
		sql := ""
		if b.PK != "" {
			sql = "ALTER TABLE " + q + " DROP CONSTRAINT IF EXISTS " + quoteIdent(name+"_pkey") + ";\n" +
				"ALTER TABLE " + q + " ADD PRIMARY KEY (" + quoteList(b.PK) + ");"
		} else if a.PK != "" {
			sql = "ALTER TABLE " + q + " DROP CONSTRAINT IF EXISTS " + quoteIdent(name+"_pkey") + ";"
		}
		items = append(items, DiffItem{
			Schema: schema, Table: name, Kind: "pk", Status: "ubah",
			Detail: detail, SQL: sql,
		})
	}

	// Index (dibandingkan per nama dalam tabel)
	idxs := map[string]bool{}
	for i := range a.Indexes {
		idxs[i] = true
	}
	for i := range b.Indexes {
		idxs[i] = true
	}
	orderedIdx := make([]string, 0, len(idxs))
	for i := range idxs {
		orderedIdx = append(orderedIdx, i)
	}
	sort.Strings(orderedIdx)
	for _, iname := range orderedIdx {
		ia, inA := a.Indexes[iname]
		ib, inB := b.Indexes[iname]
		create := func(x IndexInfo) string {
			s := "CREATE "
			if x.Unique {
				s += "UNIQUE "
			}
			return s + "INDEX " + quoteIdent(schema, x.Name) + " ON " + q + " (" + quoteList(x.Columns) + ");"
		}
		switch {
		case !inA && inB:
			items = append(items, DiffItem{
				Schema: schema, Table: name, Kind: "index", Status: "tambah",
				Detail: "index " + iname + " tidak ada di kondisi saat ini",
				SQL:    create(ib),
			})
		case inA && !inB:
			items = append(items, DiffItem{
				Schema: schema, Table: name, Kind: "index", Status: "hapus",
				Detail: "index " + iname + " tidak ada di referensi",
				SQL:    "DROP INDEX " + quoteIdent(schema, ia.Name) + ";",
			})
		default:
			if ia.Columns != ib.Columns || ia.Unique != ib.Unique {
				items = append(items, DiffItem{
					Schema: schema, Table: name, Kind: "index", Status: "ubah",
					Detail: "index " + iname + ": definisi berbeda (" + ia.Columns + "/" + boolLabel(ia.Unique) + " → " + ib.Columns + "/" + boolLabel(ib.Unique) + ")",
					SQL:    "DROP INDEX " + quoteIdent(schema, ia.Name) + ";\n" + create(ib),
				})
			}
		}
	}
	return items
}

// diffFKs membandingkan foreign key per pemilik tabel|nama constraint.
func diffFKs(cur, ref *schemaState) []DiffItem {
	var items []DiffItem
	keys := map[string]bool{}
	for k := range cur.FKs {
		keys[k] = true
	}
	for k := range ref.FKs {
		keys[k] = true
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)

	addSQL := func(f ForeignKey) string {
		ownerRef, cols := splitFKRef(f.From)
		oschema, otable := split2(ownerRef)
		targetRef, targetCols := splitFKRef(f.ToRef)
		tschema, ttable := split2(targetRef)
		return "ALTER TABLE " + quoteIdent(oschema, otable) + " ADD CONSTRAINT " + quoteIdent(f.Name) +
			" FOREIGN KEY (" + quoteList(cols) + ") REFERENCES " + quoteIdent(tschema, ttable) +
			" (" + quoteList(targetCols) + ");"
	}
	dropSQL := func(owner, name string) string {
		schema, table := split2(owner)
		return "ALTER TABLE " + quoteIdent(schema, table) + " DROP CONSTRAINT IF EXISTS " + quoteIdent(name) + ";"
	}
	for _, k := range ordered {
		owner := k[:strings.IndexByte(k, '|')]
		a, inA := cur.FKs[k]
		b, inB := ref.FKs[k]
		schema, table := split2(owner)
		switch {
		case !inA && inB:
			items = append(items, DiffItem{
				Schema: schema, Table: table, Kind: "fk", Status: "tambah",
				Detail: "foreign key " + b.Name + " tidak ada di kondisi saat ini",
				SQL:    addSQL(b),
			})
		case inA && !inB:
			items = append(items, DiffItem{
				Schema: schema, Table: table, Kind: "fk", Status: "hapus",
				Detail: "foreign key " + a.Name + " tidak ada di referensi",
				SQL:    dropSQL(owner, a.Name),
			})
		default:
			if a.ToRef != b.ToRef {
				items = append(items, DiffItem{
					Schema: schema, Table: table, Kind: "fk", Status: "ubah",
					Detail: "foreign key " + a.Name + ": tujuan berbeda (" + a.ToRef + " → " + b.ToRef + ")",
					SQL:    dropSQL(owner, a.Name) + "\n" + addSQL(b),
				})
			}
		}
	}
	return items
}

// GenerateSQL menyusun kandidat SQL migration dari daftar selisih, diurutkan
// agar aman dijalankan berurutan: buat tabel → ubah kolom/index → hapus tabel
// → foreign key.
func GenerateSQL(items []DiffItem) string {
	weight := func(d DiffItem) int {
		switch {
		case d.Kind == "table" && d.Status == "tambah":
			return 0
		case d.Kind == "table" && d.Status == "hapus":
			return 2
		case d.Kind == "fk":
			return 3
		default:
			return 1
		}
	}
	sorted := make([]DiffItem, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(i, j int) bool {
		return weight(sorted[i]) < weight(sorted[j])
	})
	var lines []string
	for _, d := range sorted {
		if d.SQL != "" {
			lines = append(lines, d.SQL)
		}
	}
	return strings.Join(lines, "\n")
}

// generateCreate menyusun CREATE TABLE dari kondisi referensi.
func generateCreate(t *tableState) string {
	var b strings.Builder
	b.WriteString("CREATE TABLE " + quoteIdent(t.Schema, t.Name) + " (\n")
	names := make([]string, 0, len(t.Columns))
	for n := range t.Columns {
		names = append(names, n)
	}
	sort.Strings(names)
	for i, n := range names {
		b.WriteString("    " + columnDef(t.Columns[n]))
		if i < len(names)-1 || t.PK != "" {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	if t.PK != "" {
		b.WriteString("    PRIMARY KEY (" + quoteList(t.PK) + ")\n")
	}
	b.WriteString(");")
	return b.String()
}

func columnDef(c ColumnInfo) string {
	def := quoteIdent(c.Name) + " " + c.Type
	if !c.Nullable {
		def += " NOT NULL"
	}
	if c.Default != nil && *c.Default != "" {
		def += " DEFAULT " + *c.Default
	}
	return def
}

// quoteIdent mengutip identifier ("schema"."tabel" / "kolom").
func quoteIdent(parts ...string) string {
	return pgx.Identifier(parts).Sanitize()
}

// quoteList mengutip daftar "a, b" per kolom.
func quoteList(csv string) string {
	parts := strings.Split(csv, ",")
	for i := range parts {
		parts[i] = quoteIdent(strings.TrimSpace(parts[i]))
	}
	return strings.Join(parts, ", ")
}

// splitFKRef memecah "public.orders(customer_id)" atau "public.orders (a, b)"
// menjadi (tabel, kolom).
func splitFKRef(s string) (table, cols string) {
	table, cols, found := strings.Cut(s, "(")
	if !found {
		return s, ""
	}
	return strings.TrimSpace(table), strings.TrimSuffix(strings.TrimSpace(cols), ")")
}

// split2 memecah "schema.tabel" jadi dua bagian.
func split2(s string) (schema, table string) {
	schema, table, found := strings.Cut(s, ".")
	if !found {
		return "public", s
	}
	return schema, table
}

func defaultOf(c ColumnInfo) string {
	if c.Default == nil {
		return "-"
	}
	return *c.Default
}

func labelPK(pk string) string {
	if pk == "" {
		return "tanpa PK"
	}
	return pk
}

func boolLabel(b bool) string {
	if b {
		return "unik"
	}
	return "biasa"
}
