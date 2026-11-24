# Pathgres Design System

Standar feedback error (Alert): background `error-100`, teks `error-500`,
judul font-weight 600 (bold), isi font-weight normal. Berlaku pada komponen Alert di katalog dan produk.
Teks Alert info memakai `primary-500`, success `success-500`, dan warning
`warning-600` (`#c47e09`); background soft masing-masing tetap dipertahankan.

Navigasi produk mobile mengikuti overlay Figma terbaru: panel putih selebar
viewport tepat di bawah app bar, heading/deskripsi konteks, divider dan baris
ber-chevron. Tombol tiga garis ada di kanan. `MobileContextMenu` dipakai bersama
oleh aplikasi dan contoh interaktif pada katalog Navigation. Panel Daftar Database
berisi pilihan koneksi; workspace berisi schema, tabel dan Migrasi; Admin berisi
Pengguna, Grup, Koneksi Database. Sidebar desktop tetap memakai `DatabaseSidebar`
dan `AdminSidebar`, bukan versi desktop yang diperkecil ke dalam drawer.

ProfilePopup mengikuti Figma `498:10530`: avatar inisial56px, nama16px/600,
email13px, badge Admin/User, divider tipis dan Logout merah soft dengan ikon
Figma lokal `profile-logout.svg`. Props `email` opsional; email kosong tidak
ditampilkan. Produk memakai fullName (fallback username), email asli dari sesi,
dan badge berdasarkan isPlatformAdmin. Nama/email contoh hanya milik katalog.

## Komponen workspace desktop

Sidebar memakai satu divider pada header bersama, baris menu 48px, slot ikon
20px, gap ikon/teks 12px, serta indent tabel 32px dari baris schema. Chevron
sejajar di kanan dan label panjang memakai ellipsis. Spacing ini dimiliki
komponen Sidebar/SidebarItem; halaman tidak menambah padding atau divider lagi.
Preview katalog memakai variant schema yang sama dengan halaman produk.

`Tabs` menerima `idPrefix` opsional untuk menghubungkan tab dan panel secara
aksesibel. `Pagination variant="table"` menyediakan kontrol Prev/Next, nomor
halaman desktop dan indikator Halaman pada mobile. `DataTable` mendukung header
interaktif dan `aria-sort` melalui properti kolom opsional.

Contoh interaktif ada di `/design-system/navigation`, melalui `src/catalog/NavigationCatalog.tsx`.
Katalog awal dibuat untuk desktop. Database List produk kini diimplementasikan
mobile-first mengikuti frame final mobile dan desktop, memakai komponen yang sama.

| Komponen | API utama dan perilaku |
| --- | --- |
| ProductHeader | `active`, `onNavigate`, `profile`, `showAdmin`; pilihan Database/Admin dengan state aktif |
| DatabaseSidebar | `database`, `schemas`, `selection`, `onSelect`, `loading`; `variant="schema"`, `activeSchema`, `onSelectSchema` untuk explorer; schema dapat dibuka/tutup, tabel dan Migrasi dapat dipilih |
| AdminSidebar | `active`, `onNavigate`; Pengguna, Grup, Koneksi Database |
| DatabaseCard | Default katalog; `variant="workspace"` untuk tile responsif produk, `readOnly`, `connectionError`, `onReconnect`; Buka nonaktif saat terputus |
| TableCard | `name`, `columnCount`, `primaryKey`, `permission`, `onOpen`, `disabled`; seluruh kartu dapat diklik |
| ListToolbar | Search terkontrol, `resultCount`, filter Dropdown dan slot `action` opsional |
| MetadataPanel | `title`, `items` label/value, slot `action`; memakai definition list |
| MigrationJobCard | Status queued/running/success/failed/canceled, progres batch opsional, aksi detail |
| ApprovalGroupCard | Nama grup, threshold persetujuan, daftar approver, aksi opsional |
| ProductBottomNavigation | Beranda/Database/Admin, state aktif dan `showAdmin`; `inline` untuk preview di desktop |

Komponen menerima data dan callback; tidak memanggil API bisnis atau memberikan
hak akses. Parent harus mengirim data/status yang benar dan menangani navigasi.
Label status pada contoh approval menunjukkan presentasi komponen, bukan keputusan
approval di backend.

