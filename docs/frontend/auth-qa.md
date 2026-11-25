# Verifikasi Login dan Application Shell

Implementasi lokal 21 November 2026. Tidak melakukan commit/push atau mengubah
database secara manual. Frontend memakai kontrak JSON Go, tanpa mock login,
token sesi di localStorage, atau bypass permission.

## Hasil pemeriksaan

- Build TypeScript/Vite berhasil.
- ESLint berhasil.
- 21 test frontend berhasil: 13 transport/route dan 8 DTO/siklus sesi.
- Gofmt berhasil memformat dan memeriksa sintaks file Go baru.
- Test Go dan preview browser belum terverifikasi: review otomatis lingkungan
  menolak proses karena koneksi telemetry eksternal terdeteksi, termasuk setelah
  opt-out telemetry disetel. Tidak ada hasil lulus backend/E2E yang diklaim.

Test frontend mencakup pemulihan sesi 401 vs kegagalan server, validasi DTO,
pengiriman cookie/CSRF, cache cleanup, respons refresh yang datang setelah logout,
401 lama yang datang setelah login baru, login gagal, dan logout gagal jaringan.
Test Go di `internal/auth/api_test.go` disiapkan untuk perjalanan login → restore
→ penolakan CSRF → logout → penolakan sesi lama, request JSON tidak valid,
akun tidak aktif, rotasi cookie, kegagalan server, dan kegagalan revocation.

## Menjalankan lokal

Gunakan database platform lokal dan `.env` yang sudah dikonfigurasi. Backend
menggunakan migrasi dan seed yang sudah ada saat startup. Gunakan akun aktif
yang memang ada; frontend tidak menyediakan kredensial contoh.

Terminal pertama, dari root repo:

```powershell
go run .
```

Terminal kedua, dari root repo:

```powershell
npm --prefix frontend run dev
```

Buka http://localhost:5173. Jika pemeriksaan sesi menampilkan gagal menghubungi
server, pastikan Go berjalan di port 8080 lalu tekan Coba lagi.

## Skenario manual desktop

1. Buka `/databases` tanpa sesi: menuju Login; submit kosong menampilkan error
   field. Password salah mendapat pesan kredensial salah tanpa membuka Beranda.
2. Toggle eye mengubah visibilitas password dan label aksesibilitas. Fokus field
   memakai border biru dengan ketebalan tetap, mengikuti komponen Input.
3. Login akun aktif: kembali ke route internal tujuan atau `/home`. Tombol
   loading mencegah submit ganda. Refresh memulihkan sesi dari cookie.
4. Di Beranda, Database dan Admin menggunakan navigasi SPA. Admin serta dua
   shortcut administrasi hanya muncul untuk admin platform. Modul tujuan masih
   menampilkan placeholder; bukan data bisnis palsu.
5. Non-admin membuka URL `/admin` langsung: akses terbatas. Kembali ke Beranda
   berfungsi juga ketika halaman dibuka langsung dari tab baru.
6. Avatar menampilkan identitas dari server. Keluar mengirim CSRF, menghapus
   cookie/sesi dan cache privat, lalu kembali ke Login. Back/refresh tetap dijaga.
7. Sesi dicabut atau akun dinonaktifkan: fokus kembali ke tab memeriksa sesi;
   401 mengarah ke Login. Gangguan server menampilkan retry dan menyembunyikan
   konten terlindungi. Logout gagal jaringan menampilkan pesan dan dapat diulang.
8. Keempat route `/design-system/*` tetap publik dan dapat dibuka tanpa backend.

Ulangi pemeriksaan pada viewport 390 px: Login satu kolom, hero/shortcut tersusun
vertikal, bottom navigation tampil dan tidak menutupi konten. Desktop adalah
prioritas tahap ini; perbandingan visual otomatis Figma belum dilakukan.

## Perintah pemeriksaan

```powershell
npm --prefix frontend run build
npm --prefix frontend run lint
npm --prefix frontend test
go test ./internal/auth ./internal/session
```
