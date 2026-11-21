# Pathgres: mapping UI, API, dan akses

Audit source lokal branch `feat/spa`, baseline `3d1aaf6`, 21 November 2026.
**Endpoint yang diusulkan dalam dokumen ini belum dibuat atau disetujui BE.**
Tidak ada perubahan backend pada tahap ini; audit bukan test live endpoint.

## Temuan yang terverifikasi dari source

- Handler bisnis saat ini melayani HTML, form, redirect 303, atau fragmen HTML.
  Prefix `/api` belum terdaftar pada routing bisnis yang diperiksa.
- Vite meneruskan `/api/...` tanpa rewrite ke port 8080. Proxy tidak berarti JSON
  API sudah siap; fetch ke prefix tersebut belum boleh diaktifkan sebagai integrasi.
- Cookie `pathgres_session`: HttpOnly, Path=/, SameSite=Lax, Secure sesuai config.
- Mutasi terautentikasi memerlukan `X-CSRF-Token` atau form `csrf_token`.
  Login lama memakai pemeriksaan same-origin CrossOriginProtection.
- Middleware tanpa sesi menghasilkan redirect `/login` atau HX-Redirect untuk HTMX.
  SPA memerlukan 401 JSON agar tidak menganggap halaman Login sebagai data sukses.
- `/admin/*` memerlukan `IsPlatformAdmin`. Admin platform **tidak otomatis**
  memperoleh akses database target. Grant grup menentukan izin per koneksi.
- Level efektif adalah tertinggi dari grup: `view < edit`. Tanpa grant ditolak.

Sumber: `main.go`, `internal/auth/{handlers,middleware}.go`,
`internal/access/policy.go`, `internal/dbm/handlers.go`, `internal/admin/handlers.go`.

## Matriks layar / route / data / akses

Semua route produk yang diregistrasikan pada SPA tahap ini masih **placeholder**.
Aturan akses di tabel adalah target integrasi, bukan route guard yang sudah aktif.

| Layar | Route SPA | Kebutuhan data/aksi | Akses target | BE saat ini |
| --- | --- | --- | --- | --- |
| Login | `/login` | username/password, sesi | Publik + same-origin | GET/POST `/login`, HTML/form |
| Beranda | `/home` | identitas, flag admin, shortcut | Sesi aktif | GET `/`, HTML |
| Database list | `/databases` | koneksi + effective access | Sesi, grant view | GET `/databases/` |
| Ringkasan | `/databases/:connectionId` | nama, DBMS, jumlah schema/tabel | view | GET `/databases/{connID}` |
| Schema Explorer | `/databases/:connectionId/schema` | schema dan tabel | view | GET `D/schema` |
| Detail tabel | `/databases/:connectionId/schemas/:schemaName/tables/:tableName` | columns, constraints, data | view | GET `D/tables/{schema.table}` |
| Data/pagination | detail + `?tab=data&page=1` | columns dinamis, rows, total | view; viewer read only | detail `?tab=data&page=...` atau `/data?page=...`; page size 20 |
| Migrasi | `/databases/:connectionId/migrations` | histori/state | view | GET `D/migrations` |
| Buat draft | migrations + `/new` | SQL | edit | GET editor, POST koleksi, form `sql` |
| Detail/preview | migrations + `/:migrationId[/preview]` | SQL/checksum/pembuat/hasil | view | GET detail/preview |
| EXPLAIN | aksi preview | plan, analyze opsional | view; analyze perlu edit | POST `M/explain`, form `analyze` |
| Eksekusi | aksi preview | hasil/error | edit + CSRF + pemeriksaan state server | POST `M/execute` |
| Schema Diff | migrations + `/diff` | ref connID, diff, SQL | view kedua koneksi; edit untuk draft | GET/POST diff, form `ref` |
| Admin dashboard | `/admin` | statistik | Platform admin | GET `/admin/` |
| Users | `/admin/users`, `/new`, `/:userId` | daftar/form/aksi akun | Platform admin | HTML/form |
| Groups | `/admin/groups`, `/new`, `/:groupId` | anggota dan grant | Platform admin | HTML/form |
| Connections | `/admin/connections`, `/new`, `/:connectionId[/edit]` | CRUD + test koneksi | Platform admin | HTML/form |
| Review/approval | Belum diregistrasikan | reviewer, threshold, transisi | Belum ada kontrak | Route tidak ditemukan pada handler yang diaudit |
| Queue/batch/rollback manual | Belum diregistrasikan | job/progres/cancel/retry/SQL Down | Belum ada kontrak | Route tidak ditemukan pada handler yang diaudit |

