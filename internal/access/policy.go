package access

import "context"

// Action adalah tindakan yang diminta user pada satu koneksi database.
// Urutan dari paling rendah ke tinggi: view < edit.
type Action string

const (
	// ActionView: lihat schema, data, dan riwayat migration.
	ActionView Action = "view"
	// ActionEdit: semua ActionView, ditambah membuat, mengubah, dan
	// menjalankan migration.
	ActionEdit Action = "edit"
)

// Rank mengembalikan peringkat sebuah action. Action yang tidak dikenal
// bernilai 0 sehingga selalu ditolak.
func Rank(a Action) int {
	switch a {
	case ActionEdit:
		return 2
	case ActionView:
		return 1
	default:
		return 0
	}
}

// AtLeast melaporkan apakah have cukup untuk memenuhi want.
func AtLeast(have, want Action) bool {
	return Rank(have) >= Rank(want) && Rank(want) > 0
}

// Effective mengambil level tertinggi dari kumpulan grant. Hasil kedua false
// bila tidak ada grant yang valid. Inilah aturan akses efektif: user yang
// tergabung di beberapa group mendapat level tertinggi di antara group-group
// tersebut, dan tanpa grant berarti ditolak.
func Effective(levels []Action) (Action, bool) {
	best := Action("")
	for _, l := range levels {
		if Rank(l) > Rank(best) {
			best = l
		}
	}
	if Rank(best) == 0 {
		return "", false
	}
	return best, true
}

// Policy memutuskan apakah user boleh melakukan aksi pada koneksi tertentu.
// Platform Admin tidak memiliki akses implisit ke database target; keputusan
// hanya berasal dari grant group.
type Policy interface {
	Can(ctx context.Context, userID int64, connID int64, a Action) (bool, error)
}

// DenyAll menolak semua request. Dipakai sebagai default sampai policy asli
// dipasang.
type DenyAll struct{}

// Can selalu mengembalikan false.
func (DenyAll) Can(ctx context.Context, userID int64, connID int64, a Action) (bool, error) {
	return false, nil
}

// LevelSource memuat level grant seorang user pada satu koneksi. Pemisahan ini
// menjaga logika akses tetap murni dan mudah diuji tanpa database.
type LevelSource interface {
	Levels(ctx context.Context, userID int64, connID int64) ([]Action, error)
}

// PostgresPolicy adalah Policy yang membaca grant dari database lalu
// menerapkan aturan Effective.
type PostgresPolicy struct {
	src LevelSource
}

// NewPolicy membungkus LevelSource menjadi Policy.
func NewPolicy(src LevelSource) *PostgresPolicy {
	return &PostgresPolicy{src: src}
}

// Can mengembalikan true bila level efektif user memenuhi action yang diminta.
// Kegagalan memuat data dianggap error dan pemanggil wajib memperlakukannya
// sebagai penolakan (fail closed).
func (p *PostgresPolicy) Can(ctx context.Context, userID int64, connID int64, a Action) (bool, error) {
	if Rank(a) == 0 {
		return false, nil
	}
	levels, err := p.src.Levels(ctx, userID, connID)
	if err != nil {
		return false, err
	}
	best, ok := Effective(levels)
	if !ok {
		return false, nil
	}
	return AtLeast(best, a), nil
}
