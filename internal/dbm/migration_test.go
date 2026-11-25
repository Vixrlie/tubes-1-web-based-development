package dbm

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDeriveName(t *testing.T) {
	cases := []struct {
		name, sql, want string
	}{
		{"komentar pertama", "-- Tambah kolom notes\nALTER TABLE t ADD COLUMN notes TEXT", "Tambah kolom notes"},
		{"komentar setelah baris kosong", "\n\n-- Buat tabel\nCREATE TABLE x (id BIGINT)", "Buat tabel"},
		{"statement pertama bila tanpa komentar", "CREATE TABLE x (id BIGINT PRIMARY KEY); DROP TABLE y;", "CREATE TABLE x (id BIGINT PRIMARY KEY)"},
		{"satu baris panjang dipotong", "-- " + strings.Repeat("a", 120), strings.Repeat("a", 80) + "…"},
		{"kosong", "   \n  ", "Migration tanpa nama"},
		{"hanya komentar kosong", "--", "--"}, // SQL berisi hanya komentar: nama = teks komentar apa adanya
	}
	for _, tc := range cases {
		if got := DeriveName(tc.sql); got != tc.want {
			t.Errorf("%s: DeriveName = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDefinitionChecksum(t *testing.T) {
	base := DefinitionChecksum("CREATE TABLE x (id INT)", "DROP TABLE x", "")
	if base != DefinitionChecksum("CREATE TABLE x (id INT)", "DROP TABLE x", "") {
		t.Error("checksum tidak stabil")
	}
	if len(base) != 64 {
		t.Errorf("panjang checksum = %d, want 64 hex SHA-256", len(base))
	}
	// Setiap bagian definisi ikut terikat: mengubah down atau alasan juga
	// mengubah checksum, sehingga revisi pasca-approval terdeteksi.
	for name, other := range map[string]string{
		"up":     DefinitionChecksum("CREATE TABLE y (id INT)", "DROP TABLE x", ""),
		"down":   DefinitionChecksum("CREATE TABLE x (id INT)", "DROP TABLE y", ""),
		"reason": DefinitionChecksum("CREATE TABLE x (id INT)", "DROP TABLE x", "alasan"),
	} {
		if other == base {
			t.Errorf("checksum tidak berubah saat %s diubah", name)
		}
	}
	// Pemisah mencegah dua definisi berbeda menghasilkan gabungan yang sama.
	if DefinitionChecksum("ab", "c", "") == DefinitionChecksum("a", "bc", "") {
		t.Error("batas up/down tidak terikat dalam checksum")
	}
}

func TestMigrationInputNormalize(t *testing.T) {
	cases := []struct {
		name string
		in   MigrationInput
		want error
	}{
		{"reversible valid", MigrationInput{UpSQL: "CREATE TABLE x (id INT)", DownSQL: "DROP TABLE x"}, nil},
		{"irreversible dengan alasan", MigrationInput{UpSQL: "DROP TABLE x", Irreversible: true, IrreversibleReason: "data hilang"}, nil},
		{"up kosong", MigrationInput{UpSQL: "  ", DownSQL: "DROP TABLE x"}, ErrEmptySQL},
		{"down wajib bila reversible", MigrationInput{UpSQL: "CREATE TABLE x (id INT)"}, ErrMissingDown},
		{"alasan wajib bila irreversible", MigrationInput{UpSQL: "DROP TABLE x", Irreversible: true}, ErrMissingReason},
		{"irreversible tidak boleh punya down", MigrationInput{UpSQL: "DROP TABLE x", DownSQL: "SELECT 1", Irreversible: true, IrreversibleReason: "x"}, ErrIrreversibleDown},
		{"down sama dengan up", MigrationInput{UpSQL: "SELECT 1", DownSQL: " SELECT 1 "}, ErrDownSameAsUp},
		{"nama terlalu panjang", MigrationInput{Name: strings.Repeat("n", 81), UpSQL: "SELECT 1", DownSQL: "SELECT 2"}, ErrNameTooLong},
	}
	for _, tc := range cases {
		_, err := tc.in.Normalize()
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}

	got, err := MigrationInput{
		Name:  "  Tambah   kolom  ",
		UpSQL: "\n-- dari komentar\nALTER TABLE t ADD c INT\n", DownSQL: "ALTER TABLE t DROP c",
		IrreversibleReason: "diabaikan karena reversible",
	}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Tambah kolom" {
		t.Errorf("Name = %q, want spasi dirapikan", got.Name)
	}
	if got.IrreversibleReason != "" {
		t.Errorf("alasan irreversible tersisa pada migration reversible: %q", got.IrreversibleReason)
	}
	derived, _ := MigrationInput{UpSQL: "-- Buat tabel\nCREATE TABLE x (id INT)", DownSQL: "DROP TABLE x"}.Normalize()
	if derived.Name != "Buat tabel" {
		t.Errorf("nama turunan = %q, want Buat tabel", derived.Name)
	}
}

func TestSanitizePGError(t *testing.T) {
	// Password di connection string tidak boleh bocor ke riwayat.
	err := errors.New(`dial failed for postgres://alice:S3kr3t@db:5432/shop: connection refused`)
	got := sanitizePGError(err)
	if strings.Contains(got, "S3kr3t") {
		t.Errorf("password bocor: %q", got)
	}
	if !strings.Contains(got, "connection refused") {
		t.Errorf("pesan error hilang: %q", got)
	}

	// Pesan panjang dipotong (termasuk elipsis "…" 3 byte).
	long := errors.New(strings.Repeat("x", 5000))
	if got := sanitizePGError(long); len(got) > maxErrorLength+len("…") {
		t.Errorf("panjang = %d, want <= %d", len(got), maxErrorLength+len("…"))
	}
}

func TestVersionNow(t *testing.T) {
	when := time.Date(2026, 10, 25, 13, 10, 0, 0, time.UTC)
	if got := VersionNow(when); got != 202610251310 {
		t.Errorf("VersionNow = %d, want 202610251310", got)
	}
}

func TestMigrationFlags(t *testing.T) {
	at := time.Date(2026, 10, 25, 13, 10, 0, 0, time.UTC)
	m := Migration{CreatedAt: at, Checksum: "abc"}
	if m.CreatedAtLabel() != "25 Oct 2026 13:10" {
		t.Errorf("CreatedAtLabel = %q", m.CreatedAtLabel())
	}
	if !m.Reversible() || m.Locked() || !m.Editable() {
		t.Errorf("draft baru: reversible=%v locked=%v editable=%v", m.Reversible(), m.Locked(), m.Editable())
	}

	m.OpenRequest = &RequestRef{ID: 1, Action: ActionUp, Status: RequestPendingReview}
	if m.Editable() {
		t.Error("migration yang sedang diajukan tidak boleh diedit")
	}
	m.OpenRequest = nil
	m.LockedAt = &at
	if m.Editable() {
		t.Error("migration yang sudah dieksekusi tidak boleh diedit")
	}

	m.Validation = Validation{OK: true, Checksum: "abc"}
	if !m.ValidatedForCurrent() {
		t.Error("validasi untuk checksum yang sama harus dianggap berlaku")
	}
	m.Checksum = "def"
	if m.ValidatedForCurrent() {
		t.Error("validasi revisi lama tidak boleh berlaku untuk revisi baru")
	}

	m.IrreversibleReason = "drop kolom"
	if m.Reversible() {
		t.Error("migration beralasan irreversible dianggap reversible")
	}
}
