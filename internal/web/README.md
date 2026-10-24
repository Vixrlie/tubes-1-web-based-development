# Panduan Development: Komponen & CSS

Folder ini berisi semua yang berhubungan dengan tampilan: template HTML, komponen reusable, dan CSS.
Stack: Go `html/template` + HTMX. Tidak ada build step untuk frontend.

---

## 1. Struktur folder

```
web/
├── render.go          # Renderer + fungsi template (dict, list, css, attr, icon, iconNames)
├── shell.go           # Data layout (header, sidebar) dan tipe pendukungnya
├── table.go           # Tipe data untuk komponen tabel
├── dropdown.go        # Tipe data untuk opsi dropdown
├── databases.go       # Tipe data halaman Daftar Database (DBCard, DatabasesPage)
├── devpage.go         # Route /dev/components (halaman uji komponen)
├── router.go
├── static/
│   ├── icons/         # SVG icon, 1 file = 1 icon
│   ├── css/
│   │   ├── app.css    # Entry point: hanya @import
│   │   ├── themes.css # TOKEN SEMANTIK (warna per fungsi, spacing, radius, dark theme)
│   │   ├── tokens.css # TOKEN MENTAH dari Figma (palet warna, font, ukuran teks)
│   │   ├── base.css, typography.css, layout.css
│   │   └── components/<nama>.css   # 1 komponen = 1 file CSS
│   ├── img/           # logo-full.png, logogram.png, login-page-image.png
│   ├── js/            # password-toggle.js
│   └── css/pages/     # CSS per halaman (login.css)
└── templates/
    ├── layout.html    # Kerangka halaman
    ├── components/    # Komponen reusable (dipanggil lewat {{template ...}})
    ├── partials/      # Potongan layout (header, sidebar, flash, avatar)
    ├── dev/           # Halaman uji komponen
    └── ...            # Halaman: home.html, login.html, admin/...
```

Semua file di `templates/components/` dan `templates/partials/` otomatis ikut di-parse ke setiap halaman, jadi
komponen bisa dipakai di template mana saja tanpa import.

---

## 2. Aturan CSS

### Dua lapis token

| File | Isi | Boleh dipakai di komponen? |
|---|---|---|
| `tokens.css` | Palet mentah: `--color-primary-500`, `--color-neutral-900`, `--font-size-p3`, ... | Hanya untuk ukuran/font. Untuk warna, lebih baik lewat token semantik |
| `themes.css` | Token semantik: `--color-text`, `--color-text-muted`, `--color-border`, `--color-danger`, `--color-focus`, `--space-*`, `--radius-*` | **Ya, ini yang utama** |

Tujuan pemisahan ini: mengganti tema (misal dark mode) cukup mengubah mapping di `themes.css`, komponen tidak
perlu disentuh. Dark theme sudah tersedia dan opt-in lewat `<html data-theme="dark">`.

Token semantik yang sering dipakai:

- **Teks:** `--color-text` (neutral-900), `--color-text-muted` (neutral-500), `--color-text-subtle`
- **Surface:** `--color-bg`, `--color-surface`, `--color-surface-raised`
- **Garis:** `--color-border`, `--color-border-strong`
- **Aksi:** `--color-primary`, `--color-danger`, `--color-focus`, `--focus-ring`, `--focus-ring-danger`
- **Jarak:** `--space-1` (0.25rem) sampai `--space-16`
- **Radius:** `--radius-sm`, `--radius-md`, `--radius-lg`, `--radius-xl`, `--radius-full`
- **Transisi:** `--transition-fast`, `--transition-base`

### Ukuran teks (dari Figma)

Title: `--font-size-t1` (22px) ... `--font-size-t6` (10px). Paragraf: `--font-size-p1` (18px) ... `--font-size-p6` (8px).

### Latar halaman

Semua halaman memakai gradasi diagonal (`--page-bg` di `layout.css`): `--color-primary-50` di kiri atas ke
`--color-primary-200` di kanan bawah, menempel di layar. Header dan kartu punya latar sendiri. Login
mengecualikan diri dengan `body:has(.login) { background: var(--color-bg); }`. Halaman lain yang butuh latar
polos cukup menimpa `body` dengan cara yang sama.

