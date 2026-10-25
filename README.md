# tubes-1-web-based-development: Pathgres

Tugas Besar IF3110 Pengembangan Aplikasi Web, Milestone 1.
Platform web untuk mengatur akses, menjelajahi, dan mencatat perubahan schema database PostgreSQL.

- Backend: Go (`chi`, `pgx`)
- Frontend: Go template (server-rendered) dengan htmx dan CodeMirror
- Database platform dan database target: PostgreSQL 16; bonus MySQL/MariaDB untuk database target
- Deadline Milestone 1: 25 Oktober 2026, 13.10 WIB

## Prasyarat

| Alat | Keterangan |
|------|------------|
| Go | versi sesuai `go.mod` |
| Docker Engine + Compose plugin (atau Docker Desktop) | daemon harus berjalan sebelum `docker compose` |
| `make` | opsional (di Windows biasanya tidak ada, perintah alternatifnya ada di bawah) |

## Quick Start dari Nol

Satu rentetan perintah dari clone sampai server jalan:

```bash
git clone https://github.com/Vixrlie/tubes-1-web-based-development.git
cd tubes-1-web-based-development

cp .env.example .env
# lalu buka .env, isi ADMIN_PASSWORD minimal 8 karakter
# (ADMIN_USERNAME sudah default "admin")

docker compose up -d      # 4 container: PostgreSQL platform :5432, target :5433, test :5434 + MySQL :5435
go run .                  # server + migrasi skema + seed admin; atau: make run
```

Buka **http://localhost:8080**, login dengan `admin` / password dari `.env`.

Dari sini database platform masih kosong (belum ada koneksi, grup, tabel),
lanjut ke langkah **"Siapkan data awal"** di bawah supaya semua fitur
(viewer, migration, EXPLAIN, schema diff) benar-benar bisa dicoba.

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

Empat container berjalan:

| Service | Port di laptop | Fungsi |
|---------|----------------|--------|
| `platform-db` | `5432` (bisa diubah, lihat di bawah) | database internal platform: user, group, session, grants, riwayat migration |
| `target-db` | `5433` | contoh database target yang dikelola platform |
| `test-db` | `5434` | database khusus test (data hilang saat container mati) |
| `mysql-db` | `5435` | contoh database target MySQL/MariaDB untuk fitur bonus |

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
curl -i http://localhost:8080/static/css/app.css   # 200
```

Di Windows PowerShell, `curl` adalah alias `Invoke-WebRequest`. Pakai `curl.exe` supaya yang dipanggil curl asli: `curl.exe -i http://localhost:8080/static/css/app.css`.

Route `/login` sudah aktif. Route terproteksi (termasuk `/admin/*`) mengarahkan ke `/login` kalau belum ada session valid, dan mengembalikan 403 untuk user non-admin.

### 5. Siapkan data awal supaya semua fitur bisa dicoba

Dari nol, aplikasi masih kosong: belum ada koneksi target, grup, grant, user
kedua, maupun tabel. Tiga langkah berikut membuat satu alur lengkap
(viewer → migration → EXPLAIN → schema diff).

**a. Buat tabel contoh di database target** (wajib, agar viewer dan migration punya isi):

```bash
docker compose exec -T target-db psql -U target -d sample <<'SQL'
CREATE TABLE customers (
  id       BIGSERIAL PRIMARY KEY,
  name     text NOT NULL,
  phone    text,
  status   text NOT NULL DEFAULT 'aktif',
  priority int  NOT NULL DEFAULT 1,
  note     text,
  notes    text
);
CREATE INDEX idx_customers_name ON customers (name);
INSERT INTO customers (name, phone, status, priority)
SELECT 'Pelanggan ' || i, '0812' || i,
       CASE WHEN i % 3 = 0 THEN 'nonaktif' ELSE 'aktif' END, i % 5
FROM generate_series(1, 120) i;
SQL
```

Windows PowerShell: simpan SQL di atas ke `seed.sql`, lalu
`docker compose exec -T target-db psql -U target -d sample -f seed.sql`.

**b. Daftarkan semuanya lewat UI (login sebagai `admin`):**

1. **Admin → Koneksi Database → Tambah Koneksi**: Nama tampilan `target-demo`,
   Host `127.0.0.1`, Port `5433`, Database `sample`, Username `target`,
   Password `target` → **Simpan** → **Tes Koneksi** (harus berhasil).
2. **Admin → Pengguna → Tambah User**: username `peserta`, password
   minimal 8 karakter → **Simpan**.
