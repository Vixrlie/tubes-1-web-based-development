# Pathgres React SPA

React + TypeScript + Vite, React Router, TanStack Query, dan CodeMirror.

## Menjalankan dari root repo

```powershell
npm --prefix frontend ci
npm --prefix frontend run dev
```

Buka http://localhost:5173. Root menuju Login atau Beranda sesuai sesi.
Login memerlukan backend Go di port 8080 dan database platform lokal yang siap.
Di terminal terpisah dari root repo, jalankan `go run .` menggunakan `.env`
lokal yang sudah dikonfigurasi. Gunakan akun aktif yang tersimpan di database;
tidak ada akun/password demo di frontend. `ADMIN_USERNAME`/`ADMIN_PASSWORD`
backend dapat menyiapkan admin jika belum ada, bukan mengganti password akun lama.

| URL | Isi |
| --- | --- |
| /design-system/foundations | warna, typography, spacing |
| /design-system/components | komponen dasar dan interaksi |
| /design-system/navigation | contoh workspace dan kartu Pathgres |
| /design-system/patterns | SQL, progres, approval |
| /login | form username/password, sesi cookie Go |
| /home | Beranda dan shortcut, memerlukan sesi |
| /databases | Database List responsif dengan mock data; memerlukan sesi |
| /databases/:connectionId/schema | Schema Explorer responsif dengan mock data |
| /databases/:connectionId/schemas/:schemaName/tables/:tableName | Column, Constraint & Index, Data dan pagination dengan mock data |
| /databases/:connectionId/migrations | riwayat eksekusi, filter status/rollback dengan mock data |
| /databases/:connectionId/migrations/:executionId | detail eksekusi dan salin SQL read-only |
| /databases/:connectionId/migrations/new | form nama, SQL Up/Down, irreversible; simpan draft mock |
| /databases/:connectionId/migrations/drafts/:draftId[/edit] | detail/edit draft pada tab browser yang sama |
| /databases/:connectionId/migrations/diff, /databases/:connectionId/migrations/:id/preview, /admin | placeholder terlindungi; Admin hanya admin platform |

## Pemeriksaan

```powershell
npm --prefix frontend run build
npm --prefix frontend run lint
npm --prefix frontend test
```

Test transport/sesi memerlukan Node >=24, tanpa database atau jaringan.
JSON tersedia untuk `/api/auth/login`, `/api/auth/session`, `/api/auth/logout`.
JSON bisnis database/admin masih harus dibangun.

## Cek manual

1. Buka `/` tanpa sesi: diarahkan ke Login. Coba field kosong dan password salah.
2. Masuk dengan akun aktif: tampil Beranda; toggle eye dan loading berfungsi.
3. Klik Database: tampil empat kartu contoh, status koneksi dan View/Edit.
   Buka menuju Schema Explorer; coba sidebar/drawer, kartu tabel dan link foreign
   key. Detail tabel memiliki tiga tab dan pagination; Migrasi membuka history
   dan detail eksekusi. Migrasi Baru → Simpan Draft membuka detail draft; bisa
   diedit dan tetap ada saat refresh tab. Logout menghapus draft mock.
   Review/antrean/Admin masih disiapkan.
4. Refresh halaman: sesi dipulihkan dari cookie. Non-admin tidak melihat Admin;
   akses URL `/admin` langsung menampilkan akses terbatas.
5. Klik avatar → Keluar: kembali ke Login. Back/refresh tidak membuka sesi lama.
6. Katalog `/design-system/foundations` tetap dapat dibuka tanpa sesi/backend.

Test backend (tanpa database, httptest):
`go test ./internal/auth ./internal/session`.

- [Arsitektur](ARCHITECTURE.md)
- [Design System](DESIGN_SYSTEM.md)
- [Mapping UI/API/Role](../docs/frontend/api-mapping.md)
- [Database List: batas mock dan QA](../docs/frontend/database-list-qa.md)
- [Schema Explorer: batas mock dan QA](../docs/frontend/schema-explorer-qa.md)
- [Table Detail: batas mock dan QA](../docs/frontend/table-detail-qa.md)
- [Migration History: batas mock dan QA](../docs/frontend/migration-history-qa.md)
- [Buat Migrasi dan Draft: batas mock dan QA](../docs/frontend/migration-draft-qa.md)
