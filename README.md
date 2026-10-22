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
| `make` | opsional (di Windows biasanya tidak ada, perintah alternatifnya ada di bawah) |

## Menjalankan untuk Development

### 1. Siapkan konfigurasi

```bash
cp .env.example .env        # PowerShell: copy .env.example .env
```

Lalu **isi `ADMIN_PASSWORD` di `.env`** (minimal 8 karakter, maksimal 72 byte). Nilainya kosong di `.env.example` dan aplikasi menolak start kalau hanya salah satu dari `ADMIN_USERNAME` dan `ADMIN_PASSWORD` yang terisi.

`.env` tidak ikut ter-commit (ada di `.gitignore`). Jangan menaruh password asli di `.env.example`, dan jangan membagikan file `.env` (termasuk saat meng-zip repo).

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

```bash
go run .
```

atau `make run`. Aplikasi membaca file `.env` di folder kerja saat start (`config.LoadDotEnv`), jadi tidak perlu memuat variabel ke terminal secara manual. Variabel yang sudah ada di environment **tidak ditimpa** oleh `.env`, jadi nilai dari environment asli (CI, production) selalu menang. Jalankan dari root repo supaya `.env` ditemukan.

Saat berhasil, aplikasi hanya mencetak `admin awal dibuat: <username>` pada start pertama, lalu terminal tetap terpakai. Saat start, aplikasi:

1. menjalankan migrasi skema internal dari folder `migrations/` (dicatat di `schema_migrations`),
2. membuat akun Platform Admin awal dari `ADMIN_USERNAME` dan `ADMIN_PASSWORD` kalau belum ada.

Seed admin aman dijalankan berulang: kalau username sudah ada, tidak ada yang diubah, termasuk password yang sudah diganti.

### 4. Cek hasilnya

```bash
docker compose exec platform-db psql -U platform -d platform -c "\dt"
```

Harus ada 8 tabel: `users`, `groups`, `memberships`, `db_connections`, `grants`, `sessions`, `migration_history`, `schema_migrations`.

```bash
docker compose exec platform-db psql -U platform -d platform -c "SELECT id, username, is_platform_admin, is_active, left(password_hash, 7) AS hash_prefix FROM users"
```

Harus ada satu baris admin dengan `is_platform_admin = t` dan `hash_prefix` `$2a$12$` (bcrypt cost 12).

```bash
curl -i http://localhost:8080/static/style.css   # 200
```

Di Windows PowerShell, `curl` adalah alias `Invoke-WebRequest`. Pakai `curl.exe` supaya yang dipanggil curl asli: `curl.exe -i http://localhost:8080/static/style.css`.

Route `/login` sudah aktif. Route `/admin/*` belum punya halaman, jadi tanpa session valid diarahkan ke `/login`, dan untuk non-admin mengembalikan 403.

### Reset database dari nol

```bash
docker compose down -v
docker compose up -d
```

`-v` menghapus volume, jadi semua data development hilang (termasuk akun admin, yang dibuat ulang saat start berikutnya). Wajib dilakukan setelah skema di `migrations/0001_init.sql` diubah, karena runner tidak mengulang file yang sudah tercatat.

### Masalah yang sering muncul

| Pesan | Penyebab dan solusi |
|-------|---------------------|
| `PLATFORM_DB_URL wajib diisi` | `.env` tidak ditemukan di folder kerja, atau baris `PLATFORM_DB_URL` tidak ada. Jalankan dari root repo dan pastikan `.env` sudah dibuat dari `.env.example`. |
| `ADMIN_USERNAME dan ADMIN_PASSWORD harus diisi bersamaan` | Salah satunya kosong. Isi `ADMIN_PASSWORD` di `.env`. Kalau sudah terisi tapi pesan tetap muncul, ada variabel kosong dengan nama yang sama di sesi terminal (environment menang atas `.env`): `Remove-Item Env:ADMIN_PASSWORD` atau buka terminal baru. |
| `password minimal 8 karakter` | `ADMIN_PASSWORD` terlalu pendek. Ganti dengan yang lebih panjang. |
| `port is already allocated` | Port 5432 bentrok, lihat bagian "Nyalakan database". |
| Gagal koneksi ke database saat start | Container belum jalan atau port di `PLATFORM_DB_URL` tidak sama dengan `PLATFORM_DB_PORT`. Cek `docker compose ps`. |

## Konfigurasi (`.env`)

| Variabel | Fungsi | Default |
|----------|--------|---------|
| `APP_ADDR` | alamat server | `:8080` |
| `APP_ENV` | environment | `development` |
| `PLATFORM_DB_URL` | koneksi ke database platform (wajib) | tidak ada |
| `PLATFORM_DB_PORT` | port `platform-db` di laptop (hanya dibaca Docker Compose) | `5432` |
| `TEST_DB_URL` | koneksi database test | lihat `.env.example` |
| `COOKIE_SECURE` | `true` bila diakses lewat HTTPS | `false` |
| `ADMIN_USERNAME`, `ADMIN_PASSWORD` | akun Platform Admin awal, dibaca saat seed. Harus diisi bersamaan; kosong keduanya berarti seed dilewati | kosong |
| `SESSION_IDLE_TIMEOUT` | idle timeout session | `30m` |
| `SESSION_ABSOLUTE_TIMEOUT` | umur maksimum session | `8h` |
| `SESSION_ROTATE_INTERVAL` | interval rotasi session ID | `15m` |
| `SESSION_GRACE_PERIOD` | masa berlaku ID lama setelah rotasi | `30s` |