`D = /databases/{connID}`, `M = D/migrations/{mID}`.
`shop_db` di katalog adalah nama contoh, bukan ID koneksi server. Route SPA tabel
memisahkan schema/table; backend lama memakai parser `schema.table` yang menolak
titik tambahan pada nama table. Adapter JSON perlu menyepakati identifier SQL.

## Inventaris route BE yang benar-benar terdaftar

Output di bawah **bukan JSON API**. Seluruh route database/admin membutuhkan sesi;
semua POST terproteksi membutuhkan CSRF, kecuali login memakai same-origin check.

| Modul | Metode/path |
| --- | --- |
| Auth | GET/POST `/login`; GET `/`; POST `/logout` |
| Database | GET `/databases/`, `D`, `D/schema`, `D/tables/{schema.table}`, `D/tables/{schema.table}/data` |
| Migrasi | GET/POST `D/migrations`; GET `D/migrations/new`; GET `M`; GET `M/preview`; POST `M/explain`; POST `M/execute` |
| Diff | GET/POST `D/migrations/diff`; GET/POST alias `D/diff` |
| Admin dashboard | GET `/admin/` |
| Users | GET `/admin/users`; GET `/admin/users/new`; POST `/admin/users`; GET/POST `/admin/users/{id}` |
| User actions | POST `/admin/users/{id}/activate`, `/deactivate`, `/password`, `/delete` |
| Groups | GET `/admin/groups`; GET `/admin/groups/new`; POST `/admin/groups`; GET `/admin/groups/{id}`; POST `/admin/groups/{id}/delete` |
| Membership | POST `/admin/groups/{id}/members`; POST `/admin/groups/{id}/members/{userID}/delete` |
| Grants | POST `/admin/groups/{id}/access` |
| Connections | GET `/admin/connections`; GET `/admin/connections/new`; POST `/admin/connections`; GET/POST `/admin/connections/{id}`; GET `/admin/connections/{id}/edit` |
| Connection actions | POST `/admin/connections/test`; POST `/admin/connections/{id}/test`; POST `/admin/connections/{id}/delete` |
| Access alias | GET `/admin/access` redirect `/admin/groups` |

Sumber: `internal/dbm/{handlers,migration_pages,diff_pages}.go`,
`internal/admin/{dashboard,users_handler,groups_handler,connections_handler}.go`.
Connection admin normal hanya dipasang saat `DEV_COMPONENTS != 1`; route demo
mode dev bukan bukti fitur/API production tersedia.

## Gap Figma/katalog terhadap backend

| Kebutuhan | Fakta source | Keputusan berikutnya |
| --- | --- | --- |
| Identitas Login | Form menerima `username`, bukan email | Cocokkan label Figma dengan kontrak final |
| SQL Up/Down | Model Migration memiliki satu `SQL` | Desain payload/skema Up/Down dengan BE |
| Review, approval, queue | Status model hanya `draft`, `applied`, `failed` | Sepakati state machine, reviewer dan endpoint |
| Batch progress | Eksekusi sekarang satu transaksi | Perlu kontrak job; jangan membuat job/progres palsu dari respons sinkron |
| Rollback manual | Rollback transaksi gagal ada, route rollback manual tidak ditemukan | Pisahkan kegagalan transaksi dan menjalankan SQL Down |
| View + Edit | Grant `edit` sudah mencakup view | Label UI dipetakan dari enum backend |
| Search/filter | Contoh katalog masih lokal | Sepakati pagination/sort/filter server |
| Activity Logs | Logging tidak otomatis menyediakan API daftar log | Tentukan scope dan endpoint layar log |