3. **Admin → Grup → Grup Baru**: nama `peserta-grup` → **Simpan**. Di
   halaman detail grup: **Tambah Anggota** → pilih `peserta` **dan `admin`**
   → **Tambahkan**. (Platform Admin tidak otomatis punya akses database,
   makanya `admin` ikut dimasukkan grup.)
4. Di kartu **Akses Database** klik **Kelola Database** → **Tambah Database**
   → pilih `target-demo`, level **View + Edit** → **Tambahkan** →
   **Simpan Perubahan**.

**c. Coba seluruh alur (logout, lalu login sebagai `peserta`):**

`/databases` → schema → detail tabel (kolom/constraints/index) → data
berpaginasi → **Migration Baru** → **Simpan Draft** → **Preview & Eksekusi** →
**Jalankan EXPLAIN** (opsional centang ANALYZE, hasilnya di-rollback) → di kartu
**Konfirmasi eksekusi**, ketik nama database tujuan (`sample`), centang
persetujuan, lalu **Jalankan Migration** → riwayat ter-update.

**d. (Opsional) Schema diff antar dua koneksi:**

```bash
docker compose exec -T target-db psql -U target -d postgres -c "CREATE DATABASE banding;"
docker compose exec -T target-db psql -U target -d banding <<'SQL'
CREATE TABLE customers (id BIGSERIAL PRIMARY KEY, nama text NOT NULL, email text);
CREATE TABLE orders (id int PRIMARY KEY, customer_id int REFERENCES customers (id));
SQL
```

Daftarkan sebagai koneksi kedua (port `5433`, database `banding`), beri akses
**View Only** ke grup `peserta-grup` lewat **Kelola Database**, lalu buka
**Riwayat Migration → Schema Diff**, pilih koneksi `banding`, **Bandingkan**:
muncul daftar selisih + kandidat SQL yang bisa disimpan jadi draft lewat
**Buat Draft Migration**.

Kredensial database target berasal dari container: user `target`, password
`target`, port `5433` (database `sample` dan `banding`).

**e. (Bonus) Koneksi MySQL / MariaDB:**

`docker-compose.yml` juga menyediakan container `mysql-db` (port **5435**,
database `sample`, user `target`, password `target`, sudah berisi tabel contoh
`customers`). Container ini sudah ikut start dengan `docker compose up -d`.
Untuk menyalakannya sendiri:

```bash
docker compose up -d mysql-db
```

Daftarkan lewat **Admin → Koneksi Database → Tambah Koneksi**, pilih **Jenis
DBMS: MySQL / MariaDB**, isi host `127.0.0.1`, port `5435`, database `sample`,
user `target`, password `target`, lalu **Tes Koneksi**. Setelah diberi grant
lewat grup, viewer (schema/kolom/index/data), migration, dan EXPLAIN berjalan
sama seperti PostgreSQL.

Batasan yang perlu diketahui (dijelaskan apa adanya di UI):

- **Schema diff** hanya untuk dua koneksi dengan DBMS yang sama.
- **EXPLAIN** PostgreSQL menampilkan ringkasan plan terstruktur; EXPLAIN
  MySQL ditampilkan sebagai keluaran mentah (bentuk JSON-nya berbeda).
- **MySQL tidak memiliki DDL transaksional**: untuk migration multi-statement
  yang gagal di tengah, statement yang sudah jalan sebelumnya tidak bisa
  di-rollback. Gunakan satu statement per migration DDL di MySQL.

### Reset database dari nol

```bash
docker compose down -v
docker compose up -d
```

`-v` menghapus volume, jadi semua data development hilang (termasuk akun admin, yang dibuat ulang saat start berikutnya). Reset wajib dilakukan setelah file migration yang **sudah pernah dijalankan** diubah, karena runner tidak mengulang file yang sudah tercatat. Menambah file migration baru tidak perlu reset: file itu dijalankan otomatis saat start berikutnya.

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
| `APP_ENV` | environment; `production` mewajibkan `COOKIE_SECURE=true` dan gagal start bila tidak | `development` |
| `PLATFORM_DB_URL` | koneksi ke database platform (wajib) | tidak ada |
| `PLATFORM_DB_PORT` | port `platform-db` di laptop (hanya dibaca Docker Compose) | `5432` |
| `TEST_DB_URL` | koneksi database test | lihat `.env.example` |
| `COOKIE_SECURE` | `true` bila diakses lewat HTTPS; wajib `true` saat `APP_ENV=production` | `false` |
| `ADMIN_USERNAME`, `ADMIN_PASSWORD` | akun Platform Admin awal, dibaca saat seed. Harus diisi bersamaan; kosong keduanya berarti seed dilewati | kosong |
| `SESSION_IDLE_TIMEOUT` | idle timeout session | `30m` |
| `SESSION_ABSOLUTE_TIMEOUT` | umur maksimum session | `8h` |
| `SESSION_ROTATE_INTERVAL` | interval rotasi session ID | `15m` |
| `SESSION_GRACE_PERIOD` | masa berlaku ID lama setelah rotasi | `30s` |

