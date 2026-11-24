import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { Card, Dropdown, EmptyState, MigrationJobCard } from '../../design-system'
import { paths } from '../../app/routes'
import { withMigrationSearch } from './migration-source'
import { useDrafts } from './useDrafts'
import { shortJobId } from './workflow-model'
import { WorkflowHeading, WorkflowState } from './WorkflowUI'
import './workflow.css'

export default function QueuePage() {
  const query = useDrafts()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const { database } = query.workspace
  const status = ['queued', 'running', 'success', 'failed', 'canceled'].includes(params.get('queueStatus') ?? '') ? params.get('queueStatus')! : 'all'
  const priority = { running: 0, queued: 1, failed: 2, success: 3, canceled: 4 }
  const jobs = (query.data ?? []).flatMap(row => (row.jobs ?? []).map(job => ({ row, job }))).sort((a, b) => priority[a.job.status] - priority[b.job.status] || b.job.queuedAt.localeCompare(a.job.queuedAt))
  const rows = jobs.filter(({ job }) => status === 'all' || job.status === status)
  const url = (path: string) => withMigrationSearch(path, params)
  const active = jobs.find(({ job }) => job.status === 'running')
  const waiting = jobs.filter(({ job }) => job.status === 'queued')
  const finished = jobs.filter(({ job }) => job.finishedAt).sort((a, b) => b.job.finishedAt!.localeCompare(a.job.finishedAt!))[0]
  return <div className="workflow-page"><WorkflowHeading title="Antrean Eksekusi" database={database.name} connectionId={database.id}><Link className="ds-button ds-button--secondary ds-button--sm" to={url(paths.reviews(database.id))}>Persetujuan Migrasi</Link></WorkflowHeading>
    <WorkflowState pending={query.isPending} error={query.isError ? query.error : null} retry={() => { void query.refetch() }}><div className="workflow-columns"><Card className="workflow-panel"><div className="workflow-panel-heading"><div><h2>Daftar Job Migration</h2><p>{database.name} · PostgreSQL · {jobs.length} job</p></div><Dropdown hideLabel label="Filter status job" value={status} onChange={value => setParams(previous => { const next = new URLSearchParams(previous); if (value === 'all') next.delete('queueStatus'); else next.set('queueStatus', value); return next })} options={[{ value: 'all', label: 'Semua status' }, { value: 'queued', label: 'Antrean' }, { value: 'running', label: 'Berjalan' }, { value: 'success', label: 'Selesai' }, { value: 'failed', label: 'Gagal' }, { value: 'canceled', label: 'Dibatalkan' }]} /></div>
      {!rows.length ? <EmptyState title={jobs.length ? 'Tidak ada job yang sesuai' : 'Belum ada job eksekusi'} action={<Link to={url(paths.reviews(database.id))}>Buka Persetujuan Migrasi</Link>}>Migrasi yang sudah disetujui semua grup dapat dimasukkan ke antrean.</EmptyState> : <div className="workflow-job-list">{rows.map(({ row, job }) => <MigrationJobCard key={job.id} jobId={shortJobId(job.id)} name={`${row.version} · ${row.name}`} database={database.name} status={job.status} completedBatches={job.status === 'running' || job.status === 'success' || job.status === 'failed' ? job.completedBatches : undefined} totalBatches={job.totalBatches} detail={job.status === 'queued' ? 'Menunggu giliran eksekusi.' : job.status === 'failed' ? job.error : `Percobaan ${job.attempt}`} onOpen={() => navigate(url(paths.job(database.id, job.id)))} />)}</div>}
    </Card><Card className="workflow-panel"><h2>Ringkasan Antrean</h2><p>Status terbaru job migration.</p><div className="workflow-info"><strong>Job Aktif</strong><p>{active ? `${shortJobId(active.job.id)} · ${active.row.name}` : 'Tidak ada job berjalan.'}</p></div><div className="workflow-info"><strong>Menunggu Eksekusi</strong><p>{waiting.length} job menunggu giliran pada {database.name}.</p></div><div className="workflow-info"><strong>Job Terakhir</strong>{finished ? <Link to={url(paths.job(database.id, finished.job.id))}>{shortJobId(finished.job.id)} · {finished.row.name}</Link> : <p>Belum ada job selesai.</p>}</div><Link to={url(paths.migrations(database.id))}>Kembali ke Riwayat</Link></Card></div></WorkflowState>
  </div>
}
