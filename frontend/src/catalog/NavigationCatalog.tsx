import { useState } from 'react'
import type { ReactNode } from 'react'
import {
  AdminSidebar, ApprovalGroupCard, Badge, Button, Card, DatabaseCard, DatabaseSidebar,
  DataTable, EmptyState, KpiCard, ListToolbar, MetadataPanel, MigrationJobCard, MobileContextMenu, Modal,
  Pagination, ProductBottomNavigation, ProductHeader, ProfilePopup, TableCard,
} from '../design-system'
import { ChevronRightIcon } from '../design-system/control-icons'
import type { AdminDestination, Column, DatabaseSelection, ProductDestination } from '../design-system'

function Section({ title, description, children }: { title: string; description?: string; children: ReactNode }) {
  return <section className="catalog-section"><div className="section-heading"><h2>{title}</h2>{description && <p>{description}</p>}</div>{children}</section>
}

const tables = [
  { name: 'products', columns: 6, permission: 'View + Edit' },
  { name: 'users', columns: 3, permission: 'View only' },
  { name: 'orders', columns: 5, permission: 'View + Edit' },
  { name: 'payments', columns: 4, permission: 'View only' },
  { name: 'categories', columns: 3, permission: 'View + Edit' },
]
const adminTitles = { users: 'Pengguna', groups: 'Grup', connections: 'Koneksi Database', activity: 'Activity Logs', observability: 'Observability' }

