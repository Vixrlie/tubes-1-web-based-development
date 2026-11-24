# API Workflow Migration

Kontrak JSON untuk frontend SPA. Semua route berada di bawah `/api`, memakai cookie sesi `pathgres_session`, dan diperiksa izinnya di backend dengan `access.Policy` yang sama dengan halaman server-rendered.

## Autentikasi dan CSRF

- Tanpa sesi yang valid, setiap `/api/*` mengembalikan `401 {"error": "...", "code": "unauthenticated"}`. SPA mengarahkan user ke `/login`.
- `GET /api/me` → `{"id", "username", "is_admin", "csrf_token"}`.
- Setiap `POST`/`PUT` wajib membawa header `X-CSRF-Token: <csrf_token>`. Token berganti saat sesi dirotasi; ambil ulang `/api/me` bila mendapat `403` dari CSRF.
- Body request berupa JSON, maksimal 1 MiB, dan field yang tidak dikenal ditolak (`400 bad_json`).

## Format galat

```json
{ "error": "pesan untuk ditampilkan", "code": "kode_stabil" }
```

| HTTP | `code` | Arti |
|------|--------|------|
| 400 | `bad_json` | body JSON tidak valid atau berisi field tak dikenal |
| 401 | `unauthenticated` | sesi tidak ada atau kedaluwarsa |
| 403 | `forbidden` | level akses tidak cukup |
| 403 | `not_reviewer` | user bukan leader group yang masih menunggu keputusan |
| 404 | `not_found` | ID di path tidak valid, atau migration/pengajuan/eksekusi/database tidak ada |
| 409 | `validation_required` | up pertama butuh validasi yang lolos untuk revisi terbaru |
| 409 | `irreversible` | rollback pada migration irreversible |
| 409 | `rollback_not_allowed` | migration belum aktif, atau bukan eksekusi aktif terakhir |
| 409 | `forward_not_allowed` | migration masih aktif, atau bukan yang terakhir di-rollback |
| 409 | `immutable` | definisi sudah pernah dieksekusi |
| 409 | `request_open` | migration sedang punya pengajuan terbuka |
| 409 | `invalid_state` | pengajuan tidak lagi menunggu review / tidak bisa dibatalkan |
| 409 | `version_conflict` | gagal mendapat versi unik setelah beberapa percobaan |
| 409 | `partial_pending` | ada migration partial yang harus di-resume atau di-rollback dulu, atau `resume` pada migration yang tidak partial |
| 409 | `database_held` | database ditahan oleh migration partial |
| 422 | `invalid_input` | isian definisi, konfigurasi batch, atau alasan penolakan tidak valid |
| 503 | `queue_unavailable` | antrian tidak terpasang |
| 503 | `unreachable` | database target tidak dapat dihubungi |
| 500 | `internal` | galat server; detail hanya di log |

## Status

| Objek | Nilai |
|-------|-------|
| Posisi migration (`state`) | `draft`, `active`, `rolled_back`, `failed`, `partial` |
| Pengajuan (`status`) | `pending_review` → `queued` → `running` → `success` / `failed` / `partial`; atau `rejected`, `cancelled` |
| Keputusan group (`decision`) | `pending`, `approved`, `rejected` |
| Eksekusi (`status`) | `running`, `success`, `failed`, `partial` |
| Aksi (`action`) | `up`, `rollback`, `forward`, `resume` |

`approved` hanya sesaat: approval terakhir langsung memasukkan job ke antrian dalam transaksi yang sama sehingga status yang terlihat adalah `queued`.

## Endpoint

### Definisi migration

| Method & path | Akses | Body | Respons |
|---------------|-------|------|---------|
| `GET /api/databases/{connID}/migrations` | View | | `200 {"items": [Migration]}` tanpa `up_sql`/`down_sql` |
| `POST /api/databases/{connID}/migrations` | Edit | `MigrationInput` | `201 Migration` |
| `GET /api/databases/{connID}/migrations/{mID}` | View | | `200 {"migration", "requests", "executions", "allowed_actions", "can_edit"}` |
| `PUT /api/databases/{connID}/migrations/{mID}` | Edit | `MigrationInput` | `200 Migration` (revisi +1, validasi lama terhapus) |
| `POST /api/databases/{connID}/migrations/{mID}/validate` | Edit | `{}` | `200 {"ok", "message", "phase", "sqlstate", "dry_run", "duration_ms"}` |

