# Schema Explorer: FE dan verifikasi

Acuan final WBD: desktop `337:6409`, mobile `583:2116`, drawer `589:5947`.
Implementasi mobile-first, lokal, tanpa perubahan backend atau database.

## Alur

Login seperti biasa, buka `/databases`, lalu Buka pada `shop_db`. Bisa juga buka
`/databases/mock-shop/schema` atau `/databases/mock-shop` setelah login.

- Mobile: Kembali ke Database List; heading/breadcrumb; kartu tabel 76px;
  kartu relasi FK; Riwayat Migration dan Bandingkan Schema di bawah relasi.
- Desktop mulai 1024px: sidebar 276px, daftar tabel enam kolom dan tabel relasi.
  Header/sidebar tetap di viewport; hanya panel konten kanan yang bergulir.
  Pergantian route mengembalikan scroll panel konten ke atas. Sidebar Admin
  memakai aturan yang sama pada desktop. Mobile tetap menggunakan scroll halaman.
  Layout tengah 768–1023px mempertahankan kartu dan hamburger yang bisa diakses.
- Schema di sidebar dapat dipilih dan dibuka/tutup. Tabel, Buka dan FK mengarah
  ke route detail tabel; pilihan sidebar mengikuti URL, termasuk direct link.
- Hamburger membuka navigasi global dan pohon schema database saat ini. Dialog
  memakai komponen Modal yang sudah ada (Escape, fokus, outside click, scroll lock).
- Relasi FK membuka tabel tujuan, bukan tabel asal. Tombol kembali pada detail
  mengarah ke schema terkait; perubahan route database menggulir ke atas.
- Identitas profil tetap dari sesi asli. Tidak ada penjelasan teknis mock di UI.

## Batas

Data schema berasal dari `schema-source.ts`, memakai interface `SchemaSource`
dan AbortSignal. Ketiga database contoh memiliki tabel/relasi masing-masing.
ID tidak dikenal ditolak, `hr_db` tetap gagal terhubung, tidak ada fallback ke
shop_db. Flag mock View Only dipropagasikan ke seluruh tabel, bukan hak akses
server. Model API dan otorisasi nyata belum diintegrasikan.

Detail tabel hanya menampilkan tujuan dan tautan kembali; Columns/Data belum
diimplementasikan. Migrasi dan Schema Diff masih memakai placeholder yang ada.
Belum ada eksekusi SQL, grant/revoke, atau mutasi backend.

## Preview development

Tambahkan pada `/databases/mock-shop/schema`:

| Query | Hasil |
| --- | --- |
| `?preview=loading` | Loading tetap tampil |
| `?preview=empty` | Schema public tanpa tabel/relasi |
| `?preview=noschema` | Tidak ada schema |
| `?preview=error` | Error dan Coba lagi |
| `?preview=readonly` | Semua tabel View Only |
| `?preview=denied` | Akses ditolak |
| `?preview=multiple` | public dan audit; pilih audit dari sidebar/drawer |
| `?schema=unknown` | Schema tidak ditemukan, tautan pemulihan |

Mode preview hanya development. Data production build masih berupa fixture ready.
Memilih schema memakai query; berpindah ke tabel/FK mempertahankan preview agar
state readonly/multiple tidak berubah di tengah alur.

## Pemeriksaan

Build dan lint berhasil. Total 31 test frontend lulus. Enam test schema menguji
FK mengarah ke tabel/PK valid, metadata, isolasi antar database dan mutasi fixture,
not-found/unavailable/denied, permission readonly, empty/multiple/unknown schema,
pelestarian URL preview dan pembatalan request saat navigasi.

Aset sidebar tabel/migrasi (20×20) dan divider (208×1) diunduh dari Figma, disimpan
lokal, dan tidak digambar ulang atau diregangkan. Ikon database dan logo memakai
komponen/aset lokal yang sudah ada; avatar memakai identitas sesi.

Verifikasi visual otomatis belum tersedia: Chromium lokal pada tahap sebelumnya
gagal startup dengan access denied; tidak ada browser tool tersedia pada sesi ini.
Tidak mengklaim perbandingan screenshot atau pengukuran DOM sudah lulus.

Cek manual di 320, 390, 768, 1024, 1440 dan 1728px: tidak ada overflow halaman,
scroll hingga Aksi Schema tidak tertutup bottom nav, buka/tutup drawer dan schema,
klik tabel/FK, Back/refresh/deep link, ganti database, serta semua preview state.

```powershell
npm --prefix frontend run build
npm --prefix frontend run lint
npm --prefix frontend test
```
