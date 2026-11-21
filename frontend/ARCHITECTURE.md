# Struktur SPA Pathgres

Tahap 1 memisahkan aplikasi/katalog dan memetakan API. Route produk masih
placeholder tanpa autentikasi palsu, data privat, atau request otomatis.

| Lokasi | Tanggung jawab |
| --- | --- |
| `src/App.tsx` | Composition root: error boundary, providers, router |
| `src/app/router.tsx` | Satu pohon route produk dan katalog |
| `src/app/routes.ts` | Browser URL builders; encode identifier |
| `src/app/providers.tsx` | BrowserRouter dan QueryClientProvider stabil per mount |
| `src/app/query-client.ts` | Cache/retry policy |
| `src/app/layouts/` | Auth, app shell, database, admin |
| `src/catalog/` | Empat halaman katalog, layout dan CSS sendiri |
| `src/design-system/` | Komponen reusable yang sudah dibuat |
| `src/shared/api/` | Transport JSON dan normalisasi error |
| `src/pages/` | Placeholder dan 404 |
| `tests/` | Transport dan URL tests tanpa network/database |

Folder fitur `features/auth`, `features/databases`, `features/migrations`, dan
`features/admin` dibuat ketika fiturnya mulai diimplementasikan. Data contoh
katalog tidak boleh menjadi sumber sesi/data produk.

## Routing

- `/` tetap redirect ke `/design-system/foundations` sampai Login/sesi siap.
- Empat URL katalog tetap tersedia melalui nested routes, Outlet dan lazy imports.
- `/login`, `/home`, `/databases`, `/databases/:connectionId/*`, `/admin/*`
  diregistrasikan sebagai placeholder. Belum ada session/role guard.
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

Generic client bukan validasi runtime. Per fitur, tambahkan parser DTO, fungsi API,
dan query/mutation hooks setelah kontrak final. Session provider berikutnya harus
membersihkan cache privat saat logout/401/pergantian akun. Otorisasi tetap server.

Lihat [mapping UI/API/Role](../docs/frontend/api-mapping.md). Proxy Vite tidak
diubah: `/api` diteruskan ke Go 8080 tanpa rewrite, JSON adapter belum tersedia.

## Pemeriksaan

```powershell
npm --prefix frontend run build
npm --prefix frontend run lint
npm --prefix frontend test
```

Tests menggunakan Node >=24 native type stripping dan fetch stub untuk menguji
CSRF, cookie transport, HTTP/HTML error, abort, dan encoding URL. Tidak menggantikan
test backend/browser E2E. CI frontend belum ditambahkan pada tahap ini.