Nilai session di atas adalah keputusan tim untuk bagian auth dan session (lihat tabel Keputusan Desain).

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
main.go                  titik masuk: config, pool, migrasi, seed admin, server
migrations/              skema database internal platform (file .sql, di-embed)
internal/
  config/                pembacaan environment variable dan file .env
  db/                    pool koneksi dan runner migrasi skema internal
  auth/                  user store, validasi dan hash password, seed admin,
                         middleware, handler login/logout
  session/               session manager, cookie, rotasi, CSRF
  access/                policy akses (siapa boleh apa), terpisah dari auth
  admin/                 halaman Platform Admin (users, groups, connections)
  dbm/                   koneksi database target, schema viewer, migration user
  web/                   router, renderer template, templates/, static/
```

Isi paket `auth`:

| File | Isi |
|------|-----|
| `store.go` | interface `Store` dan implementasi PostgreSQL (`PGStore`): `GetByUsername`, `GetByID`, `Create`. `Account` memuat hash password dan tidak keluar dari paket `auth`; yang dibagikan ke paket lain adalah `User`. |
| `validate.go` | normalisasi dan validasi username dan password |
| `users.go` | `CreateUser`: satu-satunya pintu membuat akun (normalisasi, validasi, hash, simpan) |
| `seed.go` | `SeedAdmin`: membuat Platform Admin awal secara idempotent |
| `password.go` | hash dan verifikasi password (bcrypt) |
| `middleware.go` | `AuthMiddleware`, `RequireAdmin`, `CSRFProtect`, helper cookie |

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

Spesifikasi meminta tim menentukan dan mendokumentasikan hal berikut.

| Topik | Keputusan | Alasan |
|-------|-----------|--------|
| Algoritma hash password | bcrypt, cost 12 | Standar yang matang dan tersedia di `golang.org/x/crypto`. Salt dibuat otomatis, dan cost bisa dinaikkan nanti tanpa mengubah format penyimpanan. |
| Panjang dan penyimpanan session ID | 32 byte dari `crypto/rand`, dikodekan base64url. Di DB hanya hash SHA-256, token asli hanya di cookie | Token tidak bisa ditebak, dan kalau tabel `sessions` bocor, token yang masih berlaku tidak ikut bocor. |
| Idle timeout | 30 menit | Cukup untuk pemakaian normal dan membatasi risiko session yang ditinggalkan. |
| Absolute expiration | 8 jam | Setara satu hari kerja. Memaksa login ulang walau user aktif terus, dan rotasi tidak memperpanjangnya. |
| Interval rotasi session | 15 menit | Memperkecil jendela pemakaian token yang dicuri. |
| Grace period ID lama | 30 detik | Request paralel atau response yang terlambat tidak membuat user terlempar saat cookie baru belum sampai. |
| Atribut cookie | `HttpOnly`, `SameSite=Lax`, `Secure` saat `COOKIE_SECURE=true`. Nama cookie tetap `pathgres_session`, hanya nilainya yang berganti saat rotasi | `HttpOnly` mencegah JavaScript membaca cookie. `Lax` tetap mengizinkan navigasi biasa ke aplikasi. `Secure` hanya aktif di HTTPS agar development di HTTP tetap jalan. |
| Mekanisme CSRF | Request terautentikasi: synchronizer token per session (`csrf_secret`), dikirim lewat field form `csrf_token` atau header `X-CSRF-Token` dan dibandingkan dengan `subtle.ConstantTimeCompare`. `POST /login`: pengecekan `Origin` dan `Sec-Fetch-Site` lewat `http.CrossOriginProtection`. `SameSite=Lax` sebagai lapisan tambahan | Login belum punya session sehingga belum ada token. Cek Origin menutup login CSRF tanpa state tambahan. |
| Akses efektif jika user di banyak group | level tertinggi (asumsi sementara, diputuskan Vincent) | |
| Pembuatan Platform Admin pertama | Seed dari `ADMIN_USERNAME` dan `ADMIN_PASSWORD` saat start, hanya jika username belum ada | Tidak ada pendaftaran publik dan tidak ada password di repo. Seed yang idempotent aman dijalankan di setiap restart. |
| Aturan username dan password | Username dinormalisasi ke huruf kecil lalu harus 3-32 karakter dari `a-z`, `0-9`, `_`, `.`. Password minimal 8 karakter dan maksimal 72 byte | bcrypt hanya memakai 72 byte pertama, jadi password yang lebih panjang ditolak agar tidak terpotong diam-diam. Normalisasi mencegah `Admin` dan `admin` menjadi dua akun. |

## Status Implementasi

- [x] Skema database sesuai spesifikasi
- [x] Config (termasuk pembacaan `.env`), koneksi database, runner migrasi, server dasar
- [x] Kontrak auth, session, dan access (masih stub yang fail-closed)
- [x] Autentikasi: repository user, validasi, seed admin
- [ ] Autentikasi: login dan logout
- [ ] Session: pembuatan, validasi, rotasi, revoke, CSRF
- [ ] Platform Admin: kelola user, group, koneksi, akses
- [ ] Database viewer dan migration
- [ ] Unit dan integration test

Login dan logout sudah terpasang di `main.go` (tes handler menyusul). Halaman `/` masih sementara, dan route `/admin/*` belum punya halaman.

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

- Password dan session ID tidak boleh muncul di URL, response, maupun log. Seed admin hanya mencatat username, tidak pernah password.
- Jangan commit `.env`, kredensial database, atau password admin.
- `db_connections.connection_url` berisi kredensial database target: jangan tampilkan ulang di UI dan jangan tulis ke log.
- Pengecekan izin selalu di backend. Menyembunyikan tombol di UI tidak cukup.
- Hash password (`Account.PasswordHash`) tidak pernah masuk ke `auth.User` di context, dan tidak boleh dikirim ke template.