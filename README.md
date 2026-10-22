# tubes-1-web-based-development: Pathgres

Tugas Besar IF3110 Pengembangan Aplikasi Web, Milestone 1.
Platform web untuk mengatur akses, menjelajahi, dan mencatat perubahan schema database PostgreSQL.

- Backend: Go (`chi`, `pgx`)
- Frontend: Go template (server-rendered) dengan htmx dan CodeMirror
- Database platform dan database target: PostgreSQL 16
- Deadline Milestone 1: 25 Oktober 2026, 13.10 WIB

## Prasyarat

| Alat | Keterangan |
|------|------------|
| Go | versi sesuai `go.mod` |
| Docker Desktop | harus berstatus "Engine running" sebelum `docker compose` |
| `make` | opsional (di Windows biasanya tidak ada, lihat alternatif PowerShell di bawah) |

## Menjalankan untuk Development

### 1. Siapkan konfigurasi

```bash
cp .env.example .env        # PowerShell: copy .env.example .env
```

`.env` tidak ikut ter-commit (ada di `.gitignore`). Jangan menaruh password asli di `.env.example`.

### 2. Nyalakan database

```bash
docker compose up -d
docker compose ps
```

Tiga container berjalan:

| Service | Port di laptop | Fungsi |
|---------|----------------|--------|
| `platform-db` | `5432` (bisa diubah, lihat di bawah) | database internal platform: user, group, session, grants, riwayat migration |
| `target-db` | `5433` | contoh database target yang dikelola platform |
| `test-db` | `5434` | database khusus integration test (data hilang saat container mati) |

**Kalau port 5432 sudah dipakai** (misalnya PostgreSQL lokal atau container proyek lain), `platform-db` gagal start dengan `port is already allocated`. Pindahkan port lewat `.env`:

```
PLATFORM_DB_PORT=5435
PLATFORM_DB_URL=postgres://platform:platform@localhost:5435/platform?sslmode=disable
```

Kedua baris harus memakai port yang sama. Tanpa `PLATFORM_DB_PORT`, port tetap `5432`, jadi anggota tim lain tidak terpengaruh.

### 3. Jalankan aplikasi

Dengan `make`:

```bash
make run
```

Tanpa `make` (PowerShell), muat `.env` ke sesi lalu jalankan:

```powershell
Get-Content .env | Where-Object { $_ -match '^\s*[^#\s]' } | ForEach-Object {
  $k, $v = $_ -split '=', 2
  Set-Item "env:$k" $v
}
go run .
```

`go run .` tidak membaca `.env` sendiri. Saat berhasil, aplikasi tidak mencetak apa pun dan terminal tetap terpakai. Saat start, aplikasi menjalankan migrasi skema internal otomatis dari folder `migrations/` dan mencatatnya di tabel `schema_migrations`.

### 4. Cek hasilnya

```bash
docker compose exec platform-db psql -U platform -d platform -c "\dt"
```

Harus ada 8 tabel: `users`, `groups`, `memberships`, `db_connections`, `grants`, `sessions`, `migration_history`, `schema_migrations`.

```bash
curl -i http://localhost:8080/static/style.css   # 200
curl -i http://localhost:8080/admin              # 403 (login belum diimplementasi)
```

### Reset database dari nol

```bash
docker compose down -v
docker compose up -d
```

`-v` menghapus volume, jadi semua data development hilang. Wajib dilakukan setelah skema di `migrations/0001_init.sql` diubah, karena runner tidak mengulang file yang sudah tercatat.

## Konfigurasi (`.env`)

| Variabel | Fungsi | Default |
|----------|--------|---------|
| `APP_ADDR` | alamat server | `:8080` |
| `APP_ENV` | environment | `development` |
| `PLATFORM_DB_URL` | koneksi ke database platform (wajib) | tidak ada |
| `PLATFORM_DB_PORT` | port `platform-db` di laptop (hanya dibaca Docker Compose) | `5432` |
| `TEST_DB_URL` | koneksi database test | lihat `.env.example` |
| `COOKIE_SECURE` | `true` bila diakses lewat HTTPS | `false` |
| `ADMIN_USERNAME`, `ADMIN_PASSWORD` | akun Platform Admin awal (dibaca saat seed) | kosong |
| `SESSION_IDLE_TIMEOUT` | idle timeout session | `30m` |
| `SESSION_ABSOLUTE_TIMEOUT` | umur maksimum session | `8h` |
| `SESSION_ROTATE_INTERVAL` | interval rotasi session ID | `15m` |
| `SESSION_GRACE_PERIOD` | masa berlaku ID lama setelah rotasi | `30s` |

Nilai timeout di atas adalah usulan awal, bukan keputusan final (lihat tabel keputusan desain).

## Test

```bash
make test               # unit test (go test ./... -short)
make test-integration   # semua test, butuh test-db (docker compose up -d)
make lint               # golangci-lint
```

Tanpa `make`: `go test ./... -short` dan `go test ./... -count=1`.

Aturan test:
- Setiap fitur backend minimal punya tiga kasus: **berhasil, gagal, dan akses ditolak**.
- Integration test memakai `TEST_DB_URL` dan harus dilewati saat `-short`:

```go
if testing.Short() {
	t.Skip("integration test")
}
```

