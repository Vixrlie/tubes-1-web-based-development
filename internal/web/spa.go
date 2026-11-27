package web

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// SPAHandler menyajikan hasil build SPA (misal frontend/dist) dengan fallback
// ke index.html untuk client-side routing React Router. Path /api/* tidak pernah
// di-fallback ke HTML agar klien JSON menerima 404 yang jelas.
func SPAHandler(distDir string) http.Handler {
	fsys := os.DirFS(distDir)
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"Endpoint API tidak ditemukan."}}`))
			return
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		clean := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if clean != "" {
			if info, err := fs.Stat(fsys, clean); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		indexBytes, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Build SPA belum tersedia. Jalankan 'npm --prefix frontend run build' atau gunakan Vite dev server di http://localhost:5173.\n"))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(indexBytes)
		}
	})
}
