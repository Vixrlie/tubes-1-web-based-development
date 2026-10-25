package web

import (
	"log"
	"net/http"
)

func (r *Renderer) RenderConnectionTest(w http.ResponseWriter, p ConnectionTestPanel) {
	t, ok := r.pages["admin/connection_form.html"]
	if !ok {
		http.Error(w, "template tidak ditemukan", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "connection-test", p); err != nil {
		log.Printf("render connection-test gagal: %v", err)
		http.Error(w, "gagal render hasil tes", http.StatusInternalServerError)
	}
}