Nilai session di atas adalah keputusan tim untuk bagian auth dan session (lihat tabel Keputusan Desain).

Kenaikan privilege (grant baru atau naik level, anggota baru pada grup yang sudah punya grant, promosi Platform Admin) menandai `sessions.force_rotate`. Request terautentikasi berikutnya merotasi token dan memperbarui cookie; absolute expiration tidak diperpanjang. Penurunan akses (downgrade/remove grant, hapus grup/koneksi, demosi admin) mencabut session terkait.

## Test

```bash
make test          # unit test (go test ./... -short)
go test ./... -count=1
go vet ./...
```

Aturan test:
- Setiap fitur backend minimal punya tiga kasus: **berhasil, gagal, dan akses ditolak**.
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
| `session.Revoker.RevokeSessionsByUser` | Arin | mencabut session saat akses turun atau dicabut |
| `session.Revoker.ForceRotateSessionsByUser` | Arin | memaksa rotasi token saat privilege naik |
| `access.Policy.Can(ctx, userID, connID, action)` | Vincent | pengecekan izin per database |


## Skema Database Platform

| Tabel | Isi |
|-------|-----|
| `users` | akun (`username` unik), hash password, `is_platform_admin`, `is_active` |
| `groups`, `memberships` | group dan anggotanya |
| `db_connections` | koneksi ke database target |
| `grants` | akses **group** ke database dengan level `view` (View Only) atau `edit` (View + Edit) |
| `sessions` | session di backend: hash token, idle dan absolute expiration, rotasi, `force_rotate`, grace period, CSRF, `revoked_at` |
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
| Interval rotasi session | 15 menit | Memperkecil jendela pemakaian token yang dicuri. Kenaikan privilege (grant/role) menandai `force_rotate`; token dirotasi pada request berikutnya tanpa logout. |
| Grace period ID lama | 30 detik | Request paralel atau response yang terlambat tidak membuat user terlempar saat cookie baru belum sampai. |
| Atribut cookie | `HttpOnly`, `SameSite=Lax`, `Secure` saat `COOKIE_SECURE=true`. Nama cookie tetap `pathgres_session`, hanya nilainya yang berganti saat rotasi. `APP_ENV=production` tanpa cookie secure membuat startup gagal | `HttpOnly` mencegah JavaScript membaca cookie. `Lax` tetap mengizinkan navigasi biasa ke aplikasi. `Secure` hanya aktif di HTTPS agar development di HTTP tetap jalan; production dipaksa fail-fast agar tidak salah konfigurasi. |
| Mekanisme CSRF | Request terautentikasi: synchronizer token per session (`csrf_secret`), dikirim lewat field form `csrf_token` atau header `X-CSRF-Token` dan dibandingkan dengan `subtle.ConstantTimeCompare`. `POST /login`: pengecekan `Origin` dan `Sec-Fetch-Site` lewat `http.CrossOriginProtection`. `SameSite=Lax` sebagai lapisan tambahan | Login belum punya session sehingga belum ada token. Cek Origin menutup login CSRF tanpa state tambahan. |
| Akses efektif jika user di banyak group | level tertinggi di antara grant group yang aktif | Konsisten dengan `access.Effective`; user tanpa grant ditolak. |
| Pembuatan Platform Admin pertama | Seed dari `ADMIN_USERNAME` dan `ADMIN_PASSWORD` saat start, hanya jika username belum ada | Tidak ada pendaftaran publik dan tidak ada password di repo. Seed yang idempotent aman dijalankan di setiap restart. |
| Aturan username dan password | Username dinormalisasi ke huruf kecil lalu harus 3-32 karakter dari `a-z`, `0-9`, `_`, `.`. Password minimal 8 karakter dan maksimal 72 byte | bcrypt hanya memakai 72 byte pertama, jadi password yang lebih panjang ditolak agar tidak terpotong diam-diam. Normalisasi mencegah `Admin` dan `admin` menjadi dua akun. |

## Status Implementasi

