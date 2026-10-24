package web

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

//go:embed all:templates
var TemplatesFS embed.FS

//go:embed static/*
var StaticFS embed.FS

// iconNameRe membatasi nama icon (huruf kecil, angka, strip) supaya
// tidak bisa dipakai untuk path traversal.
var iconNameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// iconCache menyimpan hasil baca static/icons/<nama>.svg supaya
// file tidak dibaca ulang tiap render.
var iconCache sync.Map

// iconSVG membaca static/icons/<nama>.svg dan menyisipkan class "icon"
// ke tag <svg>, jadi ukuran diatur CSS (.btn .icon, .side-link .icon)
// dan warna ikut currentColor. Nama hanya dari literal template.
func iconSVG(name string) (template.HTML, error) {
	if !iconNameRe.MatchString(name) {
		return "", fmt.Errorf("icon: nama tidak valid %q", name)
	}
	if v, ok := iconCache.Load(name); ok {
		return v.(template.HTML), nil
	}
	b, err := StaticFS.ReadFile("static/icons/" + name + ".svg")
	if err != nil {
		return "", fmt.Errorf("icon %q tidak ditemukan di static/icons", name)
	}
	s := strings.Replace(strings.TrimSpace(string(b)), "<svg",
		`<svg class="icon" aria-hidden="true" focusable="false"`, 1)
	h := template.HTML(s)
	iconCache.Store(name, h)
	return h, nil
}

// iconNames mengembalikan nama semua icon di static/icons (tanpa ".svg"),
// dipakai halaman dev/components untuk menampilkan daftar icon otomatis.
func iconNames() []string {
	entries, err := fs.ReadDir(StaticFS, "static/icons")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".svg") {
			names = append(names, strings.TrimSuffix(e.Name(), ".svg"))
		}
	}
	return names
}

// funcs dipakai semua template. `dict` untuk memanggil komponen dengan
// parameter bernama: {{template "button" (dict "Label" "Simpan")}}
var funcs = template.FuncMap{
	"dict": func(kv ...any) (map[string]any, error) {
		if len(kv)%2 != 0 {
			return nil, errors.New("dict: jumlah argumen harus genap (key, value, ...)")
		}
		m := make(map[string]any, len(kv)/2)
		for i := 0; i < len(kv); i += 2 {
			k, ok := kv[i].(string)
			if !ok {
				return nil, errors.New("dict: key harus string")
			}
			m[k] = kv[i+1]
		}
		return m, nil
	},
	// list membuat slice untuk {{range}}: {{range (list "a" "b")}}
	"list": func(v ...any) []any { return v },
	// css menandai nilai CSS tepercaya supaya var(--x) tidak diblokir
	// sanitizer html/template (jadi "ZgotmplZ"). Hanya untuk literal di template.
	"css": func(s string) template.CSS { return template.CSS(s) },
	// attr menandai string sebagai atribut HTML tepercaya (hanya untuk
	// literal di template, JANGAN dari input user).
	"attr": func(s string) template.HTMLAttr { return template.HTMLAttr(s) },
	// icon menyisipkan SVG inline dari static/icons/<nama>.svg.
	"icon": iconSVG,
	// iconNames daftar nama icon yang tersedia (untuk halaman dev).
	"iconNames": iconNames,
}

// Folder yang berisi potongan template (bukan halaman). Di-parse bersama
// setiap halaman, jadi {{template "button" ...}} bisa dipakai di mana saja.
var sharedGlobs = []string{
	"templates/components/*.html",
	"templates/partials/*.html",
}

func isShared(p string) bool {
	return strings.HasPrefix(p, "templates/components/") || strings.HasPrefix(p, "templates/partials/")
}

type Renderer struct {
	pages map[string]*template.Template
}

// NewRenderer mem-parse tiap halaman bersama templates/layout.html.
// Nama halaman = path relatif dari templates/, misalnya "login.html"
// atau "admin/users.html".
func NewRenderer() (*Renderer, error) {
	r := &Renderer{pages: map[string]*template.Template{}}
	err := fs.WalkDir(TemplatesFS, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".html") || p == "templates/layout.html" || isShared(p) {
			return nil
		}
		files := append([]string{"templates/layout.html"}, sharedGlobs...)
		files = append(files, p)
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(TemplatesFS, files...)
		if err != nil {
			return err
		}
		r.pages[strings.TrimPrefix(p, "templates/")] = t
		return nil
	})
	return r, err
}

func (r *Renderer) Render(w http.ResponseWriter, name string, data any) {
	t, ok := r.pages[name]
	if !ok {
		http.Error(w, "template tidak ditemukan", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render %s gagal: %v", name, err)
		http.Error(w, "gagal render halaman", http.StatusInternalServerError)
	}
}
