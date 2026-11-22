# Database List: FE responsif dan mock data

Implementasi lokal berdasarkan Figma WBD: desktop `337:6386`, mobile `583:2066`.
Tidak ada perubahan backend, database, session guard, commit atau push.

## Membuka halaman

Jalankan aplikasi seperti biasa, login dengan akun aktif, lalu buka `/databases`.
Login tetap membutuhkan backend asli. Daftar dan status koneksi di halaman ini
adalah fixture FE; tidak mengirim request `/api/databases` atau koneksi target.

- `shop_db`, `inventory_db`, `analytics_db`: Terhubung, View + Edit.
- `hr_db`: Gagal terhubung, View Only, pesan error soft red dengan teks error-500.
- Buka berpindah lewat React Router ke `/databases/mock-…/schema`; tujuan masih
  placeholder. Tidak ada implementasi schema/table yang diklaim selesai.
- Coba lagi menjalankan loading lokal lalu kembali ke state gagal. Tidak
  memalsukan koneksi berhasil atau mengubah database target.
- Keterangan teknis mock hanya dicatat di dokumentasi; halaman tidak menampilkan
  label data contoh sesuai arahan desain. Data tetap fixture FE.

Source mock terpisah di `frontend/src/features/databases/mock-source.ts`, model
dan interface `DatabaseSource` di `types.ts`. Kelak ganti adapter, query key dan
hubungkan grant efektif dari server. Jangan menyimpulkan akses dari admin flag.

## State preview development

| URL | Tampilan |
| --- | --- |
| `/databases` | Empat kartu, jeda loading awal singkat |
| `/databases?preview=loading` | Loading yang tetap tampil |
| `/databases?preview=empty` | Belum ada database |
| `/databases?preview=error` | Error daftar dan tombol Coba lagi |

Parameter preview hanya berlaku pada Vite development. Build production tetap
memakai fixture ready; integrasi API belum dilakukan.

## Responsif dan interaksi

CSS dasar mobile; breakpoint min-width 600 (dua kolom), 768 (gaya desktop),
1360 (empat kolom). Ukuran mobile 390 sesuai frame; viewport antara frame
menggunakan layout fluid. Pada 320px tetap satu kolom. Desktop 1728 memakai
padding horizontal 180px dan empat kartu.

- Mobile: header tetap 64px dengan logo proporsional, avatar identitas sesi,
  menu drawer, Kembali ke Beranda, subtitle, kartu satu kolom dan bottom navbar.
- Desktop: header Database aktif, Admin sesuai sesi; heading, empat kartu dan
  gradient primary-50 ke primary-200. Tombol Coba lagi memakai Secondary standar.
- Logo berasal dari aset lokal yang sudah ada. Ikon database desktop connected/
  disconnected 26px serta mobile 16.9px diunduh dari respons Figma tanpa digambar
  ulang atau diregangkan. Foto contoh Figma diganti identitas sesi yang dinamis.
- Drawer memakai Modal/dialog yang sudah ada: fokus terjaga, Escape/close dan
  overlay menutup drawer, navigasi memilih tujuan lewat router.
- Request contoh dibatalkan saat unmount; fixture tidak bermutasi antar query.

## Pemeriksaan

Build dan lint lulus. Total 25 test frontend lulus, termasuk empat test source
database: metadata/isolasi fixture, retry yang tetap gagal, AbortSignal, empty/error.
Pemeriksaan visual browser otomatis dicoba, tetapi Chromium lokal gagal startup;
perbandingan screenshot dan geometri runtime belum terverifikasi.

Cek manual pada 320, 390, 768, 1440 dan 1728px: tidak ada scroll horizontal,
logo/ikon tidak terdistorsi, drawer bisa ditutup, Buka tidak reload, Coba lagi
menampilkan loading, seluruh kartu dapat digulir dan tidak tertutup bottom nav.

```powershell
npm --prefix frontend run build
npm --prefix frontend run lint
npm --prefix frontend test
```
