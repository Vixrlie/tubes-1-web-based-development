package web

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
)

//go:embed all:templates
var TemplatesFS embed.FS

//go:embed static/*
var StaticFS embed.FS

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
		if d.IsDir() || !strings.HasSuffix(p, ".html") || p == "templates/layout.html" {
			return nil
		}
		t, err := template.ParseFS(TemplatesFS, "templates/layout.html", p)
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
