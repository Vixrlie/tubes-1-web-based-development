package web

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
)

var funcMap = template.FuncMap{
	"dict": func(values ...any) (map[string]any, error) {
		if len(values)%2 != 0 {
			return nil, nil
		}
		m := make(map[string]any, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				continue
			}
			m[key] = values[i+1]
		}
		return m, nil
	},
	"default": func(def, val any) any {
		if val == nil {
			return def
		}
		if s, ok := val.(string); ok && s == "" {
			return def
		}
		return val
	},
}

//go:embed all:templates
var TemplatesFS embed.FS

//go:embed static/*
var StaticFS embed.FS

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
		if d.IsDir() || !strings.HasSuffix(p, ".html") {
			return nil
		}
		if p == "templates/layout.html" || p == "templates/app.html" {
			return nil
		}
		if strings.HasPrefix(p, "templates/partials/") {
			return nil
		}
		shell := "templates/layout.html"
		target := r.pages
		if usesAppShell(p) {
			shell = "templates/app.html"
			target = r.appPages
		}
		t, err := template.New("shell").Funcs(funcMap).ParseFS(TemplatesFS, "templates/partials/icon.html", shell, p)
		if err != nil {
			return err
		}
		target[strings.TrimPrefix(p, "templates/")] = t
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
