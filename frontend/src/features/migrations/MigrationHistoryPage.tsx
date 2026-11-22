import { Link, useSearchParams } from 'react-router-dom'
import { Alert, Badge, Button, Choice, DataTable, Dropdown, EmptyState, Spinner } from '../../design-system'
import { paths } from '../../app/routes'
import { executionAction, executionStatus, filterExecutions, parseExecutionStatus, withMigrationSearch } from './migration-source'
import type { Execution } from './migration-source'
import { useMigrations } from './useMigrations'
import './migrations.css'

export default function MigrationHistoryPage() {
  const query = useMigrations()
  const [params, setParams] = useSearchParams()
  const { database } = query.workspace
  const hideRollback = params.get('hideRollback') === '1'
  const status = parseExecutionStatus(params.get('status'))
  const rows = filterExecutions(query.data ?? [], hideRollback, status)
  function filter(key: string, value: string) {
    setParams(previous => { const next = new URLSearchParams(previous); if (value) next.set(key, value); else next.delete(key); return next })
  }
  const detailPath = (row: Execution) => withMigrationSearch(paths.migration(database.id, row.id), params)
  return <div className="migration-page">
    <header className="schema-heading"><h1>Riwayat Migrasi</h1><p><Link to={paths.databases}>{database.name}</Link> › Riwayat Migrasi</p></header>
    <section className="migration-actions" aria-label="Aksi migrasi"><h2>Aksi Cepat</h2><p>Buat migrasi, buka review, atau pantau antrean.</p>
      <Button className="migration-new" disabled title={database.permission === 'view' ? 'Akses database hanya untuk melihat' : 'Pembuatan migrasi belum tersedia'}><span className="migration-plus" aria-hidden="true">＋</span>Migrasi Baru</Button>
      <nav aria-label="Menu terkait migrasi"><Button variant="secondary" disabled title="Review belum tersedia">Review</Button><Button variant="secondary" disabled title="Antrean belum tersedia">Antrean</Button><Button variant="secondary" disabled title="Schema Diff belum tersedia">Schema Diff</Button></nav>
    </section>
    <section className="migration-history" aria-labelledby="execution-list-title">
      <div className="migration-list-heading"><div><h2 id="execution-list-title">Daftar Eksekusi</h2><p>Catatan eksekusi terbaru, termasuk UP, ROLLBACK, dan FORWARD.</p></div>
        <div className="migration-filters"><Dropdown label="Filter status eksekusi" hideLabel value={status} options={[{ value: 'all', label: 'Semua status' }, { value: 'success', label: 'Sukses' }, { value: 'failed', label: 'Gagal' }]} onChange={value => filter('status', value === 'all' ? '' : value)} />
          <div className="migration-rollback-desktop"><Choice label="Sembunyikan pasangan rollback" checked={hideRollback} onChange={event => filter('hideRollback', event.target.checked ? '1' : '')} /></div>
        </div>
      </div>
      <div className="migration-rollback-mobile"><p>Filter riwayat · {hideRollback ? 'Pasangan rollback disembunyikan' : 'Semua jenis eksekusi'}</p><Button variant="secondary" aria-pressed={hideRollback} onClick={() => filter('hideRollback', hideRollback ? '' : '1')}>{hideRollback ? 'Tampilkan pasangan rollback' : 'Sembunyikan pasangan rollback'}</Button></div>
      {query.isPending ? <div className="schema-state"><Spinner label="Memuat riwayat migrasi" /><p>Memuat riwayat migrasi.</p></div>
        : query.isError ? <div className="schema-state"><Alert tone="error" title="Riwayat migrasi belum dapat dimuat">{query.error.message}</Alert><Button variant="secondary" loading={query.isFetching} onClick={() => { void query.refetch() }}>Coba lagi</Button></div>
        : !rows.length ? <EmptyState title={query.data?.length ? 'Tidak ada eksekusi yang sesuai' : 'Belum ada riwayat migrasi'} action={query.data?.length ? <Button variant="secondary" onClick={() => setParams(previous => { const next = new URLSearchParams(previous); next.delete('hideRollback'); next.delete('status'); return next })}>Reset filter</Button> : undefined}>{query.data?.length ? 'Ubah filter untuk melihat eksekusi lainnya.' : `Eksekusi migrasi pada ${database.name} akan muncul di sini.`}</EmptyState>
        : <><p className="sr-only" role="status">{rows.length} eksekusi ditampilkan</p><div className="migration-execution-cards">{rows.map(row => <Link key={row.id} to={detailPath(row)} className="migration-execution-card"><span><strong>{row.id} · {row.version} · {row.name}</strong><span>{executionAction(row)} · {row.actor} · {row.executedAt} · {row.irreversible ? 'Irreversible' : executionStatus(row.status)}</span></span><span aria-hidden="true">›</span></Link>)}</div>
          <div className="migration-execution-table"><DataTable rows={rows} rowKey={row => row.id} caption="Daftar eksekusi migrasi" columns={[
            { key: 'id', label: 'Execution ID', render: row => <strong>{row.id}</strong> },
            { key: 'version', label: 'Versi', render: row => row.version },
            { key: 'migration', label: 'Nama Migrasi', render: row => row.name },
            { key: 'actor', label: 'Actor', render: row => row.actor },
            { key: 'time', label: 'Executed at', render: row => <time dateTime={row.executedAt.replace(' ', 'T')}>{row.executedAt}</time> },
            { key: 'status', label: 'Status', render: row => <Badge tone={row.status === 'success' ? 'success' : 'error'}>{executionStatus(row.status)}</Badge> },
            { key: 'action', label: 'Tindakan', render: row => <Link className="ds-button ds-button--secondary ds-button--sm" aria-label={`Lihat detail ${row.id}`} to={detailPath(row)}>Lihat Detail</Link> },
          ]} /></div></>}
    </section>
  </div>
}