### Konvensi penulisan CSS komponen

1. **Satu komponen = satu file** di `static/css/components/<nama>.css`, lalu daftarkan di `app.css`:
   ```css
   @import "components/nama.css";
   ```
2. **Penamaan BEM**: `.komponen`, `.komponen__bagian`, `.komponen--varian`, state dengan `.is-error`, `.is-disabled`, `.is-active`.
3. **Scope ke class komponen.** Jangan menulis selector `table {}` atau `input {}` polos, supaya halaman lain tidak berubah.
4. **Override per-instance lewat CSS variable**, bukan class baru. Contoh di button: `--btn-fill`, `--btn-fg`,
   `--btn-stroke`, dengan fallback ke nilai default (`--_fill`, ...). Pola: `background: var(--btn-fill, var(--_fill))`.
5. **Ukuran icon ikut teks** memakai satuan `em` (lihat `input.css`) atau `--icon-size` (lihat `icons.css`).
6. Perhatikan **specificity**: aturan dengan selector lebih spesifik menang walau ditulis lebih dulu. Contohnya
   `.dev h2` (halaman dev) mengalahkan `.table-card__title`. Kalau ukuran tidak sesuai, cek aturan lain dulu di DevTools.

---

## 3. Cara memakai komponen

Komponen dipanggil dengan `{{template}}` dan parameter bernama lewat helper `dict`:

```html
{{template `button` (dict `Label` `Simpan` `Type` `submit` `IconLeft` `check`)}}
```

### Aturan penting di dalam template

- **Pakai backtick** (`` `...` ``) untuk string Go template di dalam tag HTML, **bukan** tanda kutip ganda.
  Formatter HTML VS Code merusak `"..."` di dalam atribut (menyisipkan spasi dan memotong string).
- **Jangan taruh `{{ }}` langsung di `style="..."`.** Editor menganggapnya CSS dan menampilkan error palsu.
  Bungkus dalam template kecil (lihat `button-style-attr` di `button.html`).
- **Atribut mentah** (`Attrs`) harus dibungkus `attr`: `(attr `hx-post="/x" hx-target="#y"`)`.
  `attr` dan `css` hanya untuk **literal di template, jangan pernah dari input user** (melewati sanitizer).
- Parameter yang tidak diisi bernilai kosong. Pakai `or` untuk default: `{{or .Variant `primary`}}`.
- Setiap komponen punya komentar di bagian atas file `.html`-nya yang menjelaskan semua parameter. Itu
  dokumentasi utama; ringkasan di bawah hanya pengingat.

### Daftar komponen

| Komponen | File | Fungsi |
|---|---|---|
| `button` | `components/button.html` | Tombol / link bergaya tombol |
| `label` | `components/label.html` | Badge status berwarna |
| `icon` | `components/icon.html` | Icon SVG inline |
| `input` | `components/input.html` | Input field (label, helper, error, icon) |
| `dropdown` | `components/dropdown.html` | Dropdown (select) bergaya input |
| `table` | `components/table.html` | Tabel dengan kolom dan baris dari backend |
| `action-bar` | `components/action-bar.html` | Bar melayang "perubahan belum disimpan" |
| `side-link` | `components/side-link.html` | Satu baris link di sidebar |
| `db-card` | `components/db-card.html` | Kartu database (terhubung / gagal) |
| `avatar`, `flash`, `header`, `sidebar` | `partials/` | Potongan layout |

### Button

```html
{{template `button` (dict `Label` `Tambah` `IconLeft` `plus`)}}
{{template `button` (dict `Label` `Batal` `Variant` `secondary`)}}
{{template `button` (dict `Label` `Buka` `Href` `/db/x` `Variant` `tertiary`)}}
{{template `button` (dict `Label` `Hapus` `Fill` `var(--color-danger)` `Color` `var(--color-on-danger)`)}}
```

`Variant`: `primary` (default) | `secondary` | `tertiary`. `State`: `default` | `active` | `disabled`.
Kalau `Href` diisi, dirender sebagai `<a>`.

### Label (badge)

```html
{{template `label` (dict `Label` `Terhubung` `Variant` `success`)}}
```

`Variant`: `success` | `error` | `warning` | `info` | `neutral` (default).

