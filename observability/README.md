# Pathgres Observability

Jalankan dari root repo pada PowerShell dengan Docker Desktop aktif. Siapkan
database platform seperti pada README utama dan gunakan `APP_ADDR=:8080`.
Jika port aplikasi berbeda, sesuaikan target di `prometheus.yml`.

```powershell
docker compose -f observability/compose.yml up -d
$env:OBSERVABILITY_METRICS = '1'
$env:OTEL_EXPORTER_OTLP_TRACES_ENDPOINT = 'http://localhost:4318/v1/traces'
go run . | Tee-Object -FilePath observability/logs/pathgres.jsonl -Append
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
