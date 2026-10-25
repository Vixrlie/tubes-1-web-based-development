package dbm

// Halaman Schema Diff (BON-02/03): GET memilih koneksi pembanding, POST
// menampilkan selisih + kandidat SQL yang bisa disimpan sebagai draft lalu
// diedit di halaman editor sebelum dieksekusi.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

// diffPage = data templates/diff.html fe-components.
type diffPage struct {
	web.Shell
	Title       string
	Crumbs      []web.Crumb
	FormAction  string // POST diff (pembanding)
	DraftAction string // POST CreateDraft (endpoint migration yang sudah ada)
	Fields      []map[string]any // dropdown koneksi pembanding
	Actions     []web.Button     // tombol form utama
	Error       string
	HasResult   bool
	RefName     string
	Count       int
	Table       web.Table
	DraftSQL    string
	SQLNote     string
	CanDraft    bool // izin edit pada koneksi ini
}

// kindLabel & statusLabel memetakan DiffItem ke label tabel.
func kindLabel(kind string) string {
	switch kind {
	case "table":
		return "Tabel"
	case "column":
		return "Kolom"
	case "pk":
		return "Primary Key"
	case "index":
		return "Index"
	case "fk":
		return "Foreign Key"
	}
	return kind
}

func statusLabel(status string) web.TableCell {
	switch status {
	case "tambah":
		return web.TableCell{Text: "Tambah", Variant: "success"}
	case "ubah":
		return web.TableCell{Text: "Ubah", Variant: "warning"}
	default:
		return web.TableCell{Text: "Hapus", Variant: "error"}
	}
}

// refOptions membangun daftar koneksi lain yang boleh dilihat user.
func (h *Handlers) refOptions(r *http.Request, connID int64) []map[string]any {
	rows, err := h.platform.Query(r.Context(),
		`SELECT id, name FROM db_connections WHERE id <> $1 ORDER BY name`, connID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return nil
	}
	var opts []map[string]any
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil
		}
		if allowed, err := h.policy.Can(r.Context(), user.ID, id, access.ActionView); err != nil || !allowed {
			continue
		}
		opts = append(opts, map[string]any{
			"Value": strconv.FormatInt(id, 10),
			"Label": name,
		})
	}
	return opts
}

// basePage membangun shell + crumb halaman diff.
func (h *Handlers) basePage(r *http.Request, connID int64) (diffPage, bool) {
	name, err := h.connectionName(r.Context(), connID)
	if errors.Is(err, ErrConnNotFound) {
		return diffPage{}, false
	}
	if err != nil {
		return diffPage{}, false
	}
	base := migrationsHref(connID)
	return diffPage{
		Shell: h.migrationShell(r.Context(), connID, name),
		Title: "Schema Diff",
		Crumbs: []web.Crumb{
			{Label: name, Href: "/databases"},
			{Label: "Migrasi", Href: base},
			{Label: "Schema Diff"},
		},
		FormAction:  base + "/diff",
		DraftAction: base,
		Actions: []web.Button{
			{Label: "Bandingkan", Type: "submit"},
			{Label: "Batal", Variant: "secondary", Href: base},
		},
	}, true
}

// diffForm menampilkan form pemilihan koneksi pembanding.
func (h *Handlers) diffForm(w http.ResponseWriter, r *http.Request) {
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	page, ok := h.basePage(r, connID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	opts := h.refOptions(r, connID)
	if len(opts) == 0 {
		page.Error = "Tidak ada koneksi lain yang boleh Anda lihat untuk dijadikan pembanding."
	} else {
		page.Fields = []map[string]any{{
			"Label":       "Koneksi pembanding",
			"Name":        "ref",
			"Options":     opts,
			"Placeholder": "Pilih koneksi",
			"Required":    true,
			"Helper":      "Schema pada koneksi ini dijadikan kondisi yang diinginkan.",
		}}
	}
	h.render(w, http.StatusOK, "diff.html", page)
}

// diffRun membandingkan kondisi saat ini dengan koneksi pembanding (POST).
func (h *Handlers) diffRun(w http.ResponseWriter, r *http.Request) {
	connID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	page, ok := h.basePage(r, connID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	page.Fields = h.refOptions(r, connID)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "permintaan tidak valid", http.StatusBadRequest)
		return
	}
	refID, err := strconv.ParseInt(r.PostFormValue("ref"), 10, 64)
	if err != nil || refID <= 0 || refID == connID {
		page.Error = "Koneksi pembanding tidak valid."
		h.render(w, http.StatusBadRequest, "diff.html", page)
		return
	}
	// Izin view di koneksi pembanding diperiksa eksplisit (bukan lewat URL).
	user, uok := auth.UserFromContext(r.Context())
	if !uok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if allowed, err := h.policy.Can(r.Context(), user.ID, refID, access.ActionView); err != nil || !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	refName, err := h.connectionName(r.Context(), refID)
	if err != nil {
		page.Error = "Koneksi pembanding tidak ditemukan."
		h.render(w, http.StatusNotFound, "diff.html", page)
		return
	}

	curPool, curErr := h.registry.Get(r.Context(), connID)
	refPool, refErr := h.registry.Get(r.Context(), refID)
	if curErr != nil || refErr != nil || curPool == nil || refPool == nil {
		page.Error = "Salah satu database tidak dapat dihubungi; diff tidak dijalankan."
		h.render(w, http.StatusServiceUnavailable, "diff.html", page)
		return
	}
	cur, err := loadSchemaState(r.Context(), curPool)
	if err != nil {
		h.serverError(w, "baca schema saat ini", err)
		return
	}
	ref, err := loadSchemaState(r.Context(), refPool)
	if err != nil {
		h.serverError(w, "baca schema pembanding", err)
		return
	}

	items := DiffSchemas(cur, ref)
	page.HasResult = true
	page.RefName = refName
	page.Count = len(items)
	page.CanDraft = func() bool {
		allowed, err := h.policy.Can(r.Context(), user.ID, connID, access.ActionEdit)
		return err == nil && allowed
	}()

	page.Table = web.Table{
		Title: "Perbedaan schema",
		Tag:   web.TableCell{Text: strconv.Itoa(len(items)) + " selisih"},
		Columns: []web.TableColumn{
			{Key: "tabel", Label: "Tabel", Bold: true},
			{Key: "jenis", Label: "Jenis"},
			{Key: "status", Label: "Status"},
			{Key: "detail", Label: "Detail"},
			{Key: "sql", Label: "SQL"},
		},
		Empty: "Tidak ada perbedaan — kedua schema sudah identik.",
	}
	for _, it := range items {
		shortSQL := strings.Join(strings.Fields(it.SQL), " ")
		if len(shortSQL) > 60 {
			shortSQL = shortSQL[:60] + "…"
		}
		if shortSQL == "" {
			shortSQL = "—"
		}
		page.Table.Rows = append(page.Table.Rows, web.TableRow{
			"tabel":  {Text: it.Qualified()},
			"jenis":  {Text: kindLabel(it.Kind)},
			"status": statusLabel(it.Status),
			"detail": {Text: it.Detail},
			"sql":    {Text: shortSQL},
		})
	}

	page.DraftSQL = GenerateSQL(items)
	page.SQLNote = "Kandidat diarahkan ke kondisi saat ini → " + refName +
		". Tinjau dan ubah dulu sebelum disimpan sebagai draft; bila perlu, pastikan schema tujuan sudah ada."
	if len(items) == 0 {
		page.SQLNote = "Tidak ada selisih, tidak ada kandidat SQL."
	}
	h.render(w, http.StatusOK, "diff.html", page)
}