### Icon

```html
{{template `icon` `heart`}}
```

Nama = nama file di `static/icons/` tanpa `.svg` (huruf kecil, angka, strip). Nama yang salah membuat
**seluruh halaman gagal render** ("gagal render halaman") dan pesannya muncul di log server.
Daftar semua icon bisa dilihat di `/dev/components`.

Menambah icon: taruh `static/icons/<nama>.svg`. Pakai `currentColor` untuk warna (`stroke="currentColor"`
atau `fill="currentColor"`) supaya ikut warna teks.

### Input

```html
{{template `input` (dict `Label` `Username` `Name` `username` `Placeholder` `Masukkan username`)}}
{{template `input` (dict `Label` `Email` `Name` `email` `Type` `email` `Error` `Format email salah`)}}
{{template `input` (dict `Label` `Kode` `Name` `kode` `State` `disabled` `IconLeft` `lock`)}}
```

- `Error` opsional. Kosong = tampilan normal dan pesan tidak muncul. Terisi = field merah dan pesan menggantikan `Helper`.
- Placeholder (kosong) berwarna neutral-500, teks terisi neutral-900. Otomatis lewat CSS, tanpa JS.
- `PasswordToggle` `true` menambah tombol mata untuk tampil/sembunyikan password. Halaman harus memuat
  `<script src="/static/js/password-toggle.js" defer></script>` (sudah ada di `login.html`).

### Dropdown

Memakai `<select>` bawaan browser yang di-style (aksesibel, ikut form, bisa dipakai dengan HTMX). Gayanya
mengikuti input, jadi `dropdown.css` bergantung pada `input.css` (keduanya sudah di-import di `app.css`).

Di Go (`dropdown.go`):

```go
opts := []web.DropdownOption{
    {Value: "public", Label: "public"},
    {Value: "analytics", Label: "analytics"},
}
```

Di template:

```html
{{template `dropdown` (dict `Label` `Schema` `Name` `schema` `Placeholder` `Pilih schema` `Options` .Opts `Value` .Selected)}}
{{template `dropdown` (dict `Name` `schema` `Options` .Opts `Error` `Schema wajib dipilih` `IconLeft` `database`)}}
```

- `Options` wajib. Alternatif tanpa tipe Go: `(list (dict `Value` `a` `Label` `A`) (dict `Value` `b` `Label` `B`))`.
- `Value` = pilihan yang sedang terpilih. Kosong -> placeholder tampil (neutral-500). Setelah memilih -> neutral-900.
- `Error` opsional, `State` `disabled`, `IconLeft` / `IconRight`, `Helper`, dan `Attrs` sama seperti input.
- Untuk memuat ulang bagian halaman saat pilihan berubah: `Attrs` (attr `hx-get="/x" hx-trigger="change" hx-target="#y"`).
- Daftar pilihan yang muncul saat dropdown dibuka digambar oleh browser/OS, jadi tampilannya tidak bisa diubah penuh lewat CSS.

### DB card

```html
{{template `db-card` (dict `Name` `inventory_db` `Meta` `PostgreSQL · 1 schema · 7 tabel` `Access` `edit` `Href` `/db/inventory_db`)}}
{{template `db-card` (dict `Name` `hr_db` `Status` `failed` `Error` `Tidak dapat terhubung.` `RetryHref` `/db/hr_db/retry`)}}
```

Icon database selalu tampil di atas nama: `--color-primary-500` saat terhubung, `--color-error-500` saat gagal
terhubung. Nama database berwarna hitam (`--color-black`, putih di dark theme). `Status` `failed` mengganti tombol
"Buka" menjadi "Coba lagi".

### Table (kolom dan baris dari backend)

Di Go (`table.go`):

```go
tbl := web.Table{
    Title: "Daftar Tabel",
    Columns: []web.TableColumn{
        {Key: "name", Label: "Nama", Bold: true},
        {Key: "conn", Label: "Koneksi"},
    },
    Rows: []web.TableRow{
        {"name": {Text: "hr_db"}, "conn": {Text: "Terhubung", Variant: "success"}},
    },
}
```

Di template: `{{template `table` .Table}}`

