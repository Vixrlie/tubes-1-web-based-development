package web

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func newDev(nav, active string, admin bool) devData {
	return devData{Shell: devShell(nav, active, admin), Avatars: devAvatars()}
}

func renderDev(t *testing.T, r *Renderer, d devData) string {
	t.Helper()
	tpl, ok := r.pages["dev/components.html"]
	if !ok {
		t.Fatal("dev/components.html tidak ter-parse")
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "layout.html", d); err != nil {
		t.Fatalf("render gagal: %v", err)
	}
	return buf.String()
}

func TestTemplatesParse(t *testing.T) {
	if _, err := NewRenderer(); err != nil {
		t.Fatalf("parse template gagal: %v", err)
	}
}

func TestDevPage(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		d       devData
		want    []string
		notWant []string
	}{
		{"tanpa sidebar", newDev("none", "", true),
			[]string{
				`class="btn btn--primary"`, `type="button"`,
				`btn btn--secondary is-active`, `is-disabled`,
				`btn--custom`, `--btn-fill:var(--color-danger);`,
				`aria-disabled="true"`, `app-shell--solo`,
			},
			[]string{`app-header`, `class="sidebar"`}},
		{"admin", newDev("admin", "connections", true),
			[]string{
				`app-header`, `aria-label="Navigasi admin"`, `Koneksi Database`,
				`re:href="/admin/connections"\s+aria-current="page"`, // spasi antar atribut bebas
			}, nil},
		{"database", newDev("database", "table:public.orders", false),
			[]string{
				`aria-label="Navigasi database"`, `nama_database`, `orders`, `Migrasi`,
			},
			[]string{`/admin/users`}}, // bukan platform admin -> link Admin tidak ada
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := renderDev(t, r, c.d)
			for _, s := range append([]string{}, c.want...) {
				if rx, ok := strings.CutPrefix(s, "re:"); ok {
					if !regexp.MustCompile(rx).MatchString(out) {
						t.Errorf("harusnya cocok dengan: %s", rx)
					}
				} else if !strings.Contains(out, s) {
					t.Errorf("harusnya ada: %s", s)
				}
			}
			bad := append(c.notWant, `ZgotmplZ`, `<no value>`, `&lt;no value&gt;`, `type=" button"`)
			for _, s := range bad {
				if strings.Contains(out, s) {
					t.Errorf("tidak boleh ada: %s", s)
				}
			}
		})
	}
}

func TestDatabasesPage(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	tpl, ok := r.pages["databases.html"]
	if !ok {
		t.Fatal("databases.html tidak ter-parse")
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "layout.html", devDatabases()); err != nil {
		t.Fatalf("render gagal: %v", err)
	}
	out := buf.String()
	for _, s := range []string{"Daftar Database", "inventory_db", "hr_db", "Gagal terhubung", "Coba lagi", "app-shell--solo"} {
		if !strings.Contains(out, s) {
			t.Errorf("harusnya ada: %s", s)
		}
	}
	for _, s := range []string{"ZgotmplZ", "<no value>", "&lt;no value&gt;", `class="sidebar"`, "tabs__item"} {
		if strings.Contains(out, s) {
			t.Errorf("tidak boleh ada: %s", s)
		}
	}
	// header menyorot menu Database
	if !regexp.MustCompile(`href="/"\s+aria-current="page"[^>]*>Database`).MatchString(out) {
		t.Error("menu Database di header harusnya aktif")
	}
}

func TestSchemaPage(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	tpl, ok := r.pages["schema.html"]
	if !ok {
		t.Fatal("schema.html tidak ter-parse")
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "layout.html", devSchema()); err != nil {
		t.Fatalf("render gagal: %v", err)
	}
	out := buf.String()
	for _, s := range []string{
		"Schema public", "breadcrumb", "shop_db", "Daftar Tabel", "Relasi (Foreign Key)",
		"orders_user_id_fkey", "users.id", `label label--info`, `aria-label="Navigasi database"`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("harusnya ada: %s", s)
		}
	}
	for _, s := range []string{"ZgotmplZ", "<no value>", "&lt;no value&gt;"} {
		if strings.Contains(out, s) {
			t.Errorf("tidak boleh ada: %s", s)
		}
	}
}