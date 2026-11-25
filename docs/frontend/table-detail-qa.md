# Table Detail: Column, Constraint & Index, Data

Implementasi FE lokal berdasarkan frame WBD desktop `337:6511`, `337:6575`,
`337:6653`, mobile `583:2154`, `587:4561`, `587:4602`. Tidak mengubah backend,
database, atau autentikasi; tidak melakukan commit/push.

## Membuka dan mencoba

Login seperti biasa. Buka Database → shop_db → users. URL langsung:
`/databases/mock-shop/schemas/public/tables/users`.

- Column: nama/tipe, nullable, default, penanda PK/FK. Kartu mobile dapat dibuka
  untuk melihat nullable/default dan tautan FK. Desktop menampilkan tabel.
- Constraint & Index: PK/UNIQUE/FK dan index yang konsisten dengan kolom; mobile
  memakai kartu expandable untuk definisi SQL, desktop dua panel tabel.
- Data: tampilan read-only. Empat baris per halaman mengikuti Figma; users memiliki
  1.250 baris contoh, halaman terakhir 313 berisi dua baris. Data berubah pada
  pergantian halaman. Nomor desktop/Prev/Next bekerja; mobile menampilkan indikator
  Halaman dan kontrol berukuran sentuh. Klik id pada header desktop untuk sort.
- Tab, page dan direction tersimpan di query string; Back/Forward/refresh mengikuti
  URL. Ganti tabel/tab mereset halaman; halaman terlalu besar dinormalisasi.
- Sidebar desktop tetap diam, scroll hanya konten kanan dengan scrollbar di tepi.
  Drawer dan bottom navigation yang sudah ada dipertahankan. Tidak ada label mock
  di antarmuka produk.

## Konsistensi data dan perbedaan antarframe

Frame Column mobile berisi id/username/email, sedangkan Column desktop dan Data
mobile/desktop berisi id/email/created_at. Implementasi memilih id/email/created_at
untuk users shop_db di semua ukuran, menjaga jumlah tiga kolom dari Schema
Explorer serta struktur/data/constraint yang konsisten. Username tetap ada pada
users analytics yang memiliki lima kolom. Index UNIQUE email juga ditampilkan
bersama constraint-nya agar model tetap sesuai struktur PostgreSQL.

Setiap database/tabel mempunyai definisi dan jumlah baris sendiri. Tidak ada
fallback ke users ketika ID/nama tidak ditemukan. Null ditampilkan NULL, string
kosong ditampilkan dua tanda kutip, false/0 dan string angka besar dipertahankan.
Nilai cell dirender sebagai teks React; tidak dijadikan HTML.

Model dan adapter berada di `table-source.ts`: `describe` untuk metadata dan
`rows` untuk halaman data. Masing-masing menerima AbortSignal. Query key
memisahkan database/schema/table/preview/page/direction; cache dibersihkan oleh
pengelola sesi yang sudah ada. Future API harus memvalidasi DTO/otorisasi server.
Mode View Only berasal dari mock workspace, bukan grant server. Data tetap
read-only untuk semua fixture; belum ada insert/update/delete maupun SQL execution.

## Preview development

Gunakan URL tabel users di atas, lalu:

| Query | Hasil |
| --- | --- |
| `?tab=data` | Isi data halaman pertama |
| `?tab=data&page=313` | Dua baris terakhir |
| `?tab=data&page=99999` | Dinormalisasi ke halaman 313 |
| `?tab=data&direction=desc` | Urutan id menurun |
| `?tab=constraints` | Constraint dan index |
| `?tab=data&tablePreview=empty` | Tidak ada data, metadata kolom tetap tersedia |
| `?tablePreview=loading` | Loading detail tabel |
| `?tablePreview=error` | Error detail tabel dan Coba lagi |
| `?preview=readonly&tab=data` | Mock workspace View Only |

Preview teknis hanya development. Build production masih fixture ready, belum API.

## Validasi

40 test frontend lulus; sembilan test baru memeriksa konsistensi semua sembilan
tabel, FK dan PK, pagination/rentang/sort, isolasi fixture, readonly/empty,
unknown/disconnected/error, NULL/false/0/empty, URL/page numbers dan cancellation.
Build serta lint diperiksa. Aset sidebar aktif 20×20 diunduh dari respons Figma,
memakai dimensi intrinsik dan tanpa filter tambahan; logo dan ikon lain reuse.

Verifikasi visual browser otomatis masih belum tersedia karena Chromium lokal
sebelumnya gagal startup (access denied); tidak mengklaim pengukuran runtime atau
visual regression lulus. Periksa manual pada 320, 390, 768, 1024 dan 1728px,
khususnya tab bar yang bisa digulir di layar sempit, kartu expandable, pagination
yang tidak tertutup navbar, keyboard tab/arrow dan sidebar aktif.

```powershell
npm --prefix frontend run build
npm --prefix frontend run lint
npm --prefix frontend test
```
