package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/go-chi/chi/v5"
)

// ActivityJSONRoutes is separate from the HTML/redirect admin middleware.
func ActivityJSONRoutes(r chi.Router, reader activity.ActivityReader, validator auth.SessionValidator, secure bool) {
	r.Route("/api/admin/activity-logs", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				cookie, err := r.Cookie(auth.SessionCookieName)
				if err != nil || cookie.Value == "" {
					activityAPIError(w, 401, "UNAUTHENTICATED", "Silakan masuk untuk melanjutkan.")
					return
				}
				info, err := validator.Validate(r.Context(), cookie.Value)
				if errors.Is(err, auth.ErrInvalidSession) || (err == nil && (!info.User.IsActive || info.User.ID <= 0)) {
					auth.ClearSessionCookie(w, secure)
					activityAPIError(w, 401, "SESSION_EXPIRED", "Sesi sudah berakhir. Silakan masuk kembali.")
					return
				}
				if err != nil {
					activityAPIError(w, 503, "SESSION_UNAVAILABLE", "Sesi belum dapat diperiksa. Coba lagi.")
					return
				}
				if info.NewToken != "" {
					auth.SetSessionCookie(w, info.NewToken, secure)
				}
				if !info.User.IsPlatformAdmin {
					activityAPIError(w, 403, "ADMIN_REQUIRED", "Halaman ini hanya dapat diakses administrator platform.")
					return
				}
				next.ServeHTTP(w, r)
			})
		})
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			f, err := activityFilters(r, time.Now())
			if err != nil {
				activityAPIError(w, 400, "INVALID_FILTER", err.Error())
				return
			}
			page, err := reader.List(r.Context(), f)
			if err != nil {
				activityAPIError(w, 503, "ACTIVITY_UNAVAILABLE", "Layanan log belum tersedia. Pastikan Loki dan collector sudah berjalan.")
				return
			}
			_ = json.NewEncoder(w).Encode(page)
		})
		r.Get("/{eventID}", func(w http.ResponseWriter, r *http.Request) {
			row, err := reader.Get(r.Context(), chi.URLParam(r, "eventID"))
			if errors.Is(err, activity.ErrActivityNotFound) {
				activityAPIError(w, 404, "ACTIVITY_NOT_FOUND", "Aktivitas tidak ditemukan atau sudah tidak tersedia.")
				return
			}
			if err != nil {
				activityAPIError(w, 503, "ACTIVITY_UNAVAILABLE", "Detail log belum dapat dimuat. Coba lagi.")
				return
			}
			_ = json.NewEncoder(w).Encode(row)
		})
	})
}

func activityAPIError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func activityFilters(r *http.Request, now time.Time) (activity.ActivityFilter, error) {
	q := r.URL.Query()
	f := activity.ActivityFilter{Search: strings.TrimSpace(q.Get("search")), Status: q.Get("status"), Category: q.Get("category"), Start: now.Add(-7 * 24 * time.Hour), End: now, Page: 1, PageSize: 8}
	if len(f.Search) > 200 {
		return f, errors.New("Pencarian maksimal 200 karakter.")
	}
	if f.Status != "" && f.Status != "all" && f.Status != "success" && f.Status != "failed" && f.Status != "warning" && f.Status != "started" {
		return f, errors.New("Filter status tidak valid.")
	}
	if f.Category != "" && f.Category != "all" && f.Category != "sql" && f.Category != "http" && f.Category != "admin" && f.Category != "user" {
		return f, errors.New("Kategori aktivitas tidak valid.")
	}
	// Numeric timezone avoids relying on a host-installed IANA tz database on Windows.
	wib := time.FixedZone("WIB", 7*60*60)
	if q.Get("from") != "" {
		start, err := time.ParseInLocation("2006-01-02", q.Get("from"), wib)
		if err != nil || start.Year() < 1971 || start.Year() > 2200 {
			return f, errors.New("Tanggal awal tidak valid.")
		}
		f.Start = start
	}
	if q.Get("to") != "" {
		end, err := time.ParseInLocation("2006-01-02", q.Get("to"), wib)
		if err != nil || end.Year() < 1971 || end.Year() > 2200 {
			return f, errors.New("Tanggal akhir tidak valid.")
		}
		f.End = end.AddDate(0, 0, 1)
		if q.Get("from") == "" {
			f.Start = f.End.Add(-7 * 24 * time.Hour)
		}
	}
	if !f.Start.Before(f.End) || f.End.Sub(f.Start) > 30*24*time.Hour {
		return f, errors.New("Pilih rentang tanggal berurutan, maksimal 30 hari.")
	}
	for _, key := range []string{"page", "pageSize"} {
		if q.Get(key) == "" {
			continue
		}
		n, err := strconv.Atoi(q.Get(key))
		if err != nil || n < 1 || (key == "pageSize" && n > 100) || (key == "page" && n > 1000000) {
			return f, errors.New("Paginasi tidak valid.")
		}
		if key == "page" {
			f.Page = n
		} else {
			f.PageSize = n
		}
	}
	return f, nil
}
