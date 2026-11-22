# Migration History dan Execution Detail

Implementasi FE lokal mengacu Figma WBD desktop `269:2115`, mobile `585:2203`,
detail success desktop `269:2302`, mobile `585:2241`. Menggunakan komponen
Dropdown, Choice, Badge, Button, Alert, DataTable dan SqlPreview yang tersedia.

## Lingkup

- `/databases/mock-shop/migrations`: enam eksekusi dari desain. Mobile memakai
  kartu yang dapat dibuka; desktop memakai tabel. Panjang teks dapat membungkus.
- `/databases/mock-shop/migrations/E1048`: detail execution, metadata dan SQL
  read-only. Salin SQL memakai Clipboard API dan memberi feedback gagal/berhasil.
- E1050 menampilkan peringatan irreversible; E1047 memiliki link ke target E1045.
- Filter status dan sembunyikan pasangan rollback tersimpan di URL. Pasangan
  E1047 dan E1045 dihilangkan bersama hanya jika rollback dan targetnya sukses,
  version sama dan target bukan rollback. Detail/back mempertahankan filter.
- Inventory dan analytics memiliki empty history, tidak memakai data shop_db.
- Read-only workspace tetap bisa membaca history. Tidak ada perubahan SQL,
  session, backend, job queue, approval, atau database nyata.
- Migrasi Baru membuka form dan detail/edit draft mock. Draft tampil pada bagian
  sendiri, terpisah dari eksekusi. Review, Antrean dan Schema Diff masih disabled;
  diff/preview belum diimplementasikan. Lihat [QA draft](migration-draft-qa.md).

`MigrationSource` dan fixture terpisah dari komponen dan katalog. Query key
memuat connection, workspace preview, history preview dan mode mock. Pembatalan
memakai AbortSignal. Cache memakai siklus sesi yang sudah tersedia. JSON API
history/detail belum disambungkan; model status di tahap ini success/failed
untuk execution selesai, bukan state lifecycle draft/review/job.

Timestamp fixture dipertahankan sesuai Figma, tanpa mengasumsikan zona waktu
server. Kontrak API perlu menetapkan zona waktu, pagination dan hubungan rollback
eksplisit; jangan menyimpulkan rollback dari nama migration.

## Cek manual

Login memerlukan Go dan database platform lokal yang berjalan. Katalog tetap
bisa dibuka tanpa sesi, tetapi history produk memerlukan sesi aktif.

1. Dari Database → shop_db → Migrasi, periksa enam entri dan sidebar aktif.
2. Aktifkan sembunyikan rollback: hanya E1050, E1048, E1046, E1044 yang tampil.
3. Buka E1048, periksa metadata dan SQL, coba Salin SQL dan kembali ke history.
   Filter tidak hilang; Back/Forward dan refresh mempertahankan URL/state.
4. Buka E1047 langsung; klik target E1045. Buka E1050 untuk warning irreversible.
5. Filter Gagal menghasilkan empty filter state; Reset filter memulihkan daftar.
6. Buka inventory/analytics history: empty state, tanpa eksekusi shop.
7. Coba ID eksekusi tidak dikenal atau E1048 pada inventory: tidak ditemukan.
8. Coba `?preview=readonly`, `?migrationPreview=loading`, `empty`, `error`.
   Error menampilkan retry. Skenario preview hanya berlaku pada dev build.
9. Lebar 360/390/768: kartu, tombol minimum44px, drawer dan bottom nav; SQL
   bergulir di editor. Lebar1024/1440: sidebar tetap, konten kanan scroll;
   tabel dapat scroll horizontal di area tabel saat kolom tidak muat.
10. Keyboard: buka detail dengan Enter, toggle checkbox/rollback, dropdown dengan
    Arrow/Enter/Escape. Periksa clipboard denied dan navigasi setelah logout.

## Validasi

Unit tests meliputi scope database, detail, rollback, status, cloning fixture,
abort, error dan URL. Build/lint/test perlu dijalankan sebelum handoff. Pemeriksaan
visual browser dan clipboard tetap harus dilakukan manual; unit tests tidak
membuktikan pixel parity dengan Figma.
