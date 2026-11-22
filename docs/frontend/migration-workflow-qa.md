# Review, Approval, Antrean dan Progress (FE mock)

Figma acuan desktop: inbox `314:3671`, review `314:3846`, konfirmasi `314:4029`,
reject `314:4210`, queue `320:3665`, queued `320:3848`, batch `320:4215`, failed
`320:4397`. Mobile: review `585:2667`, queue `587:2772`, batch `587:2883`.
Header, drawer dan sidebar yang sudah ada dipertahankan. Tidak ada aset baru.

## Route dan alur

Semua route di bawah berada dalam `/databases/:connectionId/migrations` dan
memerlukan sesi login yang sama dengan halaman Database.

| Suffix | Isi |
| --- | --- |
| `/reviews` | Daftar review, filter status |
| `/reviews/:draftId` | SQL Up/Down, approval per grup, alasan, revision history |
| `/queue` | Job, filter status, ringkasan antrean |
| `/queue/:jobId` | Timeline, batch progress, checkpoint, hasil/retry/cancel |

1. Simpan draft; dari detail klik **Ajukan Review**, lalu konfirmasi.
2. Semua grup mulai Pending. SQL dikunci setelah submit, termasuk jika membuka
   URL edit secara langsung. Alasan wajib pada approve/reject, maksimal1.000
   karakter. Setiap modal mengikat group dan review ID agar keputusan lama tidak
   diterapkan ke grup/revisi lain.
3. Pilih Group reviewer dan setujui ketiga grup satu per satu. **Disetujui** baru
   tercapai setelah seluruh grup menyetujui; tidak otomatis mengeksekusi SQL.
4. **Masukkan ke Antrean** → konfirmasi → detail job. Menunggu3 detik, lalu80 batch
   dengan interval250 ms. Satu job selesai sekitar23 detik tanpa antrean lain.
5. Polling lokal per1 detik hanya saat ada queued/running job. Refresh atau pindah
   halaman merekonstruksi progress dari jadwal tersimpan; tidak membuat job baru.
   Job pada database yang sama dijadwalkan berurutan. Database lain terpisah.
6. Selesai/gagal menghasilkan satu entri history per attempt; SQL snapshot, actor,
   waktu dan durasi bisa dibuka melalui detail execution yang sudah tersedia.
7. Reject menghentikan review. Edit dan simpan mengembalikan Draft; submit ulang
   membuat revision baru, semua approval direset. Alasan revisi lama tetap ada.
8. Cancel hanya untuk queued; jika job sudah berjalan, dialog lama ditolak. Cancel
   tidak membuat hasil eksekusi dan mengembalikan migrasi ke Approved.
9. Retry biasa hanya untuk Failed tanpa batch committed dan SQL yang tetap dikunci
   dengan approval lengkap. Partial failure memakai Resume/Compensation dengan
   persetujuan ulang. Attempt/Job ID baru; hasil gagal lama tetap tersimpan.

## Batas mock dan permission

Semua data workflow berada pada record draft sessionStorage per akun/database.
Logout, expiry/error sesi, dan pergantian akun menghapusnya bersama draft. Tidak
ada HTTP bisnis, SQL execution, worker server, sinkronisasi multi-tab/akun, atau
perubahan database nyata. Daftar review/job awal kosong sampai ada draft diajukan.

Capability reviewer adalah fixture FE: mode normal memberi akun yang sedang login
kemampuan mewakili ketiga grup melalui dropdown, supaya seluruh flow bisa dicoba
dalam satu akun. Ini **bukan** pembuktian bahwa akun tersebut benar-benar leader
ketiga grup; pembatasan self-approval dan membership belum ditetapkan oleh kontrak
API. Tidak mengadopsi kebijakan dari proyek lain. Mode `?preview=readonly` hanya
memberi capability Analytics Team (View): bisa memberi keputusan grup tersebut,
tetapi tidak membuat/mengedit/submit/enqueue/retry/cancel. Mutasi SQL memerlukan
mock grant Edit; tidak disimpulkan dari status platform admin.

Backend harus memasok capability efektif dan menegakkan seluruh aturan sendiri.
Model mock memeriksa transisi status/review ID, approval lengkap, scope, sesi,
AbortSignal, storage yang valid dan quota sebelum commit perubahan lokal.

Default hasil job sukses. Untuk mencoba gagal saat dev, tambahkan
`?jobOutcome=failed` pada URL review sebelum enqueue. Job menyimpan outcome itu,
gagal pada batch43 dengan42 batch selesai, untuk Resume/Compensation. Gunakan
`?jobOutcome=error` untuk gagal0 batch dan mencoba Retry; hapus param sebelum retry
agar hasil berikutnya sukses. Parameter ini tidak digunakan oleh production build;
tidak ada kontrol teknis simulasi pada UI produk. Tidak ada parser SQL atau
inferensi hasil eksekusi dari teks SQL.

## QA manual

- Coba happy path langkah1–6, refresh saat running dan buka history setelah selesai.
- Coba approve tanpa alasan, alasan terlalu panjang, klik ganda, approve grup sama
  dua kali, enqueue sebelum3 approval dan URL edit setelah submit.
- Coba reject → edit → submit ulang: revision bertambah, approval reset, alasan
  lama masih bisa dibuka. Dialog dari revision lama tidak bisa memberi keputusan.
- Buat2 migrasi approved dan enqueue keduanya: hanya1 running per database.
- Coba failed/retry, cancel queued, dan modal cancel yang ditahan sampai running.
- Coba filter review/job, Back/Forward, unknown IDs, database/akun lain, logout,
  storage disabled/quota dan `?preview=readonly`.
- Lebar360/390/768: stack mobile, editor scroll internal, tombol tidak tertutup
  bottom nav. Lebar1024/1440: panel utama/keterangan di kanan, sidebar tetap.
- Cek keyboard dropdown, textarea error, fokus modal, feedback dan badge100/500.

Unit tests meliputi lifecycle,3-group threshold, reasons, stale/concurrent action,
revision, FIFO, persisted progress, fail/retry/cancel, scope/capability, abort,
session cleanup, corrupt/quota storage dan route. Visual browser/E2E belum dapat
diverifikasi otomatis pada sesi ini.