```json
// MigrationInput
{
  "name": "Tambah kolom status",          // opsional; diambil dari komentar pertama SQL up
  "up_sql": "ALTER TABLE products ADD COLUMN status TEXT",
  "down_sql": "ALTER TABLE products DROP COLUMN status",
  "irreversible": false,
  "irreversible_reason": "",              // wajib bila irreversible=true; down_sql harus kosong
  "batch": {                              // opsional; lihat "Migration per batch"
    "table": "public.orders", "column": "id", "size": 50000
  }
}
```

`allowed_actions` berisi aksi yang sah saat ini, misalnya `["validate", "up"]` atau `["validate", "rollback"]`. Tampilkan tombol sesuai daftar ini, tetapi tetap tangani `409` karena state bisa berubah di antara request.

### Pengajuan dan approval

| Method & path | Akses | Body | Respons |
|---------------|-------|------|---------|
| `POST /api/databases/{connID}/migrations/{mID}/requests` | Edit | `{"action": "up\|rollback\|forward", "note": "..."}` | `201 Request` |
| `GET /api/databases/{connID}/requests?status=pending_review,queued` | View | | `200 {"items": [Request]}` |
| `GET /api/migration-requests/{rID}` | View | | `200 Request` lengkap dengan `reviewers`, `sql`, `executions` |
| `POST /api/migration-requests/{rID}/approve` | leader | `{"note": "..."}` | `200 Request`, atau `202 {"request", "job_id"}` bila ini approval terakhir |
| `POST /api/migration-requests/{rID}/reject` | leader | `{"note": "alasan"}` (wajib) | `200 Request` |
| `POST /api/migration-requests/{rID}/cancel` | pengaju | `{}` | `200 Request` |
| `GET /api/approvals` | login | | `200 {"items": [Request]}` yang menunggu keputusan user |

Reviewer dihitung dari grant saat ini: setiap group yang punya akses (view atau edit) ke database wajib disetujui oleh leader-nya. Satu leader yang memimpin beberapa group memutuskan untuk semua group tersebut sekaligus. Satu penolakan langsung menolak pengajuan.

Approve tidak menunggu SQL selesai. Untuk memantau progres, lakukan polling `GET /api/migration-requests/{rID}` (misalnya tiap 1–2 detik) sampai `status` menjadi `success` atau `failed`.

### Migration per batch

Untuk dataset besar, isi `batch` pada `MigrationInput`. SQL up (dan down bila reversible) wajib memakai `$1` dan `$2` sebagai batas rentang kursor, misalnya `UPDATE orders SET status = 'done' WHERE id BETWEEN $1 AND $2`. Konfigurasi batch ikut checksum dan ikut terkunci setelah eksekusi.

- Worker mengunci rentang kursor di awal, lalu menjalankan satu transaksi per batch bersama checkpoint di tabel target `_pathgres_batch_checkpoints`. Tabel internal ini tidak tampil di schema maupun schema diff.
- Progres terlihat di `Execution`: `batch_size`, `committed_batches`, `committed_rows`, `total_batches`, `cursor_last`; detail eksekusi juga memuat `checkpoints`.
- Bila gagal setelah sebagian batch tersimpan, eksekusi dan pengajuan berstatus `partial`. Database ditahan: migration lain ditolak (`database_held`) sampai partial diselesaikan dengan `resume` (lanjut dari batch berikutnya) atau `rollback` (membalik batch yang sudah tersimpan, urutan terbalik). Kedua aksi itu juga butuh approval.
- Worker yang berhenti di tengah attempt melanjutkan dari checkpoint terakhir pada attempt berikutnya.

### Board workflow

| Method & path | Akses | Respons |
|---------------|-------|---------|
| `GET /api/databases/{connID}/workflow` | View | `200 {"migrations", "requests", "leader_group_ids", "can_edit"}` |

