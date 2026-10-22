package access

import "context"

// Action = tindakan yang diminta user pada satu koneksi database.
// Urutan dari paling rendah ke tinggi: view < edit.
type Action string

const (
	// View Only: lihat schema, data, dan riwayat migration.
	ActionView Action = "view"
	// View + Edit: semua View Only, ditambah buat, ubah, dan jalankan migration.
	ActionEdit Action = "edit"
)

// Policy memutuskan apakah user boleh melakukan aksi pada koneksi tertentu.
type Policy interface {
	Can(ctx context.Context, userID int64, connID int64, a Action) (bool, error)
}

// DenyAll menolak semua request. Dipakai sebagai default sampai policy asli jadi.
type DenyAll struct{}

func (DenyAll) Can(ctx context.Context, userID int64, connID int64, a Action) (bool, error) {
	return false, nil
}