Sumber state: `internal/dbm/migration.go`, `migration_pages.go`; izin:
`internal/access/policy.go`. EXPLAIN ANALYZE di-rollback, bukan fitur rollback
migrasi sukses yang bisa dipanggil pengguna.

## Kontrak JSON usulan: belum tersedia

Tambahkan namespace `/api` menggunakan service dan policy yang sama; pertahankan
HTML lama saat transisi. Jangan scrape HTML atau mematikan CSRF agar SPA berjalan.

| Prioritas | Endpoint usulan | Kebutuhan |
| --- | --- | --- |
| P0 | POST `/api/auth/login` | username/password JSON, cookie, user + CSRF terbaru |
| P0 | GET `/api/auth/session` | identitas, isPlatformAdmin, csrfToken; 401 JSON jika tidak ada sesi |
| P0 | POST `/api/auth/logout` | revoke, clear cookie, 204; revoke gagal bukan sukses palsu |
| P0 | GET `/api/databases` | koneksi yang diizinkan dan effective access |
| P0 | GET `/api/databases/{id}` | ringkasan tanpa kredensial target |
| P0 | GET `/api/databases/{id}/schemas` | schema dan tabel |
| P0 | GET `/api/databases/{id}/schemas/{schema}/tables/{table}` | metadata columns/constraints |
| P0 | GET path tabel + `/rows?page=1` | columns, rows, page, pageSize, total |
| P1 | GET/POST `/api/databases/{id}/migrations` | histori / draft |
| P1 | GET `/api/databases/{id}/migrations/{migrationId}` | detail dan state asli backend |
| P1 | POST detail + `/explain`, `/execute` | plan/eksekusi, otorisasi/state tetap server |
| P1 | POST `/api/databases/{id}/schema-diff` | ref connection ID; view kedua koneksi |
| P1 | GET `/api/admin/dashboard` | statistik |
| P1 | `/api/admin/users`, `/groups`, `/connections` + detail/aksi | JSON adapter; metode update/delete perlu disepakati |
| P2 | Review, approval, jobs, batch, SQL Down, rollback | Kontrak baru setelah aturan BE jelas |

### Payload dan error usulan

- ID int64 PostgreSQL menjadi string untuk menghindari kehilangan presisi JS.
- JSON camelCase; timestamp ISO 8601; nilai SQL null dipertahankan.
- Respons sukses DTO langsung. Client tidak otomatis membongkar `{data: ...}`.
- Daftar: `{items, page, pageSize, total}`; rows tabel memerlukan columns serta
  representasi angka besar/date/binary yang disepakati.
- Backend mengirim izin/capability efektif; FE tidak menebak dari role saja.
- Error: 401 sesi, 403 izin/CSRF, 404 resource, 409 state/conflict,
  422 validasi, 500/503 kegagalan server/koneksi.

Contoh respons sesi **usulan**:

```json
{
  "user": { "id": "12", "username": "arina", "isPlatformAdmin": false },
  "csrfToken": "token-dari-server"
}
```

Contoh error **usulan**:

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Nama wajib diisi.",
    "fields": { "name": "Wajib diisi." }
  }
}
```

Cookie sesi tidak disalin ke JSON. CSRF disimpan di memori session provider kelak,
diperbarui setelah login/bootstrap sesi. Login JSON tetap memakai same-origin
check; pengecualian CSRF-nya perlu disetujui BE. Jangan gunakan `csrf: 'omit'`
pada mutasi terlindungi. Tipe generik API client bukan validator runtime DTO.

## Batas tahap ini dan checkpoint berikutnya

Sudah: router/layout skeleton, query provider, transport JSON, katalog terpisah,
serta mapping ini. Belum: session provider, route guard, halaman bisnis final,
JSON API Go. Placeholder tidak memuat data privat. Root tetap membuka katalog.

Checkpoint berikutnya: setujui P0 dan payload/status dengan BE; buat JSON adapter;
implementasi Login, sesi, guard dan pembersihan cache saat logout/401; kemudian
selesaikan satu alur database desktop menggunakan data nyata.