- Sel `{Text: ...}` = teks biasa. `Variant` diisi = label berwarna. `Href` diisi = link.
- `Align`: `center` | `right` per kolom. Baris kosong menampilkan `Empty` (default "Belum ada data.").

### Action bar

```html
{{template `action-bar` (dict `Message` `2 perubahan akses belum disimpan` `ID` `access-bar` `Hidden` true
  `SubmitAttrs` (attr `form="access-form"`))}}
```

Melayang di bawah layar. Munculkan dengan melepas atribut `hidden` lewat JS/HTMX. Tambahkan `Class`
`action-bar--static` kalau tidak mau melayang.

---

## 4. Menambah komponen baru

1. **Template** `templates/components/<nama>.html`:
   - Mulai dengan komentar parameter (contoh: `button.html`).
   - `{{define "<nama>"}} ... {{end}}`. Untuk helper internal beri awalan nama komponen: `{{define "<nama>-class"}}`.
   - Gunakan `{{template `icon` .}}`, `{{template `button` ...}}`, `{{template `label` ...}}` kalau butuh.
2. **CSS** `static/css/components/<nama>.css` + `@import` di `app.css`. Ikuti konvensi di bagian 2.
3. **Tipe Go** (kalau data datang dari backend, seperti tabel): buat struct di file Go sendiri, dokumentasikan
   dengan contoh pemakaian di komentar.
4. **Contoh di halaman dev**: tambahkan section di `templates/dev/components.html` (dan data contoh di
   `devpage.go` bila perlu). Semua varian dan state harus terlihat di sana.
5. Cek `/dev/components` di mode terang, dan dark theme bila perlu.

## 5. Halaman uji komponen

Jalankan server lalu buka `http://localhost:8080/dev/components`.

Parameter query untuk menguji layout:

- `?nav=admin&active=users` sidebar admin (`active` = `users` | `groups` | `connections`)
- `?nav=database&active=table:public.orders` sidebar database
- `?nav=admin&active=users&admin=0` tampilan bukan platform admin

Route ini hanya boleh dipasang saat development.

## 6. Troubleshooting

| Gejala | Penyebab umum |
|---|---|
| Halaman terpotong lalu muncul teks "gagal render halaman" | Error saat eksekusi template, biasanya nama icon salah. Lihat log server: `render <halaman> gagal: ...` |
| Icon sangat besar | CSS icon belum termuat (cek `@import` di `app.css` dan nama file CSS-nya), atau komponen belum menyetel ukuran icon |
| Perubahan CSS tidak terlihat | Cache browser. Hard refresh (Ctrl+F5). File static di-embed saat build, restart server (atau pakai `air`) |
| Style di halaman dev beda dengan halaman asli | Aturan `.dev ...` di `components.html` ikut menimpa. Cek specificity |
| Atribut HTML berantakan setelah save | Formatter VS Code merusak string `"..."` di dalam tag. Pakai backtick |
| `<no value>` muncul di halaman | Parameter `dict` yang tidak diisi dipakai langsung. Bungkus dengan `{{with}}` atau `{{or}}` |

## 7. Catatan teknis yang perlu dirapikan

- Beberapa CSS memakai token palet langsung, bukan token semantik: `side-link.css` dan `action-bar.css`
  (`--color-primary-50`, `--color-primary-500`), `db-card.css` (`--color-primary-500`, `--color-error-500` untuk icon), `input.css` (`--color-neutral-400` untuk hover). Akibatnya warna ini
  tidak ikut berubah di dark theme. Idealnya dibuatkan token semantik di `themes.css`, misalnya
  `--color-primary-soft` (sudah ada) atau `--color-border-hover`.
- Di `themes.css`, `--color-text-muted` di dark theme dipetakan ke neutral-300, bukan neutral-500, supaya terbaca.

## 8. CSS khusus halaman

CSS yang hanya dipakai satu halaman ditaruh di `static/css/pages/<halaman>.css`. Ada dua cara memuatnya:

- lewat `@import "pages/<halaman>.css";` di `app.css` (dipakai `login.css`), atau
- lewat `<link rel="stylesheet" href="/static/css/pages/<halaman>.css">` di awal template halamannya
  (dipakai `databases.css`, jadi tidak bergantung pada `app.css`).