- Waktu pada test session diinjeksi lewat `clock`, jangan memakai `time.Now()` langsung.

## Struktur Proyek

```
main.go                  titik masuk: config, pool, migrasi, server
migrations/              skema database internal platform (file .sql, di-embed)
internal/
  config/                pembacaan environment variable
  db/                    pool koneksi dan runner migrasi skema internal
  auth/                  password, autentikasi, middleware, handler login/logout
  session/               session manager, cookie, rotasi, CSRF
  access/                policy akses (siapa boleh apa), terpisah dari auth
  admin/                 halaman Platform Admin (users, groups, connections)
  dbm/                   koneksi database target, schema viewer, migration user
  web/                   router, renderer template, templates/, static/
```

Aturan dependensi:
- `session` tidak boleh meng-import `auth`.
- Logika izin hanya ada di `access`. `auth` hanya menjawab "siapa", bukan "boleh apa".
- Fitur baru memasang route lewat `Routes(r chi.Router)` di paketnya sendiri, tanpa mengedit router bersama.

## Pembagian Tugas

| Anggota | Bagian | Paket utama |
|---------|--------|-------------|
| Arin | Authentication, session, Platform Admin (kelola user) | `auth`, `session`, `admin/users_*` |
| Vincent | Authorization, user group, pemberian akses | `access`, `admin/groups_*` |
| Majiid | Database management, migration | `dbm`, `admin/connections_*` |

Kontrak yang dipakai bersama:

| Kontrak | Pemilik | Fungsi |
|---------|---------|--------|
| `auth.UserFromContext(ctx)` | Arin | user yang sedang login |
| `auth.AuthMiddleware`, `auth.RequireAdmin`, `auth.CSRFProtect` | Arin | pelindung route |
| `session.CSRFToken(ctx)` | Arin | token CSRF untuk hidden field form POST |
| `session.Revoker.RevokeSessionsByUser` | Arin | mencabut session saat akses berubah |
| `access.Policy.Can(ctx, userID, connID, action)` | Vincent | pengecekan izin per database |

Kalau perlu mengubah kontrak, kabari pemilik dan anggota lain sebelum merge.

## Skema Database Platform

| Tabel | Isi |
|-------|-----|
| `users` | akun (`username` unik), hash password, `is_platform_admin`, `is_active` |
| `groups`, `memberships` | group dan anggotanya |
| `db_connections` | koneksi ke database target |
| `grants` | akses **group** ke database dengan level `view` (View Only) atau `edit` (View + Edit) |
| `sessions` | session di backend: hash token, idle dan absolute expiration, rotasi, grace period, CSRF, `revoked_at` |
| `migration_history` | riwayat migration per database: versi, SQL, pelaksana, waktu, status |
| `schema_migrations` | pencatat file migrasi skema internal yang sudah dijalankan |

Akses database hanya diberikan lewat group, tidak langsung ke user. Platform Admin bukan tingkat akses ke database.

## Keputusan Desain

Spesifikasi meminta tim menentukan dan mendokumentasikan hal berikut. Isi setelah disepakati.

| Topik | Keputusan | Alasan |
|-------|-----------|--------|
| Algoritma hash password | | |
| Panjang dan penyimpanan session ID | hash SHA-256 di DB, token asli hanya di cookie | |
| Idle timeout | | |
| Absolute expiration | | |
| Interval rotasi session | | |
| Grace period ID lama | | |
| Atribut cookie | | |
| Mekanisme CSRF | | |
| Akses efektif jika user di banyak group | level tertinggi | |
| Pembuatan Platform Admin pertama | | |
| Aturan username dan password | | |

## Status Implementasi

- [x] Skema database sesuai spesifikasi
- [x] Config, koneksi database, runner migrasi, server dasar
- [x] Kontrak auth, session, dan access (masih stub yang fail-closed)
- [ ] Autentikasi: repository user, login, seed admin
- [ ] Session: pembuatan, validasi, rotasi, revoke, CSRF
- [ ] Platform Admin: kelola user, group, koneksi, akses
- [ ] Database viewer dan migration
- [ ] Unit dan integration test

Saat ini semua route `/admin` mengembalikan 403 dan halaman `/login` belum ada.

## Alur Kerja Git

Branch dibuat dari branch utama dengan prefix:

| Prefix | Dipakai untuk |
|--------|---------------|
| `feat/` | fitur baru, misalnya `feat/auth`, `feat/session` |
| `fix/` | perbaikan bug |
| `chore/` | setup, dependency, konfigurasi |
| `docs/` | dokumentasi |
| `test/` | menambah test |

Pesan commit memakai format `tipe: deskripsi singkat` dalam bahasa Inggris, misalnya `feat: add user repository`.

Sebelum membuka pull request:

```bash
go build ./...
go vet ./...
go test ./... -short
```

Pull request yang mengubah skema (`migrations/`) atau kontrak antar anggota harus ditinjau oleh pemilik bagian terkait.

## Catatan Keamanan

- Password dan session ID tidak boleh muncul di URL, response, maupun log.
- Jangan commit `.env`, kredensial database, atau password admin.
- `db_connections.connection_url` berisi kredensial database target: jangan tampilkan ulang di UI dan jangan tulis ke log.
- Pengecekan izin selalu di backend. Menyembunyikan tombol di UI tidak cukup.