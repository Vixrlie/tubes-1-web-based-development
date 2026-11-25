# Database dan Beranda — checkpoint FE

Lingkup checkpoint: Beranda (`/home`) dan seluruh alur Database dengan data
bisnis mock. Login/sesi tetap memakai backend Go. Halaman Admin dan integrasi
JSON API bisnis belum termasuk checkpoint ini.

## Acuan visual

- [Beranda desktop](https://www.figma.com/design/lylECEm2d95UMZ0R8djvPk/WBD?node-id=339-8244).
- [Beranda mobile](https://www.figma.com/design/lylECEm2d95UMZ0R8djvPk/WBD?node-id=583-2036).
- [Menu database aktif mobile](https://www.figma.com/design/lylECEm2d95UMZ0R8djvPk/WBD?node-id=589-5947).
- [Pilihan database mobile](https://www.figma.com/design/lylECEm2d95UMZ0R8djvPk/WBD?node-id=911-7848).
- [Menu Admin mobile](https://www.figma.com/design/lylECEm2d95UMZ0R8djvPk/WBD?node-id=911-7815).

Menu mobile memakai panel putih selebar viewport di bawah header, heading dan
deskripsi singkat, divider, serta baris tautan dengan chevron kanan. Tidak lagi
menggabungkan navigasi utama dengan sidebar desktop. Navigasi utama tetap pada
navbar bawah. Tombol tiga garis di kanan mengikuti instruksi pengguna.

`MobileContextMenu` menggunakan dialog native: Escape/klik area luar menutup,
fokus kembali ke pemicu, fokus keyboard terkurung saat panel terbuka. Tombol
Tutup tersedia untuk pembaca layar dan terlihat ketika mendapat fokus keyboard.
Panel panjang memiliki batas tinggi agar semua item bisa dijangkau; sidebar
desktop tetap diam dan konten workspace yang scroll.

## Alur yang tersedia

| Area | Alur FE |
| --- | --- |
| Beranda | Hero responsif, ilustrasi schema, CTA database, quick actions sesuai peran |
| Database | Daftar koneksi, loading/empty/error, koneksi gagal dan percobaan ulang |
| Explorer | Schema, tabel, foreign key, tiga tab detail tabel, pagination |
| Migrasi | History/filter, form SQL Up/Down, irreversible, draft/detail/edit |
| Review | Pengajuan, SQL preview, alasan approve/reject, tiga grup, revisi |
| Eksekusi | Antrean, progress, hasil/history, detail gagal sebagian |
| Tools | Schema Diff ke draft, EXPLAIN/cost/rows dari draft atau aksi cepat |
| Recovery | Rollback/forward, lanjut checkpoint, kompensasi dengan review baru |

## Pemeriksaan lokal

```powershell
npm --prefix frontend run dev
npm --prefix frontend run build
npm --prefix frontend run lint
npm --prefix frontend test
```

Build/lint dan 84 tes model/adaptor lulus pada checkpoint ini. Tes tersebut
mencakup isolasi akun/database, validasi, status review, antrean, rollback,
recovery, diff, EXPLAIN, dan sesi. Pemeriksaan visual/interaksi di browser
belum dapat diotomatisasi dalam sesi ini; checklist berikut perlu dicek pada
320, 390, 768, 1024 dan 1440 px:

1. `/home`: mobile berurutan judul → deskripsi → ilustrasi → CTA. Desktop
   menempatkan ilustrasi di kanan. Akun non-admin tidak melihat tautan Admin.
2. `/databases`: buka tiga garis di kanan. Panel berjudul “Pilih database”,
   memuat empat koneksi; `hr_db` mengarah ke pesan gagal koneksi, bukan schema palsu.
3. `/databases/mock-shop/schema`: menu menampilkan `shop_db`, Schema public,
   products/users/orders dan Migrasi. Tautan menutup panel dan pindah route SPA.
   Pindah inventory/analytics harus mengganti nama tabel sesuai database tersebut.
4. Uji Escape, klik area luar, Tab/Shift+Tab, pemulihan fokus, resize ke desktop,
   dan panel panjang. Popup profil tidak boleh melewati tepi viewport.
5. Buat draft reversible → refresh → edit → ajukan review → approve ketiga
   grup beserta alasan → antrekan → tunggu selesai → buka history.
6. Ulangi dengan reject/revisi, irreversible, serta `?preview=readonly`.
   Recovery memakai `?jobOutcome=failed` (42 batch committed) atau
   `?jobOutcome=error` (belum ada batch committed).
7. Buka Schema Diff, pilih pembanding/perubahan dan simpan draft. EXPLAIN
   tersedia dari Aksi Cepat tanpa harus membuat draft dahulu.
8. Periksa `?preview=loading`, `empty`, `error`, `noschema`, `multiple`, `denied`
   pada explorer; parameter tools khusus dijelaskan pada QA masing-masing fitur.

## Batas penyelesaian

SQL, perubahan schema, biaya/rencana EXPLAIN dan progress masih simulasi lokal.
Draft disimpan per akun/database di sessionStorage dan dibersihkan saat logout.
Permission mock tidak menggantikan otorisasi backend. Integrasi endpoint, data
nyata, pengujian E2E browser dan perbandingan visual akhir masih diperlukan
sebelum menyebut aplikasi siap produksi. Activity Logs milik pekerjaan terpisah;
halaman bisnis Admin tetap menjadi tahap selanjutnya.
