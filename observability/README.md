# Pathgres Observability

Di SPA, admin dapat membuka **Admin → Observability** pada
`http://localhost:5173/admin/observability`, lalu memilih dashboard metrik atau
Grafana Explore untuk log/trace. Dashboard Grafana juga dapat dibuka langsung
di `http://localhost:3000/d/pathgres-observability`.

Halaman **Admin → Activity Logs** (`/admin/activity-logs`) membaca log nyata
melalui API Go → Loki. API hanya dapat diakses admin platform. Menjalankan SPA
tidak otomatis menjalankan stack monitoring. URL Grafana FE dapat diatur melalui
`VITE_GRAFANA_URL` pada `frontend/.env.local` (lihat `frontend/.env.example`).

Jalankan dari root repo pada PowerShell dengan Docker Desktop aktif. Siapkan
database platform seperti pada README utama dan gunakan `APP_ADDR=:8080`.
Jika port aplikasi berbeda, sesuaikan target di `prometheus.yml`.

```powershell
docker compose -f observability/compose.yml up -d
$env:LOKI_URL = 'http://127.0.0.1:3100'
$env:ACTIVITY_LOG_FILE = 'observability/logs/activity.jsonl'
$env:OBSERVABILITY_METRICS = '1'
$env:OTEL_EXPORTER_OTLP_TRACES_ENDPOINT = 'http://localhost:4318/v1/traces'
go run .
```

Di terminal kedua, periksa endpoint Prometheus.

```powershell
Invoke-RestMethod http://localhost:8080/metrics
```

Buka [Grafana](http://localhost:3000) dengan akun lokal bawaan `admin`.
Dashboard **Pathgres Observability** memuat jumlah migration berjalan, selesai,
dan gagal, durasi migration dan lock, serta koneksi platform dan target aktif dan idle
dan waktu tunggu pool. Jalankan migration terhadap database uji untuk melihat
perubahan metrik. Metrik migration dihitung sejak proses Pathgres mulai.

Di Grafana Explore, pilih Loki dan jalankan
`{service_name="pathgres"} | json | action="sql.result"`.
Cocokkan `request_id` dan `migration_id` dengan log SQL di terminal.
Loki menerima log JSON dari Collector. Collector juga menerima span melalui
OTLP di localhost port 4317 (gRPC) atau 4318 (HTTP) dan meneruskannya ke Tempo.

Untuk mencocokkan log dan trace, ambil `trace_id` dari log `sql.result`
yang memiliki `migration_id`, lalu cari ID tersebut di Grafana Explore
dengan datasource Tempo. Untuk migration sinkron, trace mencakup request
HTTP, `migration.execute`, `migration.lock`, dan span SQL. Field
`span_id` pada log menunjukkan span yang mencatat event itu.

Metrik queue, retry, batch dan row serta propagasi trace melalui queue
dan worker masih perlu diintegrasikan saat alur asynchronous tersedia.

## Activity Logs pada SPA

Backend menulis JSONL UTF-8 langsung ke `observability/logs/activity.jsonl` dan
stdout; tidak memerlukan `Tee-Object` atau bergantung pada encoding PowerShell.
Collector membaca file baru ini dan memakai timestamp JSON asli. File lama
`pathgres.jsonl` tetap dibiarkan, tetapi tidak lagi dikumpulkan. Jika mengubah
`ACTIVITY_LOG_FILE`, sesuaikan volume/include collector juga. Log lokal tidak
masuk git; rotasi/retensi file perlu disesuaikan untuk deployment jangka panjang.

Setelah memperbarui konfigurasi, jalankan kembali
`docker compose -f observability/compose.yml up -d` agar port Loki dan konfigurasi
collector terpasang. Jika collector lama masih memakai config lama, jalankan
`docker compose -f observability/compose.yml restart collector`.

1. Jalankan backend dan FE (`npm --prefix frontend run dev`).
2. Login dengan admin platform, lalu buka `/admin/activity-logs`.
3. Cari `user.login` atau kategori Autentikasi, lalu pilih Muat ulang setelah
   collector mengirim batch. Detail memuat request/trace ID jika tersedia.
4. Jika Loki mati, API memberi 503 dan FE menawarkan Coba lagi. Tidak ada
   fallback mock. Jika belum ada log yang terkumpul, respons sukses berisi daftar kosong.

Endpoint: `GET /api/admin/activity-logs` dan
`GET /api/admin/activity-logs/{eventID}`. Request memakai cookie sesi aplikasi;
Grafana tetap memakai sesi login terpisah. Port Loki hanya dipublikasikan ke
loopback `127.0.0.1:3100` untuk backend yang dijalankan di host. Alamat Loki
berasal dari konfigurasi server, tidak pernah dari input browser.

Default daftar adalah 7 hari terakhir, batas rentang 30 hari. Filter dan pagination
berjalan di backend. Maksimum 5.000 hasil terbaru per pencarian; jika mencapai
batas, FE meminta pengguna mempersempit pencarian. Total adalah jumlah dalam
jendela hasil tersebut, bukan hitungan seluruh retensi Loki. Refresh/page request
membaca kondisi terkini sehingga posisi baris dapat bergeser saat log baru masuk.

ID detail berasal dari timestamp nanodetik Loki dan hash isi baris sehingga tetap
stabil untuk baris yang sama. Detail diambil independen dari filter daftar;
setelah retensi/kehilangan data, ID tersebut dapat menghasilkan 404.

Aksi Database pada FE masih mock. Aksi simulasi itu tidak menghasilkan SQL nyata
atau log backend. Gunakan login, request HTTP, atau operasi backend nyata untuk
memverifikasi pekerjaan logging Arin.
