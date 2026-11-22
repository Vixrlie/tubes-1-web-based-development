import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { Alert, Badge, Button, Card, DataTable, Dropdown, EmptyState, Input, MetadataPanel, Modal, Pagination, Spinner } from '../../design-system'
import { activityStatus, createMockActivitySource, filterActivity } from './monitoring-source'
import type { ActivityEvent, ActivityStatus } from './monitoring-source'
import './monitoring.css'

const tone = { success: 'success', warning: 'warning', failed: 'error', started: 'info' } as const
const date = new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'medium', timeZone: 'Asia/Jakarta' })
export default function ActivityLogsPage() {
  const [params, setParams] = useSearchParams()
  const preview = import.meta.env.DEV ? params.get('preview') ?? 'ready' : 'ready'
  const source = useMemo(() => createMockActivitySource(preview), [preview])
  const query = useQuery({ queryKey: ['activity-logs', 'mock', preview], queryFn: ({ signal }) => source.list(signal), enabled: preview !== 'loading', retry: false })
  const filters = { search: params.get('search') ?? '', status: Object.hasOwn(activityStatus, params.get('status') ?? '') ? params.get('status')! : 'all', category: ['sql', 'http', 'user', 'admin'].includes(params.get('category') ?? '') ? params.get('category')! : 'all', from: params.get('from') ?? '', to: params.get('to') ?? '' }
  const rangeError = filters.from && filters.to && filters.from > filters.to ? 'Tanggal akhir harus sama atau setelah tanggal awal.' : ''
  const rows = rangeError ? [] : filterActivity(query.data ?? [], filters)
  const pageSize = 8
  const requested = Number(params.get('page'))
  const page = Math.min(Math.max(1, Math.ceil(rows.length / pageSize)), Number.isSafeInteger(requested) && requested > 0 ? requested : 1)
  const selectedId = params.get('event')
  const selected = query.data?.find(event => event.id === selectedId)
  function update(key: string, value: string, resetPage = true) {
    setParams(previous => { const next = new URLSearchParams(previous); if (value) next.set(key, value); else next.delete(key); if (resetPage) next.delete('page'); return next }, { replace: key === 'search' })
  }
  function clear() { setParams(previous => { const next = new URLSearchParams(previous); for (const key of ['search', 'status', 'category', 'from', 'to', 'page']) next.delete(key); return next }) }
  const statusBadge = (status: ActivityStatus) => <Badge tone={tone[status]}>{activityStatus[status]}</Badge>
  return <section className="monitoring-page" aria-labelledby="activity-title">
    <header className="monitoring-heading"><div><p className="ds-eyebrow">ADMIN / MONITORING</p><h1 id="activity-title">Activity Logs</h1><p>Telusuri aktivitas dan hasil operasi platform.</p></div><Button variant="secondary" loading={query.isFetching && preview !== 'loading'} onClick={() => { void query.refetch() }}>Muat ulang</Button></header>
    <Card className="monitoring-panel"><form className="activity-filters" onSubmit={event => event.preventDefault()}>
      <Input type="search" label="Cari aktivitas" placeholder="Aksi, user ID, request ID, trace ID…" value={filters.search} onChange={event => update('search', event.target.value)} />
      <Dropdown label="Status" value={filters.status} onChange={value => update('status', value === 'all' ? '' : value)} options={[{ value: 'all', label: 'Semua status' }, ...Object.entries(activityStatus).map(([value, label]) => ({ value, label }))]} />
      <Dropdown label="Kategori" value={filters.category} onChange={value => update('category', value === 'all' ? '' : value)} options={[{ value: 'all', label: 'Semua aktivitas' }, { value: 'sql', label: 'SQL' }, { value: 'http', label: 'HTTP' }, { value: 'user', label: 'Autentikasi' }, { value: 'admin', label: 'Administrasi' }]} />
      <Input type="date" label="Dari tanggal (WIB)" value={filters.from} onChange={event => update('from', event.target.value)} />
      <Input type="date" label="Sampai tanggal (WIB)" value={filters.to} error={rangeError || undefined} onChange={event => update('to', event.target.value)} />
      <Button variant="tertiary" onClick={clear}>Reset filter</Button>
    </form></Card>
    <Card className="monitoring-panel"><div className="monitoring-section-heading"><h2>Daftar Aktivitas</h2><span role="status">{rows.length} aktivitas</span></div>
      {query.isPending ? <Spinner label="Memuat activity logs" /> : query.isError ? <div className="monitoring-stack"><Alert tone="error" title="Activity logs belum dapat dimuat">{query.error.message}</Alert><Button variant="secondary" loading={query.isFetching} onClick={() => { void query.refetch() }}>Coba lagi</Button></div> : !rows.length ? <EmptyState title={query.data?.length ? 'Aktivitas tidak ditemukan' : 'Belum ada aktivitas'}>{rangeError || 'Aktivitas yang sesuai akan muncul di sini.'}</EmptyState> : <>
        <DataTable<ActivityEvent> caption="Daftar activity logs" rows={rows.slice((page - 1) * pageSize, page * pageSize)} rowKey={event => event.id} columns={[
          { key: 'time', label: 'Waktu (WIB)', render: event => <time dateTime={event.time}>{date.format(new Date(event.time))}</time> },
          { key: 'actor', label: 'User ID', render: event => event.userId && event.userId !== 'unattributed' ? event.userId : 'Tidak teridentifikasi' },
          { key: 'action', label: 'Aksi', render: event => <code>{event.action}</code> },
          { key: 'database', label: 'Database', render: event => event.database ?? '—' },
          { key: 'status', label: 'Status', render: event => statusBadge(event.status) },
          { key: 'request', label: 'Request ID', render: event => <code>{event.requestId}</code> },
          { key: 'detail', label: 'Tindakan', render: event => <Button variant="tertiary" size="sm" aria-label={`Detail ${event.action}, ${date.format(new Date(event.time))}`} onClick={() => update('event', event.id, false)}>Detail</Button> },
        ]} /><Pagination total={rows.length} pageSize={pageSize} page={page} onChange={value => update('page', String(value), false)} />
      </>}
    </Card>
    <Modal open={!!selectedId && !query.isPending && !query.isError} onClose={() => update('event', '', false)} title="Detail Aktivitas" footer={<Button variant="secondary" onClick={() => update('event', '', false)}>Tutup</Button>}>
      {selected ? <div className="monitoring-stack"><MetadataPanel title={selected.action} items={[
        { label: 'Waktu (WIB)', value: date.format(new Date(selected.time)) }, { label: 'Status', value: statusBadge(selected.status) },
        { label: 'User ID', value: selected.userId || 'Tidak teridentifikasi' }, { label: 'Database', value: selected.database ?? '—' },
        { label: 'Migration ID', value: selected.migrationId ?? '—' }, { label: 'Request ID', value: <code>{selected.requestId}</code> },
        { label: 'Trace ID', value: <code>{selected.traceId ?? 'Tidak tersedia'}</code> }, { label: 'Span ID', value: <code>{selected.spanId ?? 'Tidak tersedia'}</code> },
      ]} /><MetadataPanel title="Detail operasi" items={Object.entries(selected.detail).map(([label, value]) => ({ label, value: <code>{value}</code> }))} /></div> : <EmptyState title="Aktivitas tidak ditemukan">Pilih aktivitas yang tersedia pada daftar.</EmptyState>}
    </Modal>
  </section>
}
