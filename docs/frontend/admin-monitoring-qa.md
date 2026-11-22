# Admin — Activity Logs dan Observability

Lingkup pekerjaan Arin: `/admin` langsung mengarahkan ke Activity Logs pada
`/admin/activity-logs`, dan Observability pada `/admin/observability`. Ketiganya
berada di dalam `SessionGate` dan `AdminGate`. Pengguna, Grup, Koneksi Database,
dan pengaturan akses belum diimplementasikan pada tahap ini.

## Activity Logs

Sumber FE adalah `createMockActivitySource`; belum memanggil endpoint bisnis.
Backend `internal/activity` menulis JSON ke stdout, bukan menyediakan daftar log
JSON untuk SPA. Bentuk fixture mengikuti field yang tersedia: time, action,
status, user_id, request_id, trace_id/span_id opsional, database, migration_id,
serta metadata operasi yang telah disamarkan. ID event adalah ID fixture FE;
backend memerlukan ID/cursor stabil saat kontrak API daftar log dibuat.

Kategori yang tersedia: HTTP, SQL, autentikasi (`user.*`) dan administrasi
(`admin.*`). Filter pencarian, kategori, status, tanggal dan halaman tersimpan
di query URL. Tanggal menggunakan WIB termasuk batas hari. `event` menyimpan
ID detail terpilih, sehingga modal dapat dibuka kembali ketika refresh.
Data detail hanya-baca; tidak ada edit/hapus log. Contoh data tidak mengambil
log lokal maupun menampilkan password, token, DSN atau nilai parameter SQL.

Skenario development: `?preview=loading`, `?preview=empty`, `?preview=error`.

## Observability

Halaman SPA adalah pintu masuk Grafana, bukan dashboard metrik simulasi.
Konfigurasi repo yang menjadi acuan:

| Kebutuhan | Akses |
| --- | --- |
| Metrics | Grafana `/d/pathgres-observability`, datasource Prometheus |
| Logs | Grafana Explore, datasource Loki |
| Traces | Grafana Explore, datasource Tempo |
| Scrape | Backend `/metrics` jika `OBSERVABILITY_METRICS=1` |

URL Grafana lokal default pada development adalah `http://localhost:3000`.
Salin `frontend/.env.example` menjadi `frontend/.env.local` untuk mengganti
`VITE_GRAFANA_URL`, lalu restart Vite. Production memerlukan URL eksplisit;
tanpa URL valid tombol eksternal tidak ditampilkan. Base URL mendukung subpath
Grafana dan menolak protokol selain HTTP(S), credentials, query, serta fragment.

Halaman tidak mengklaim Grafana sedang sehat/terhubung. Grafana menggunakan sesi
login tersendiri. Proteksi route SPA tidak melindungi URL Grafana atau endpoint
`/metrics`; akses kedua layanan tersebut mengikuti konfigurasi layanan masing-masing.
Stack, collector, scrape dan tracing dijalankan sesuai
[petunjuk observability](../../observability/README.md).

## Verifikasi

```powershell
npm --prefix frontend run build
npm --prefix frontend run lint
npm --prefix frontend test
```

Tes tambahan memeriksa korelasi pencarian, gabungan filter, batas tanggal WIB,
isolasi fixture, redaksi parameter, abort/error/empty serta URL Grafana.

Checklist browser manual (belum dijalankan otomatis):

1. Login sebagai admin; buka `/admin`. Langsung masuk Activity Logs, tanpa dashboard tambahan.
   Sidebar hanya menyediakan item halaman; tidak ada item Dashboard.
2. Desktop: sidebar menandai item aktif; konten scroll terpisah. Mobile:
   menu tiga garis menampilkan Activity Logs dan Observability, tautan menutup panel.
3. Cari `req-migration-014`; dua event saling terkait. Filter Gagal menyisakan
   `sql.result`. Detail memuat request/trace/migration ID dan SQLSTATE tanpa SQL mentah.
4. Filter 22 November 2026 harus mencakup event 21 November pukul 18.15 UTC
   (22 November pukul 01.15 WIB). Tanggal akhir sebelum awal menampilkan validasi.
5. Pindah halaman tabel, refresh, buka/tutup modal dengan Escape dan keyboard.
   Reset filter mengembalikan semua data; URL event tidak dikenal memberi empty state.
6. Periksa preview loading/empty/error, viewport 320/390/768/1024/1440 px.
7. Non-admin membuka kedua URL: akses ditolak; sesi anonim diarahkan ke Login.
8. Setelah stack berjalan, tombol Metrics membuka dashboard UID yang diprovisikan;
   Explore dapat memakai datasource Loki/Tempo. Log mock tidak dikirim ke Grafana.

Integrasi berikutnya: kontrak daftar/detail log dengan paginasi dan filter server,
otorisasi admin, adaptasi nama field, penanganan 401/403, dan E2E memakai backend.

Background shell Admin dan Database Workspace memakai token gradasi bersama
`--workspace-background-mobile` dan `--workspace-background-desktop`.
