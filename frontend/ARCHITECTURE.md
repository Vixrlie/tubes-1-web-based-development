# Struktur SPA Pathgres

SPA memiliki Login, Beranda dan application shell dengan sesi cookie Go asli.
Database List, Schema Explorer dan Table Detail responsif memakai mock data
terpisah. Migrasi dan admin masih placeholder terlindungi. Katalog tetap publik.

| Lokasi | Tanggung jawab |
| --- | --- |
| `src/App.tsx` | Composition root: error boundary, providers, router |
| `src/app/router.tsx` | Satu pohon route produk dan katalog |
| `src/app/routes.ts` | Browser URL builders; encode identifier |
| `src/app/providers.tsx` | BrowserRouter, QueryClientProvider dan SessionProvider |
| `src/app/query-client.ts` | Cache/retry policy |
| `src/app/layouts/` | Auth, app shell, database, admin |
| `src/catalog/` | Empat halaman katalog, layout dan CSS sendiri |
| `src/design-system/` | Komponen reusable yang sudah dibuat |
| `src/shared/api/` | Transport JSON dan normalisasi error |
| `src/features/auth/` | Login, parser DTO, controller sesi, provider dan guards |
| `src/features/home/` | Beranda desktop berdasarkan Figma, shortcut sesuai role |
| `src/features/databases/` | Database List, Schema Explorer, sidebar, model dan source mock yang bisa diganti API |
| `src/pages/` | Placeholder dan 404 |
| `tests/` | Transport, URL dan siklus sesi tanpa network/database |

Folder `features/migrations` dan `features/admin` dibuat ketika fiturnya dimulai.
Fixture database berada di `features/databases/mock-source.ts`, terpisah dari
katalog dan sesi. Query key memuat mode mock; fixture bukan bukti grant backend.
Adapter `DatabaseSource` menerima AbortSignal agar request batal saat navigasi.
`TableSource` memisahkan deskripsi kolom/constraint/index dan halaman data.
Query key mencakup connection/schema/table, mode preview, halaman dan urutan;
tidak memakai placeholder data dari tabel sebelumnya. Tab/page/direction disimpan
di URL. Halaman di luar rentang dinormalisasi ke halaman terakhir, pergantian
tab/tabel mengatur ulang halaman. UI data masih read-only, tanpa mutasi SQL.
`SchemaSource` memakai model workspace/schema/table/FK. `DatabaseLayout` mengambil
query workspace dan membagikannya ke halaman anak melalui Outlet context.
Drawer memakai query key yang sama; query dideduplikasi. Schema terpilih berasal
dari `?schema=...`; tabel terpilih berasal dari path. Cache berbeda per connection
ID dan skenario preview, dan ikut dibersihkan oleh pengelola sesi yang sudah ada.

## Routing

- `/` menuju `/home`; tanpa sesi diarahkan ke `/login`.
- Empat URL katalog tetap tersedia melalui nested routes, Outlet dan lazy imports.
- `/login` memakai guest guard; pengguna yang sudah masuk kembali ke tujuan
  internal sebelumnya atau `/home`.
- `/home`, `/databases`, `/databases/:connectionId/*`, `/admin/*` memerlukan
  sesi aktif. `/admin/*` juga memerlukan flag `isPlatformAdmin` dari server.
- Guard menunggu pemeriksaan sesi; gangguan server menampilkan retry, bukan
  dianggap sebagai sesi berakhir. Non-admin yang membuka Admin melihat 403 UI.
- Header/sidebar produk memakai router navigation, bukan pergantian demo state.
- URL tabel memisahkan schema/table; URL backend dipetakan di adapter API kelak.
- Alamat tidak dikenal menampilkan 404.

Contoh lokal: `/home`, `/databases/1/schema`,
`/databases/1/schemas/public/tables/products`, `/admin/groups`.
ID 1 hanya contoh parameter, bukan bukti resource server ada.

## Query dan API

Query provider: staleTime 30 detik, refetch on window focus off, satu retry
network/server error, tanpa retry pada 4xx/CSRF/kontrak salah. Mutasi tanpa retry.
Belum ada query bisnis yang dijalankan saat mount.

`createApiClient()` memakai same-origin `/api`, credentials include, AbortSignal,
Accept JSON, penolakan redirect/HTML, dan CSRF untuk mutasi. Token CSRF berasal
dari callback; tidak ada session token di localStorage atau cookie HttpOnly yang
dibaca JavaScript. Client tidak otomatis menghubungi endpoint yang belum tersedia.

Contoh berikut baru dihubungkan setelah DTO dan endpoint disepakati:

```ts
const api = createApiClient({ getCsrfToken: () => session.csrfToken })
const result = await api.request<DatabaseListDto>('/databases', {
  signal,
  query: { page: 1 },
})
```

Generic client bukan validasi runtime. Auth memvalidasi DTO melalui `parseSession`.
Per fitur berikutnya, tambahkan parser DTO dan query/mutation hooks. Gunakan
`useSession().session.api` untuk request bisnis: 401 membersihkan sesi dan cache.
Controller mengabaikan respons sesi/401 dari generasi akun sebelumnya.
Logout, pergantian akun/role, sesi kedaluwarsa dan kegagalan pemeriksaan sesi
membersihkan cache privat. Cookie HttpOnly tetap dikelola browser; CSRF hanya
di memori. Pemeriksaan ulang saat fokus/visibility tidak memakai polling yang
memperpanjang idle timeout. Otorisasi sebenarnya tetap di server.

Lihat [mapping UI/API/Role](../docs/frontend/api-mapping.md). Proxy Vite tidak
diubah: `/api` diteruskan ke Go 8080 tanpa rewrite. Tiga endpoint auth JSON
tersedia di `internal/auth/api.go`; adapter database/admin belum tersedia.

## Pemeriksaan

```powershell
npm --prefix frontend run build
npm --prefix frontend run lint
npm --prefix frontend test
```

Tests menggunakan Node >=24 native type stripping dan fetch stub untuk menguji
CSRF, cookie transport, HTTP/HTML error, abort, dan encoding URL. Tidak menggantikan
test backend/browser E2E. CI frontend belum ditambahkan pada tahap ini.