export default function NavigationCatalog({ notify }: { notify: (message: string) => void }) {
  const [workspace, setWorkspace] = useState<ProductDestination>('database')
  const [admin, setAdmin] = useState<AdminDestination>('users')
  const [selection, setSelection] = useState<DatabaseSelection>({ kind: 'table', schemaId: 'public', tableId: 'products' })
  const [drawer, setDrawer] = useState(false)
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState('all')
  const [page, setPage] = useState(1)
  const [detail, setDetail] = useState<{ title: string; items: { label: string; value: ReactNode }[] } | null>(null)
  const filtered = tables.filter(table => table.name.includes(search.toLowerCase()) && (filter === 'all' || table.permission === filter))
  const currentTable = selection.kind === 'table' ? tables.find(table => table.name === selection.tableId) : undefined

  function openTable(table: typeof tables[number]) {
    setSelection({ kind: 'table', schemaId: 'public', tableId: table.name })
    setDetail({ title: table.name, items: [
      { label: 'Database', value: 'shop_db' }, { label: 'Schema', value: 'public' },
      { label: 'Jumlah kolom', value: table.columns }, { label: 'Primary key', value: 'id' }, { label: 'Akses', value: table.permission },
    ] })
  }

  const databaseSidebar = <DatabaseSidebar variant="schema" database="shop_db" schemas={[
    { id: 'public', name: 'public', tables: tables.map(table => ({ id: table.name, name: table.name })) },
    { id: 'archive', name: 'archive', tables: [] },
  ]} selection={selection} onSelect={value => { setSelection(value); setDrawer(false) }} />
  const adminSidebar = <AdminSidebar active={admin} onNavigate={value => { setAdmin(value); setDrawer(false) }} />
  const columns: Column<typeof tables[number]>[] = [
    { key: 'name', label: 'Nama', render: table => <strong>{table.name}</strong> },
    { key: 'columns', label: 'Kolom', render: table => table.columns },
    { key: 'pk', label: 'Primary Key', render: () => 'id' },
    { key: 'permission', label: 'Akses', render: table => <Badge>{table.permission}</Badge> },
    { key: 'action', label: 'Aksi', render: table => <Button variant="tertiary" size="sm" onClick={() => openTable(table)}>Buka</Button> },
  ]

  return <>
    <Section title="Header & sidebar produk" description="Pilih Database atau Admin untuk mencoba navigasi. Klik schema untuk membuka/tutup daftar tabel.">
      <div className="workspace-preview">
        <ProductHeader active={workspace} onNavigate={setWorkspace} onMenu={() => setDrawer(true)} menuOpen={drawer} profile={<ProfilePopup name="Arina Azka" email="arina@example.com" onLogout={() => notify('Contoh aksi logout dipilih')} />} />
        <div className="workspace-preview-body">
          {workspace !== 'home' && <aside className="workspace-preview-sidebar">{workspace === 'database' ? databaseSidebar : adminSidebar}</aside>}
          <div className="workspace-preview-content">
            <Badge tone="neutral">Preview komponen</Badge>
            <h3>{workspace === 'home' ? 'Beranda' : workspace === 'admin' ? adminTitles[admin] : selection.kind === 'migration' ? 'Migrasi' : currentTable?.name ?? 'Database'}</h3>
            <p className="ds-muted">{workspace === 'admin' ? 'Pilihan menu admin mengikuti sidebar.' : workspace === 'home' ? 'Pilih Database atau Admin untuk berpindah workspace.' : 'Pilihan tabel dan migrasi mengikuti sidebar database.'}</p>
            {workspace === 'database' && <MetadataPanel title="Pilihan aktif" items={[
              { label: 'Database', value: 'shop_db' },
              { label: 'Halaman', value: selection.kind === 'migration' ? 'Migrasi' : `public.${selection.tableId}` },
              ...(currentTable ? [{ label: 'Kolom', value: currentTable.columns }, { label: 'Akses', value: currentTable.permission }] : []),
            ]} />}
          </div>
        </div>
      </div>
      <Button variant="secondary" onClick={() => setDrawer(true)}>Buka navigasi mobile</Button>
      <MobileContextMenu open={drawer} onClose={() => setDrawer(false)} title={workspace === 'admin' ? 'Kelola platform' : 'shop_db'} description={workspace === 'admin' ? 'Pengguna · Grup · Koneksi' : `Schema public · ${tables.length} tabel`}>
        <nav className="ds-context-links" aria-label="Contoh navigasi mobile">{workspace === 'admin' ? (Object.keys(adminTitles) as AdminDestination[]).map(item => <button key={item} type="button" onClick={() => { setAdmin(item); setDrawer(false) }}>{adminTitles[item]}<ChevronRightIcon /></button>) : <>
          <button type="button" onClick={() => { setDrawer(false); notify('Schema public dipilih') }}>Schema public<ChevronRightIcon /></button>
          {tables.map(table => <button key={table.name} type="button" onClick={() => { setSelection({ kind: 'table', schemaId: 'public', tableId: table.name }); setDrawer(false) }}>{table.name}<ChevronRightIcon /></button>)}
          <button type="button" onClick={() => { setSelection({ kind: 'migration' }); setDrawer(false) }}>Migrasi<ChevronRightIcon /></button>
        </>}</nav>
      </MobileContextMenu>
    </Section>

    <Section title="Database cards" description="Contoh koneksi aktif, terputus, dan sedang diperiksa.">
      <div className="catalog-grid three">
        <DatabaseCard name="shop_db" description="PostgreSQL / 1 schema / 5 tabel" permission="View + Edit" onOpen={() => { setWorkspace('database'); notify('Database shop_db dipilih') }} />
        <DatabaseCard name="analytics_db" description="PostgreSQL / 2 schema / 12 tabel" permission="View only" connected={false} onOpen={() => {}} onReconnect={() => notify('Contoh aksi hubungkan kembali dipilih')} />
        <DatabaseCard name="staging_db" description="PostgreSQL / Memeriksa metadata" permission="View + Edit" loading onOpen={() => {}} />
      </div>
    </Section>

    <Section title="Table cards & toolbar" description="Kartu tabel seperti lampiran: nama, jumlah kolom, primary key, akses, dan chevron. Seluruh kartu bisa diklik.">
      <ListToolbar searchLabel="Cari tabel" search={search} onSearch={value => { setSearch(value); setPage(1) }} resultCount={filtered.length}
        filter={{ label: 'Hak akses', value: filter, onChange: value => { setFilter(value); setPage(1) }, options: [
          { value: 'all', label: 'Semua akses' }, { value: 'View + Edit', label: 'View + Edit' }, { value: 'View only', label: 'View only' },
        ] }} />
      <div className="catalog-preview"><div className="catalog-grid">{filtered.map(table => <TableCard key={table.name} name={table.name} columnCount={table.columns} primaryKey="id" permission={table.permission} onOpen={() => openTable(table)} />)}</div>
        {!filtered.length && <EmptyState title="Tabel tidak ditemukan">Ubah pencarian atau filter hak akses.</EmptyState>}
      </div>
      <Card><DataTable columns={columns} rows={filtered.slice((page - 1) * 3, page * 3)} rowKey={table => table.name} caption="Contoh tabel shop_db" /><Pagination page={page} total={filtered.length} pageSize={3} onChange={setPage} /></Card>
    </Section>

    <Section title="KPI & metadata panel"><div className="catalog-grid three">
      <KpiCard label="Database terhubung" value={12} detail="Seluruh koneksi aktif" />
      <KpiCard label="Migrasi berhasil" value="98%" detail="30 hari terakhir" />
      <MetadataPanel title="Database" items={[{ label: 'Nama', value: 'shop_db' }, { label: 'Engine', value: 'PostgreSQL' }, { label: 'Koneksi', value: <Badge tone="success">Terhubung</Badge> }]} />
    </div></Section>

    <Section title="Migration job cards"><div className="catalog-grid">
      <MigrationJobCard jobId="JOB-1052" name="backfill_users" database="shop_db" status="running" completedBatches={42} totalBatches={80} detail="210.000 rows committed" onOpen={() => setDetail({ title: 'JOB-1052', items: [{ label: 'Migrasi', value: 'backfill_users' }, { label: 'Database', value: 'shop_db' }, { label: 'Status', value: 'Berjalan' }, { label: 'Batch', value: '42 / 80' }] })} />
      <MigrationJobCard jobId="JOB-1051" name="add_product_status" database="shop_db" status="success" completedBatches={80} totalBatches={80} detail="400.000 rows committed" onOpen={() => setDetail({ title: 'JOB-1051', items: [{ label: 'Migrasi', value: 'add_product_status' }, { label: 'Status', value: 'Selesai' }] })} />
      <MigrationJobCard jobId="JOB-1050" name="update_orders" database="shop_db" status="failed" detail="Koneksi terputus pada batch 12. Periksa detail sebelum melanjutkan." onOpen={() => setDetail({ title: 'JOB-1050', items: [{ label: 'Status', value: 'Gagal' }, { label: 'Batch terakhir', value: '12' }, { label: 'Pesan', value: 'Koneksi terputus' }] })} />
      <MigrationJobCard jobId="JOB-1053" name="archive_payments" database="shop_db" status="queued" detail="Menunggu giliran eksekusi." onOpen={() => setDetail({ title: 'JOB-1053', items: [{ label: 'Status', value: 'Antrean' }, { label: 'Database', value: 'shop_db' }] })} />
    </div></Section>

    <Section title="Approval group cards"><div className="catalog-grid">
      <ApprovalGroupCard name="Database Administrators" required={2} approvers={[{ id: '1', name: 'Arina Azka', status: 'approved' }, { id: '2', name: 'Database Reviewer', status: 'pending' }]} onOpen={() => setDetail({ title: 'Database Administrators', items: [{ label: 'Arina Azka', value: 'Disetujui' }, { label: 'Database Reviewer', value: 'Menunggu' }] })} />
      <ApprovalGroupCard name="Engineering" required={1} approvers={[{ id: '3', name: 'Engineering Reviewer', status: 'approved' }]} />
    </div></Section>

    <Section title="Navigasi bawah produk" description="Preview inline Beranda / Database / Admin. Komponen yang sama dipakai untuk navigasi aplikasi pada mobile.">
      <div className="catalog-preview"><ProductBottomNavigation inline active={workspace} onNavigate={setWorkspace} /></div>
      <small role="status">Pilihan aktif: {workspace === 'home' ? 'Beranda' : workspace === 'admin' ? 'Admin' : 'Database'}</small>
    </Section>

    <Modal open={detail !== null} onClose={() => setDetail(null)} title={detail?.title ?? 'Detail'} footer={<Button variant="secondary" onClick={() => setDetail(null)}>Tutup</Button>}>
      {detail && <MetadataPanel title="Ringkasan" items={detail.items} />}
    </Modal>
  </>
}
