# Schema Diff, EXPLAIN dan Recovery (FE mock)

Figma acuan: Diff desktop `287:3641` dan mobile `587:3451`; EXPLAIN `339:8938`;
partial failure `320:4580`/`588:5154`; compensation `320:4767`; rollback/forward
`320:4956`/`320:5139`; persetujuan ulang `314:4742`, `383:8463`, `383:8600`.
Layout mobile-first dan komponen bersama dipertahankan, tanpa aset baru.

## Schema Diff

Route `/databases/:connectionId/migrations/diff`. Target mengikuti route; memilih
target lain berpindah workspace. Pembanding tersimpan pada `?reference=...`.
Koneksi yang tidak terhubung tidak bisa dipilih. Snapshot berasal dari mock
SchemaSource/TableSource yang sama dengan Explorer, bukan daftar tabel terpisah.

Perbedaan tabel/kolom ditampilkan BEFORE/AFTER. Pilihan awal hanya perubahan yang
didukung dan tidak destructive. SQL untuk create table dibuat dahulu, baru foreign
key, dan SQL Down menghapus foreign key sebelum tabel. Tabel dependency wajib
dipilih. Identifier di-quote. Default sequence ID diadaptasi menjadi identity.
Perubahan NOT NULL yang memerlukan backfill, kolom FK/PK dan constraint kompleks
tidak dapat dibuat otomatis. Perbandingan constraint/index/rename belum dicakup.

Penghapusan tabel/kolom dan perubahan tipe ditandai destructive, tidak dipilih
otomatis. Pilihan tersebut membuat draft irreversible tanpa SQL Down dan meminta
konfirmasi. Generator hanya menghasilkan kandidat draft yang harus ditinjau;
tidak memvalidasi kompatibilitas atau data sebenarnya. Draft target memerlukan
Edit, lalu memakai flow review yang sudah ada. Snapshot schema fixture tidak
berubah ketika job selesai; penerapan perubahan schema menunggu backend.

## Preview SQL dan EXPLAIN

Dari detail draft klik **Preview SQL & EXPLAIN**:
`/databases/:connectionId/migrations/drafts/:draftId/preview`.
Standalone: `/databases/:connectionId/migrations/explain`.
SQL Up/Down asli read-only, SQL untuk EXPLAIN dapat diedit tanpa mengubah draft.

EXPLAIN menampilkan fixture plan, cost, estimated rows, width dan planning time.
Rows mengikuti jumlah fixture tabel pada koneksi yang dipilih. Analyze menambah
fixture execution time dan memerlukan Edit. Tidak ada SQL yang dikirim/dijalankan,
termasuk ketika Analyze dicentang. Plan ini bukan hasil optimizer PostgreSQL.
Adapter mengenali SELECT/INSERT/UPDATE/DELETE sederhana dan nama tabel yang ada;
bukan parser SQL, tidak mendukung WITH/DDL/seluruh query kompleks. ALTER TABLE
menampilkan pesan unsupported, tidak diberi plan SELECT palsu. Setelah SQL/toggle
diubah, hasil lama direset. Error/retry/abort memakai state yang sudah tersedia.

## Rollback, Forward dan Recovery

Tombol muncul pada detail job/hasil eksekusi yang memiliki snapshot lokal.
Fixture history lama tanpa snapshot SQL Down tetap read-only. Semua permintaan
membuat record tindakan terpisah, SQL terkunci dan persetujuan ketiga grup baru.
Review dan queue yang sama digunakan kembali; execution asal tidak diubah.

- Rollback: hanya hasil reversible aktif terakhir, SQL Down asli.
- Forward: setelah rollback/compensation sukses, SQL Up asli.
- Resume: gagal sebagian, mulai dari checkpoint+1 tanpa mengulang batch selesai.
- Compensation: SQL Down asli, membalik hanya batch yang selesai, urutan reverse.
- Request terkait yang masih pending/approved/queued/running mencegah duplikasi.
  Eligibility diperiksa lagi saat enqueue agar approval lama tidak membatalkan
  eksekusi yang sudah bukan aktif terakhir.
- Semua job tindakan baru memiliki ID/history sendiri. Rollback menyimpan target
  execution, sehingga filter pasangan rollback menghilangkan pasangan sukses.
- Resume pada checkpoint42 dimulai42/80; compensation selesai42/42. Job tindakan
  dalam fixture ini selalu sukses; hasil itu tidak bergantung pada isi SQL.
- Retry biasa dibatasi pada kegagalan tanpa batch committed. Partial failure
  wajib Resume/Compensation. Rejection tindakan immutable perlu request baru dari
  target; tombol Edit tidak tersedia untuk mengubah snapshot tindakan.

Perubahan ada di `operation-model.ts`, `workflow-model.ts` dan `draft-source.ts`.
Storage tetap sessionStorage per akun/database, ikut dibersihkan oleh lifecycle
sesi. Metadata intent, snapshot, referensi job dan progress divalidasi saat dibaca.
Model mock ini tidak mengimplementasikan locking, cursor SQL nyata, transaksi,
kompensasi data atau otorisasi server; kontrak API tetap perlu disepakati.

## Cek manual

1. Login → shop_db → Migrasi → Schema Diff → pembanding analytics_db. Pilih
   perubahan additive default → Jadikan Draft Migrasi → periksa SQL Up/Down.
2. Coba pilih hanya events tanpa dependency event_types: ditolak. Coba pilihan
   drop orders: warning irreversible dan konfirmasi sebelum membuat draft.
3. Coba target/pembanding sama via URL, hr_db disconnected, readonly, error/empty
   `?toolsPreview=error|empty`, dan loading preview pada dev.
4. Detail draft → Preview SQL & EXPLAIN. Jalankan `SELECT * FROM public.users;`:
   shop1250 rows, analytics17 rows. DDL dan tabel tidak dikenal menampilkan error.
   Analyze readonly ditolak; perubahan SQL tidak mengubah draft.
5. Buat draft reversible → approve3 grup → enqueue → tunggu sukses → Ajukan
   Rollback → approve ulang → enqueue → sukses → Ajukan Forward → ulangi approval.
6. Dev: pada review draft biasa, tambahkan `?jobOutcome=failed` sebelum enqueue.
   Setelah42/80 gagal, Ajukan Resume → approve ulang → enqueue → selesai80/80.
7. Ulangi dengan draft lain dan pilih Compensation: reverse42→1, selesai42/42;
   lalu Forward tersedia. Irreversible tidak dapat rollback/compensation.
8. `?jobOutcome=error` membuat kegagalan0 batch untuk menguji Retry biasa; hapus
   param agar attempt berikutnya berhasil. Parameter hanya untuk dev build.
9. Coba duplicate click/request, stale rollback sesudah migrasi lain selesai,
   readonly, akun/database lain, refresh di tengah progress, quota dan logout.
10. Periksa360/390/768 dan1024/1440, SQL scroll internal, label BEFORE/AFTER,
    keyboard, modal dan bottom nav. Visual browser belum diverifikasi otomatis.

Unit tests mencakup diff/generator/dependency, quote escaping, parser scope plan,
DDL/read-only, data isolation, abort, operasi berantai, fresh approval, latest
active eligibility, immutable SQL, partial checkpoint dan reverse progress.