```tsx
import { ProductHeader, TableCard, DatabaseSidebar } from './design-system'

<ProductHeader active="database" onNavigate={navigateWorkspace} profile={profile} />
<TableCard
  name="products"
  columnCount={6}
  primaryKey="id"
  permission="View + Edit"
  onOpen={openProducts}
/>
<DatabaseSidebar
  database="shop_db"
  schemas={[{ id: 'public', name: 'public', tables: [{ id: 'products', name: 'products' }] }]}
  selection={{ kind: 'table', schemaId: 'public', tableId: 'products' }}
  onSelect={selectTable}
/>
```

Pemeriksaan tambahan: build dan lint berhasil. Preview browser desktop 1440 px
berhasil untuk switch Database/Admin, pilihan menu admin, collapse schema,
pencarian tabel, filter hak akses, membuka detail TableCard dan menutupnya dengan
Escape. Viewport dikembalikan ke ukuran awal setelah pemeriksaan.

Implementasi React + TypeScript dari empat artboard Style Guide WBD:

| Acuan Figma | Implementasi |
| --- | --- |
| 731:3126 — Foundations | `src/design-system/tokens.css` |
| 734:3126 — Components & Interaction | `primitives.tsx`, `components.css` |
| 739:3219 — Navigation & Data | `navigation.tsx`, `data.tsx` |
| 741:3388 — Responsive Patterns & QA | responsive CSS, patterns di katalog |

File Figma: https://www.figma.com/design/lylECEm2d95UMZ0R8djvPk/WBD

## Menjalankan katalog

Dari root repository:

```powershell
npm --prefix frontend install
npm --prefix frontend run dev
```

Buka http://localhost:5173/design-system/foundations.

Empat route katalog: `/design-system/foundations`, `/design-system/components`,
`/design-system/navigation`, dan `/design-system/patterns`. Route ini adalah katalog
komponen dengan data contoh; belum menghubungkan API bisnis atau autentikasi.
Root `/` sementara diarahkan ke katalog. Ketika halaman produk dibuat, ubah
redirect di `src/app/router.tsx` dan pertahankan katalog di `/design-system/*`.

## Pemakaian

```tsx
import { Button, Input, Badge, Card, DatabaseCard } from './design-system'

<Button variant="primary" onClick={save}>Simpan</Button>
<Button variant="secondary" disabled>Batal</Button>
<Button loading>Menyimpan</Button>
<Input label="Nama database" value={name} onChange={handleName} error={error} />
<Badge tone="success">Terhubung</Badge>
<Card>Konten</Card>
<DatabaseCard
  name="shop_db"
  description="PostgreSQL · 1 schema · 3 tabel"
  permission="View + Edit"
  onOpen={openDatabase}
/>
```

Import seluruh komponen dari `src/design-system/index.ts`. Halaman produk sebaiknya
memakai komponen ini, sehingga perubahan master berlaku di seluruh pemakaian.

| Kelompok | Komponen |
| --- | --- |
| Primitive | Button, IconButton, Input, Select, Dropdown, Textarea, Choice, Badge, Tabs |
| Feedback | Alert, Spinner, EmptyState, Toast |
| Navigation | Header, Sidebar, SidebarItem, Breadcrumb, BottomNavigation, Icon |
| Surface dan data | Card, DatabaseCard, KpiCard, DataTable, Pagination, MetadataRow, ListRow |
| Overlay | Modal, MobileContextMenu, ConfirmationModal, ProfilePopup |
| Pathgres | SqlEditor, SqlPreview, BatchProgress, MigrationJobRow, ApprovalGroupRow, SchemaDiffRow |

Button mendukung `primary`, `secondary`, `tertiary`, `destructive`, ukuran
`sm/md/lg`, serta `disabled/loading`. Hover, pressed, dan keyboard focus memakai
pseudo-class CSS asli. IconButton memerlukan `label` untuk nama aksesibel.
Primary dan destructive pada state disabled/loading memakai fill `neutral-100`
dan teks/spinner `neutral-500`. Destructive aktif memakai fill `error-500`.
Choice mendukung checkbox/radio dengan atribut input HTML asli.

