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
func iconSVG(params any) (template.HTML, error) {
	var name, class, alt string
	switch v := params.(type) {
	case string:
		name = v
	case map[string]any:
		if n, ok := v["Name"].(string); ok {
			name = n
		}
		if c, ok := v["Class"].(string); ok {
			class = c
		}
		if a, ok := v["Alt"].(string); ok {
			alt = a
		}
	default:
		return "", fmt.Errorf("icon: argumen tidak valid %T", params)
	}
	if !iconNameRe.MatchString(name) {
		return "", fmt.Errorf("icon: nama tidak valid %q", name)
	}
	key := name
	if class != "" {
		key = name + "|" + class
	}
	if v, ok := iconCache.Load(key); ok {
		return v.(template.HTML), nil
	}
	b, err := StaticFS.ReadFile("static/icons/" + name + ".svg")
	if err != nil {
		return "", fmt.Errorf("icon %q tidak ditemukan di static/icons", name)
	}
	svgClass := "icon"
	if class != "" {
		svgClass += " " + class
	}
	s := strings.Replace(strings.TrimSpace(string(b)), "<svg",
		fmt.Sprintf(`<svg class="%s" aria-hidden="true" focusable="false"`, svgClass), 1)
	if alt != "" {
		s = strings.Replace(s, ">", fmt.Sprintf("><title>%s</title>", template.HTMLEscapeString(alt)), 1)
	}
	h := template.HTML(s)
	iconCache.Store(key, h)
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
	// default mengembalikan val, atau def bila val kosong (string "" atau nil).
	"default": func(def, val any) any {
		if val == nil {
			return def
		}
		if s, ok := val.(string); ok && s == "" {
			return def
		}
		return val
	},
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
	pages    map[string]*template.Template
	appPages map[string]*template.Template
}

// NewRenderer mem-parse tiap halaman bersama shell-nya. Halaman bagian
// database management memakai templates/app.html, halaman lain memakai
// templates/layout.html. Nama halaman = path relatif dari templates/,
// misalnya "login.html", "admin/users.html", atau "dbm/databases.html".
func NewRenderer() (*Renderer, error) {
	r := &Renderer{
		pages:    map[string]*template.Template{},
		appPages: map[string]*template.Template{},
	}
	err := fs.WalkDir(TemplatesFS, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".html") || p == "templates/layout.html" || isShared(p) {
			return nil
		}
		files := append([]string{"templates/layout.html"}, sharedGlobs...)
		files = append(files, p)
		name := strings.TrimPrefix(p, "templates/")
		isApp := usesAppShell(p)
		root := "layout.html"
		if isApp {
			root = "app.html"
			files[0] = "templates/app.html"
		}
		t, err := template.New(root).Funcs(funcs).ParseFS(TemplatesFS, files...)
		if err != nil {
			return err
		}
		if isApp {
			r.appPages[name] = t
		} else {
			r.pages[name] = t
		}
		return nil
	})
	return r, err
}

func usesAppShell(path string) bool {
	if strings.HasPrefix(path, "templates/dbm/") {
		return true
	}
	return strings.HasPrefix(strings.TrimPrefix(path, "templates/"), "admin/")
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

func (r *Renderer) RenderApp(w http.ResponseWriter, name string, data any) {
	t, ok := r.appPages[name]
	if !ok {
		http.Error(w, "template tidak ditemukan", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "app.html", data); err != nil {
		log.Printf("render %s gagal: %v", name, err)
		http.Error(w, "gagal render halaman", http.StatusInternalServerError)
	}
}
