import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { Alert, Badge, Button, Card, DataTable, Dropdown, EmptyState, Input, MetadataPanel, Modal, Pagination, Spinner } from '../../design-system'
import { activityStatus } from './monitoring-source'
import type { ActivityEvent, ActivityStatus } from './monitoring-source'
import { createActivityApi } from './activity-api'
import { useSession } from '../auth/context'
import { ApiError } from '../../shared/api/api-error'
import './monitoring.css'

const tone = { success: 'success', warning: 'warning', failed: 'error', started: 'info' } as const
const date = new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'medium', timeZone: 'Asia/Jakarta' })
export default function ActivityLogsPage() {
  const [params, setParams] = useSearchParams()
  const { state, session } = useSession()
  const source = useMemo(() => createActivityApi(session.api), [session])
  const filters = { search: params.get('search') ?? '', status: Object.hasOwn(activityStatus, params.get('status') ?? '') ? params.get('status')! : 'all', category: ['sql', 'http', 'user', 'admin'].includes(params.get('category') ?? '') ? params.get('category')! : 'all', from: params.get('from') ?? '', to: params.get('to') ?? '' }
  const rangeError = filters.from && filters.to && filters.from > filters.to ? 'Tanggal akhir harus sama atau setelah tanggal awal.' : ''
  const requested = Number(params.get('page'))
  const requestedPage = Number.isSafeInteger(requested) && requested > 0 && requested <= 1000000 ? requested : 1
  const query = useQuery({ queryKey: ['activity-logs', 'api', state.user?.id, filters, requestedPage], queryFn: ({ signal }) => source.list(filters, requestedPage, signal), enabled: !rangeError, retry: false })
  const rows = query.data?.items ?? []
  const total = query.data?.total ?? 0
  const page = query.data?.page ?? 1
  const selectedId = params.get('event')
  const detail = useQuery({ queryKey: ['activity-log', 'api', state.user?.id, selectedId], queryFn: ({ signal }) => source.get(selectedId!, signal), enabled: !!selectedId, retry: false })
  const selected = detail.data
  function update(key: string, value: string, resetPage = true) {
    setParams(previous => { const next = new URLSearchParams(previous); if (value) next.set(key, value); else next.delete(key); if (resetPage) next.delete('page'); return next }, { replace: key === 'search' })
  }
  function clear() { setParams(previous => { const next = new URLSearchParams(previous); for (const key of ['search', 'status', 'category', 'from', 'to', 'page']) next.delete(key); return next }) }
  const statusBadge = (status: ActivityStatus) => <Badge tone={tone[status]}>{activityStatus[status]}</Badge>
  return <section className="monitoring-page" aria-labelledby="activity-title">
    <header className="monitoring-heading"><div><p className="ds-eyebrow">ADMIN / MONITORING</p><h1 id="activity-title">Activity Logs</h1><p>Telusuri aktivitas dan hasil operasi platform. Tanpa filter tanggal, menampilkan 7 hari terakhir.</p></div><Button variant="secondary" disabled={!!rangeError} loading={query.isFetching} onClick={() => { void query.refetch() }}>Muat ulang</Button></header>
    <Card className="monitoring-panel"><form className="activity-filters" onSubmit={event => event.preventDefault()}>
      <Input type="search" maxLength={200} label="Cari aktivitas" placeholder="Aksi, user ID, request ID, trace ID…" value={filters.search} onChange={event => update('search', event.target.value)} />
      <Dropdown label="Status" value={filters.status} onChange={value => update('status', value === 'all' ? '' : value)} options={[{ value: 'all', label: 'Semua status' }, ...Object.entries(activityStatus).map(([value, label]) => ({ value, label }))]} />
      <Dropdown label="Kategori" value={filters.category} onChange={value => update('category', value === 'all' ? '' : value)} options={[{ value: 'all', label: 'Semua aktivitas' }, { value: 'sql', label: 'SQL' }, { value: 'http', label: 'HTTP' }, { value: 'user', label: 'Autentikasi' }, { value: 'admin', label: 'Administrasi' }]} />
      <Input type="date" label="Dari tanggal (WIB)" value={filters.from} onChange={event => update('from', event.target.value)} />
      <Input type="date" label="Sampai tanggal (WIB)" value={filters.to} error={rangeError || undefined} onChange={event => update('to', event.target.value)} />
      <Button variant="tertiary" onClick={clear}>Reset filter</Button>
    </form></Card>
    <Card className="monitoring-panel"><div className="monitoring-section-heading"><h2>Daftar Aktivitas</h2><span role="status">{total} aktivitas{query.data?.limited ? ' (batas hasil)' : ''}</span></div>
      {rangeError ? <Alert tone="warning" title="Rentang tanggal tidak valid">{rangeError}</Alert> : query.isPending ? <Spinner label="Memuat activity logs" /> : query.isError ? <div className="monitoring-stack"><Alert tone="error" title="Activity logs belum dapat dimuat">{query.error.message}</Alert><Button variant="secondary" loading={query.isFetching} onClick={() => { void query.refetch() }}>Coba lagi</Button></div> : !rows.length ? <EmptyState title="Belum ada aktivitas yang sesuai">Ubah filter atau lakukan aktivitas, lalu muat ulang setelah log terkumpul.</EmptyState> : <>
        {query.data?.limited && <Alert tone="warning" title="Hasil mencapai batas 5.000 aktivitas">Persempit tanggal atau pencarian untuk melihat aktivitas lainnya.</Alert>}
        <DataTable<ActivityEvent> caption="Daftar activity logs" rows={rows} rowKey={event => event.id} columns={[
          { key: 'time', label: 'Waktu (WIB)', render: event => <time dateTime={event.time}>{date.format(new Date(event.time))}</time> },
          { key: 'actor', label: 'User ID', render: event => event.userId && event.userId !== 'unattributed' ? event.userId : 'Tidak teridentifikasi' },
          { key: 'action', label: 'Aksi', render: event => <code>{event.action}</code> },
          { key: 'database', label: 'Database', render: event => event.database ?? '—' },
          { key: 'status', label: 'Status', render: event => statusBadge(event.status) },
          { key: 'request', label: 'Request ID', render: event => <code>{event.requestId}</code> },
          { key: 'detail', label: 'Tindakan', render: event => <Button variant="tertiary" size="sm" aria-label={`Detail ${event.action}, ${date.format(new Date(event.time))}`} onClick={() => update('event', event.id, false)}>Detail</Button> },
        ]} /><Pagination total={total} pageSize={query.data?.pageSize ?? 8} page={page} onChange={value => update('page', String(value), false)} />
      </>}
    </Card>
    <Modal open={!!selectedId} onClose={() => update('event', '', false)} title="Detail Aktivitas" footer={<Button variant="secondary" onClick={() => update('event', '', false)}>Tutup</Button>}>
      {detail.isPending ? <Spinner label="Memuat detail aktivitas" /> : detail.isError ? detail.error instanceof ApiError && detail.error.status === 404 ? <EmptyState title="Aktivitas tidak ditemukan">Log mungkin sudah tidak tersedia.</EmptyState> : <div className="monitoring-stack"><Alert tone="error" title="Detail aktivitas belum dapat dimuat">{detail.error.message}</Alert><Button variant="secondary" loading={detail.isFetching} onClick={() => { void detail.refetch() }}>Coba lagi</Button></div> : selected ? <div className="monitoring-stack"><MetadataPanel title={selected.action} items={[
        { label: 'Waktu (WIB)', value: date.format(new Date(selected.time)) }, { label: 'Status', value: statusBadge(selected.status) },
        { label: 'User ID', value: selected.userId || 'Tidak teridentifikasi' }, { label: 'Database', value: selected.database ?? '—' },
        { label: 'Migration ID', value: selected.migrationId ?? '—' }, { label: 'Request ID', value: <code>{selected.requestId}</code> },
        { label: 'Trace ID', value: <code>{selected.traceId ?? 'Tidak tersedia'}</code> }, { label: 'Span ID', value: <code>{selected.spanId ?? 'Tidak tersedia'}</code> },
      ]} /><MetadataPanel title="Detail operasi" items={Object.entries(selected.detail).map(([label, value]) => ({ label, value: <code>{value}</code> }))} /></div> : <EmptyState title="Aktivitas tidak ditemukan">Pilih aktivitas yang tersedia pada daftar.</EmptyState>}
    </Modal>
  </section>
}