- [x] Skema database sesuai spesifikasi
- [x] Config (termasuk pembacaan `.env`), koneksi database, runner migrasi, server dasar
- [x] Kontrak auth, session, dan access
- [x] Autentikasi: repository user, validasi, seed admin
- [x] Autentikasi: login dan logout
- [x] Session: pembuatan, validasi, rotasi berkala + rotasi paksa saat privilege naik + grace period, revoke, CSRF
- [x] Platform Admin: kelola user, group, koneksi, pemberian akses (grants)
- [x] Database viewer: `/databases`, schema, table (columns/constraints/indexes), data berpaginasi
- [x] Migration: draft, editor, preview + konfirmasi, eksekusi transaksional (commit/rollback), riwayat + filter status
- [x] Halaman 403 (akses ditolak) dan koneksi gagal yang aman tanpa kredensial
- [x] Unit test session/auth/config: idle timeout, absolute expiration, grace period, rotasi berkala/paksa, login/logout, CSRF, atribut cookie, dan aturan production
- [x] Bonus (BON-02/03): schema diff antar dua koneksi + generator kandidat SQL migration
- [x] Bonus (BON-04/05): `EXPLAIN` / `EXPLAIN ANALYZE` sebelum eksekusi + ringkasan execution plan
- [x] Bonus (BON-01): dukungan DBMS tambahan (MySQL/MariaDB): koneksi, viewer, migration,
  EXPLAIN, dan diff bekerja lewat abstraksi dialect per DBMS

Alur minimum (GEN-07) berjalan end-to-end: Login → Access Control → Pilih Database → Lihat Schema/Data → Buat Migration → Execute → Commit/Rollback → Riwayat Migration ter-update.

Route database management:

| Route | Fungsi |
|-------|--------|
| `/databases` | Daftar database yang boleh diakses user (lewat group) |
| `/databases/{id}` | Redirect ke penjelajah schema |
| `/databases/{id}/schema` | Pohon schema/tabel + daftar relasi foreign key |
| `/databases/{id}/tables/{schema.nama}` | Detail tabel: `?tab=columns\|constraints` (kolom / constraint+index) |
| `/databases/{id}/tables/{schema.nama}/data` | Data tabel dengan paginasi |
| `/databases/{id}/migrations` | Riwayat migration + tombol **Migration Baru** dan **Schema Diff** |
| POST `/databases/{id}/migrations` | Simpan SQL sebagai draft (View + Edit) |
| `/databases/{id}/migrations/new` | Editor draft (View + Edit) |
| `/databases/{id}/migrations/{mid}` | Detail migration (status, SQL, pelaksana) |
| `/databases/{id}/migrations/{mid}/preview` | Pratinjau + tombol analisa EXPLAIN (baca) |
| POST `/databases/{id}/migrations/{mid}/explain` | `EXPLAIN` / `EXPLAIN ANALYZE` sebelum eksekusi; ANALYZE dijalankan dalam transaksi lalu di-ROLLBACK (View + Edit) |
| POST `/databases/{id}/migrations/{mid}/execute` | Eksekusi transaksional, commit/rollback (View + Edit) |
| `/databases/{id}/migrations/diff` (alias `/databases/{id}/diff`) | Schema diff vs koneksi lain + kandidat SQL yang bisa dijadikan draft (baca; draft butuh View + Edit) |

Setiap route memeriksa `access.Policy` di backend; penolakan dirender sebagai halaman 403 dengan level akses efektif. Kegagalan koneksi ke database target menampilkan halaman "koneksi gagal" tanpa kredensial. Pool database target di-cache per koneksi (`PGRegistry`) dan ditutup saat koneksi diubah/dihapus atau aplikasi berhenti.

Halaman Platform Admin (route utama: `/` untuk beranda, `/admin` untuk dashboard):

| Route | Fungsi |
|-------|--------|
| `/admin` | Dashboard: ringkasan jumlah user, grup, koneksi + jalan pintas |
| `/admin/users` | Daftar user: status/peran, group, aksi Kelola |
| `/admin/users/new`, `/admin/users/{id}` | Form buat/ubah user: data akun, peran, status akun, reset password, hapus |
| POST `/admin/users/{id}/delete` | Hapus user; ditolak untuk diri sendiri, admin aktif terakhir, dan user dengan riwayat migration |
| `/admin/groups`, `/admin/groups/{id}` | Daftar group; detail: anggota + akses database (mode ubah lewat `?edit=1`) |
| `/admin/groups/new` | Form group baru |
| `/admin/connections`, `/admin/connections/new`, `/admin/connections/{id}`, `/admin/connections/{id}/edit` | Daftar koneksi, form tambah/ubah, uji koneksi, hapus |
| `/admin/access` | Dialihkan ke `/admin/groups` (editor akses berada di detail group) |

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