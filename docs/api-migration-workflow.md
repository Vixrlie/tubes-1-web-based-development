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
| 422 | `invalid_input` | isian definisi atau alasan penolakan tidak valid |
| 503 | `queue_unavailable` | antrian tidak terpasang |
| 500 | `internal` | galat server; detail hanya di log |

## Status

| Objek | Nilai |
|-------|-------|
| Posisi migration (`state`) | `draft`, `active`, `rolled_back`, `failed` |
| Pengajuan (`status`) | `pending_review` → `queued` → `running` → `success` / `failed`; atau `rejected`, `cancelled` |
| Keputusan group (`decision`) | `pending`, `approved`, `rejected` |
| Eksekusi (`status`) | `running`, `success`, `failed` |
| Aksi (`action`) | `up`, `rollback`, `forward` |

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
  "irreversible_reason": ""               // wajib bila irreversible=true; down_sql harus kosong
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

### Riwayat dan eksekusi

| Method & path | Akses | Respons |
|---------------|-------|---------|
| `GET /api/databases/{connID}/history?hide_rollback=1` | View | `200 {"items": [Execution], "hide_rollback": true}` tanpa `sql_executed` |
| `GET /api/databases/{connID}/executions/{eID}` | View | `200 Execution` lengkap dengan `sql_executed` |

`hide_rollback=1` menyembunyikan pasangan eksekusi: eksekusi yang sudah di-rollback beserta rollback sukses yang membatalkannya. Attempt gagal tetap tampil. Ini hanya filter tampilan; data tidak dihapus.

Field penting `Execution`: `id`, `request_id`, `migration_id`, `migration_name`, `migration_version`, `migration_revision`, `action`, `reverts_execution_id`, `reverted_by`, `status`, `error_stage` (`revalidation`: definisi/approval/urutan berubah sebelum jalan, `execution`: SQL gagal di target, `platform`: worker berhenti di tengah attempt), `error`, `sqlstate`, `attempt`, `executed_by_name`, `started_at`, `finished_at`, `duration_ms`, `rows_affected`.
