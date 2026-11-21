# Pathgres React SPA

React + TypeScript + Vite, React Router, TanStack Query, dan CodeMirror.

## Menjalankan dari root repo

```powershell
npm --prefix frontend ci
npm --prefix frontend run dev
```

Buka http://localhost:5173. Root sementara membuka katalog Design System.
Login dan halaman produk masih placeholder sampai tahap berikutnya.

| URL | Isi |
| --- | --- |
| /design-system/foundations | warna, typography, spacing |
| /design-system/components | komponen dasar dan interaksi |
| /design-system/navigation | contoh workspace dan kartu Pathgres |
| /design-system/patterns | SQL, progres, approval |
| /login, /home, /databases, /admin | kerangka route produk, belum terintegrasi |

## Pemeriksaan

```powershell
npm --prefix frontend run build
npm --prefix frontend run lint
npm --prefix frontend test
```

Test transport memerlukan Node >=24, tanpa database atau jaringan.
Backend JSON belum tersedia; proxy /api saja tidak menyediakan endpoint.

- [Arsitektur](ARCHITECTURE.md)
- [Design System](DESIGN_SYSTEM.md)
- [Mapping UI/API/Role](../docs/frontend/api-mapping.md)
