# Admin — integrasi Activity Logs dan Observability

`/admin` langsung mengarahkan ke Activity Logs. Tidak ada dashboard tambahan.
`/admin/activity-logs` dan `/admin/observability` berada dalam SessionGate dan
AdminGate. Background Admin dan Database memakai token gradasi yang sama.
CRUD Pengguna/Grup/Koneksi tetap di luar pekerjaan tahap ini.

## Kontrak live

| Endpoint | Respons/akses |
| --- | --- |
| GET `/api/admin/activity-logs` | items, total, page, pageSize, limited, from, to; admin platform |
| GET `/api/admin/activity-logs/{eventID}` | satu event; admin platform |

Kedua endpoint mengembalikan JSON no-store, 401 untuk sesi hilang/invalid/inaktif,
403 untuk non-admin, 503 untuk kegagalan sesi atau Loki, dan 404 untuk detail yang
tidak tersedia. Tidak memakai middleware HTML/redirect. Rotasi cookie mengikuti
validator sesi yang sama dengan auth. Hanya operasi GET, tanpa mutasi log.

Query daftar: search (maksimum 200 karakter), status, category (sql/http/user/admin),
from/to (YYYY-MM-DD WIB, kedua tanggal inklusif), page (mulai 1), pageSize (1–100).
Default 7 hari terakhir; maksimal 30 hari. Query LogQL dibuat server dengan
literal search yang di-escape, tidak menerima LogQL arbitrer dari browser.

Backend membaca maksimal 5.000 event terbaru pada rentang/filter tersebut lalu
mengurutkan dan melakukan pagination. `limited=true` memberi tahu FE bahwa hasil
mencapai batas dan filter perlu dipersempit. Total mengacu pada jendela hasil,
bukan seluruh histori. Tidak ada snapshot antar-request; log baru dapat menggeser
posisi halaman. ID detail stabil memakai timestamp nanodetik + hash baris.

DTO mengikuti field log yang tersedia: time/action/status/userId/requestId,
traceId/spanId/database/migrationId opsional dan detail operasi. API hanya
mengirim allowlist field; raw SQL, nilai parameter, credentials dan pesan error
mentah tidak dikirim. SQL hanya menampilkan nama operasi yang diizinkan.

## Jalur data dan batas

Go activity logger → file UTF-8 `observability/logs/activity.jsonl` → filelog
collector → Loki → API Go khusus admin → FE. API dan Grafana membaca Loki yang
sama. Collector memakai timestamp JSON asli. Log tidak dimasukkan ke git.

`ActivityLogsPage` memakai `session.api`, query cache dipisahkan per akun/filter/
halaman, dan 401 otomatis mengakhiri sesi. Modal detail memakai endpoint sendiri,
tidak bergantung pada baris halaman/filter aktif. Source mock tetap hanya sebagai
fixture unit test, tidak menjadi fallback dan tidak ada parameter preview live.

Observability tetap mengarah ke dashboard UID `pathgres-observability` serta
Explore Loki/Tempo. Tidak memalsukan angka atau health status di SPA.

Cara menjalankan stack dan konfigurasi environment terdapat di
[petunjuk observability](../../observability/README.md).

## Verifikasi

FE: build, lint dan 94 tes lulus. Tes adaptor mencakup URL/query/cookie transport,
DTO, detail independen, error 401/403/503, abort, tanpa fallback mock.

Go: ditambahkan httptest untuk otorisasi dan validasi filter, kontrak Loki,
redaksi, ID detail stabil dan penolakan redirect upstream. Build/test Go belum
terverifikasi: auto-review menolak build karena telemetry eksternal yang belum
terverifikasi. Stack live juga belum diuji karena akses Docker daemon ditolak.
Konfigurasi Compose lolos `docker compose ... config --quiet`.

Checklist lokal setelah restart backend/collector:

1. Login admin → Activity Logs → Muat ulang. Cari `user.login`/request ID nyata.
2. Filter tanggal/status/kategori; detail menunjukkan metadata operasi tersamarkan.
3. Buka detail, ubah filter/halaman dan refresh URL; event tetap dapat dibuka.
4. Logout/expire → 401 mengarahkan Login. Non-admin → 403, tanpa query Loki.
5. Matikan Loki → 503 dan retry, bukan mock/empty sukses. Aktifkan kembali → muat ulang.
6. Buka `/admin/observability` → Grafana; cari request/trace ID yang sama.
7. Uji 320/390/768/1024/1440 px, keyboard, Escape, sidebar diam dan scroll konten.

Pekerjaan FE Database masih memakai simulasi; aksi simulasi tidak menghasilkan
SQL/log backend. Logging setiap fitur anggota lain perlu diuji saat fitur nyata
tersebut diintegrasikan. Metrics queue/worker masih mengikuti batas implementasi
backend yang tercatat di dokumentasi observability.
