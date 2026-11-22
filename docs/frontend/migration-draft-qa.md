# Buat Migrasi dan Draft (FE mock)

Acuan Figma: desktop create `269:2190`, mobile create `583:2195`, desktop draft
`269:2246`, mobile draft `585:2630`. Komponen Input, Choice, SqlEditor, SqlPreview,
Badge, Alert dan ConfirmationModal digunakan bersama. Tidak ada aset baru.

## Alur dan batas implementasi

- Migrasi Baru → nama, SQL Up/Down, pilihan irreversible → Simpan Draft → detail.
- Detail draft bisa diedit, dan bisa dibuka lagi dari bagian Draft Migrasi dalam
  history. Draft terpisah dari eksekusi, tidak pernah dianggap telah dijalankan.
- Route: `/databases/:connectionId/migrations/new`,
  `/databases/:connectionId/migrations/drafts/:draftId`, dan detail + `/edit`.
- Versi shop mulai v015 karena history sudah mencapai v014. Inventory/analytics
  mulai v001. Contoh v013 di Figma tidak diulang. Edit mempertahankan ID/versi.
- Review, antrean, diff dan eksekusi belum diaktifkan pada tahap ini.

`draft-source.ts` memisahkan storage dan UI. sessionStorage dipisahkan per akun
dan database. Draft tersimpan bertahan saat refresh tab yang sama; logout, sesi
kedaluwarsa/error, atau pergantian akun/role membersihkannya. Tidak ada sinkronisasi
antar-tab, penyimpanan server, pengiriman SQL, atau perubahan database nyata.

Nama wajib, maksimal120 karakter. SQL Up wajib, maksimal50.000 karakter. SQL Down
wajib untuk reversible, maksimal50.000 karakter. Irreversible menyimpan SQL Down
kosong; jika toggle dikembalikan sebelum save, isi editor Down sebelumnya pulih.
Validasi ini hanya field, bukan pemeriksaan sintaks/efek SQL PostgreSQL.

Adapter memeriksa akses edit, sesi aktif dan pembatalan sebelum save. Error/quota
storage tidak dianggap sukses. Error ditampilkan tanpa menghilangkan input.
Cancel mengonfirmasi perubahan belum disimpan; refresh/menutup tab memakai native
beforeunload. Sidebar/back browser belum memakai navigation blocker, jadi simpan
sebelum navigasi internal agar perubahan tidak hilang.

## Cek manual

1. Login → Database → shop_db → Migrasi → Migrasi Baru.
2. Simpan kosong: nama/Up/Down error dan fokus berpindah ke field bermasalah.
3. Isi nama, SQL Up dan Down → simpan → detail v015, SQL sesuai isian.
4. Refresh, kembali ke Migrasi dan buka kartu draft. Edit dan simpan: ID/versi
   dan jumlah draft tetap sama. Buat draft kedua: versi berbeda.
5. Coba irreversible: Down read-only, detail warning dan tanpa SQL Down.
6. Coba inventory/analytics atau ID draft tidak dikenal: tidak ada data shop.
7. `?preview=readonly`: form ditolak, detail baca tetap tersedia. Logout/login:
   draft mock terhapus. Coba blocked storage/quota: error tanpa feedback sukses.
8. Lebar360/390/768: satu kolom, SQL panjang scroll di editor, tombol tidak
   tertutup bottom nav. Lebar1024/1440: ringkasan kiri, editor kanan, sidebar
   tetap dan scroll konten kanan. Periksa keyboard, error dan cancel modal.

Unit tests mencakup validasi, simpan/edit, persistensi adapter, versi concurrent,
scope akun/database, denied/readonly, abort/sesi, quota/corrupt storage, cleanup
dan route. Visual browser/E2E masih perlu diperiksa manual.