Satu respons untuk halaman draft, review, antrean, dan riwayat SPA. `migrations` memuat SQL; `requests` (terbaru dulu) memuat `reviewers` (dengan `decided_at`) dan `executions`; `leader_group_ids` adalah group yang dipimpin user.

### Riwayat dan eksekusi

| Method & path | Akses | Respons |
|---------------|-------|---------|
| `GET /api/databases/{connID}/history?hide_rollback=1` | View | `200 {"items": [Execution], "hide_rollback": true}` tanpa `sql_executed` |
| `GET /api/databases/{connID}/executions/{eID}` | View | `200 Execution` lengkap dengan `sql_executed` |

`hide_rollback=1` menyembunyikan pasangan eksekusi: eksekusi yang sudah di-rollback beserta rollback sukses yang membatalkannya. Attempt gagal tetap tampil. Ini hanya filter tampilan; data tidak dihapus.

Field penting `Execution`: `id`, `request_id`, `migration_id`, `migration_name`, `migration_version`, `migration_revision`, `action`, `reverts_execution_id`, `reverted_by`, `status`, `error_stage` (`revalidation`: definisi/approval/urutan berubah sebelum jalan, `execution`: SQL gagal di target, `platform`: worker berhenti di tengah attempt), `error`, `sqlstate`, `attempt`, `executed_by_name`, `started_at`, `finished_at`, `duration_ms`, `rows_affected`.

## Database (SPA)

| Method & path | Akses | Respons |
|---------------|-------|---------|
| `GET /api/databases` | login | `200 {"items": [Database]}` hanya database yang boleh dilihat user |
| `GET /api/databases/{connID}` | View | `200 Database` |
| `POST /api/databases/{connID}/reconnect` | View | `200 Database`, atau `503 unreachable` |
| `GET /api/databases/{connID}/schema` | View | `200 {"database", "schemas": [{"name", "tables", "relations"}]}` |
| `GET /api/databases/{connID}/tables/{schema}/{table}` | View | `200 {"schema", "name", "columns", "constraints", "indexes", "permission"}` |
| `GET /api/databases/{connID}/tables/{schema}/{table}/rows?page=1&page_size=25&direction=asc` | View | `200 {"columns", "rows", "total", "page", "page_size", "pages"}` |
| `GET /api/databases/{connID}/snapshot` | View | `200 {"id", "name", "tables": [struktur tabel]}` untuk schema diff |
| `POST /api/databases/{connID}/explain` | View; Edit bila `analyze` | body `{"sql", "analyze"}`, respons `200 {"items": [ExplainResult]}` |

- `Database`: `id`, `name`, `engine`, `status` (`connected`/`disconnected`), `permission` (`view`/`edit`), `schema_count`, `table_count`.
- Data tabel diurutkan menurut primary key; `page_size` maksimal 100 dan `page` di luar rentang dikembalikan ke halaman terakhir. `NULL` dikirim sebagai `null`; bilangan di luar rentang aman JavaScript dikirim sebagai string.
- Nama schema dan tabel divalidasi terhadap `information_schema` sebelum dipakai di query; tabel yang tidak ada menghasilkan `404`.
- `ExplainResult`: `n`, `statement`, `kind`, `note`, `plan` (pohon teks), `root` (`node`, `relation`, `startup_cost`, `total_cost`, `rows`, `width`; `null` untuk DDL), `planning_ms`, `execution_ms`. `EXPLAIN ANALYZE` benar-benar menjalankan statement dalam transaksi yang selalu di-rollback.

## Leader group (admin)

| Method & path | Akses | Body | Respons |
|---------------|-------|------|---------|
| `GET /api/admin/groups/{id}` | Platform Admin | | detail group, termasuk `leaderId` (`null` bila kosong) |
| `PUT /api/admin/groups/{id}/leader` | Platform Admin | `{"userId": "5"}` atau `{"userId": null}` | `200 {"leaderId"}`; `422 LEADER_NOT_MEMBER` bila bukan anggota aktif |

API admin memakai format galat `{"error": {"code", "message"}}`.