`Input` mendukung tipe search/password/email dan lainnya; password memiliki toggle.
`Select` memakai native select untuk dukungan keyboard dan perangkat mobile.
`Dropdown` menyediakan overlay pilihan yang mengikuti style Pathgres. Gunakan
`options`, `value`, dan `onChange`; opsi dapat dinonaktifkan. Mendukung panah,
Home/End, Enter/Space, Escape, Tab, dan pencarian melalui ketikan awal label.
Contoh Hak akses pada katalog menggunakan Dropdown.
Password menggunakan ikon eye/eye-slash. Fokus input, textarea, select, dan
Dropdown mengubah border menjadi primary 500 dengan ketebalan tetap 1 px;
tanpa outline tambahan di luar field.
`DataTable<T>` menerima `columns`, `rows`, `rowKey`, dan `caption`.
`Tabs` dikendalikan melalui `value/onChange` dan mendukung panah, Home, serta End.

`Modal` menggunakan native dialog agar focus trap dan Escape konsisten. Drawer
katalog memakai `Modal` dengan `drawer`; navigasi produk memakai
`MobileContextMenu` (`Modal context`) dengan batas tinggi dan safe area. `ProfilePopup` memakai disclosure native dengan
Escape dan penutupan saat klik di luar. Avatar memakai inisial nama dari props;
foto contoh pengguna pada Figma tidak dijadikan identitas pengguna aplikasi.

## Tokens dan responsive

- Poppins 400/500/600 disimpan melalui `@fontsource/poppins`, tanpa Google Fonts runtime.
- Primary `#317BFF`; neutral dan semantic mengikuti variabel Figma.
- Spacing 4/8/12/16/24/32/48; control radius 6, panel 10, card 16, pill full.
- Figma input memiliki beberapa radius berbeda; implementasi memakai token control
  6 px sesuai aturan foundations agar seluruh form konsisten.
- Kartu umum memakai radius 16; tabel desktop flat tanpa shadow.
- Header desktop 98 px; mobile 64 px, breakpoint 767 px. Breakpoint 1100 px
  menyesuaikan grid katalog. Breakpoint adalah keputusan implementasi web.
- Sidebar aktif: teks dan ikon primary, tanpa fill. Hover: primary 50.
- Mobile: tabel menjadi kartu berlabel, navigasi bawah memperhitungkan safe area,
  target sentuh minimal 44 px, toast berada di atas navigasi bawah.
- SQL editor di-load terpisah ketika dipakai, sehingga katalog awal lebih ringan.
- Komponen memakai CSS variables. Token tersedia pula melalui Tailwind `@theme inline`.

Aset logo dan ikon berasal dari respons Figma dan disimpan di
`public/design-system/`. Screenshot artboard tidak dipakai sebagai UI.
Katalog mengadaptasi artboard besar menjadi halaman web responsif, sehingga
susunan kolomnya mengikuti lebar viewport.

## Pemeriksaan

```powershell
npm --prefix frontend run build
npm --prefix frontend run lint
```

Keduanya berhasil pada implementasi ini. Pemeriksaan visual browser otomatis
belum selesai karena browser preview gagal menjangkau server lokal pada percobaan
awal. Periksa katalog pada desktop dan viewport 390 px sebelum integrasi halaman
produk; khususnya drawer, modal, tabel mobile, tabs, dan SQL editor.

## Struktur aplikasi tahap 1

Katalog dipindahkan dari App.tsx ke src/catalog/; empat URL tetap tersedia.
Router dan provider berada di src/app/. Detail ada di [ARCHITECTURE.md](ARCHITECTURE.md)
dan [mapping API](../docs/frontend/api-mapping.md). Login dan Beranda kini memakai
komponen katalog dengan sesi cookie Go. Database memakai data bisnis mock;
Admin menyediakan monitoring Activity Logs dan Observability; CRUD administrasi
masih placeholder terlindungi.
Aset `login-background.png` adalah background asli Figma Login, disimpan lokal.

Form Buat Migrasi/detail draft menggunakan komponen bersama. `SqlEditor` menerima
`error` opsional; pesan error terhubung ke content editor melalui aria-describedby
dan aria-invalid, tanpa mengubah pemakaian SqlPreview di layar lain. Model dan
penyimpanan draft berada di feature migrations, bukan di design system.


Beranda sekarang mobile-first: ilustrasi berada setelah deskripsi dan sebelum
CTA pada mobile, kemudian pindah ke kolom kanan pada desktop. Menu EXPLAIN dapat
dibuka langsung dari Aksi Cepat Migrasi, selain melalui preview SQL draft.
Batas checkpoint dan checklist lintas viewport ada di
[QA Database/Beranda](../docs/frontend/database-home-final-qa.md).
