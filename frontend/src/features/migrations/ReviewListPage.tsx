import { Link, useSearchParams } from 'react-router-dom'
import { Badge, Card, Dropdown, EmptyState } from '../../design-system'
import { paths } from '../../app/routes'
import { withMigrationSearch } from './migration-source'
import { latestReview } from './workflow-model'
import { useDrafts } from './useDrafts'
import { WorkflowBadge, WorkflowDate, WorkflowHeading, WorkflowState } from './WorkflowUI'
import './workflow.css'

export default function ReviewListPage() {
  const query = useDrafts()
  const [params, setParams] = useSearchParams()
  const { database } = query.workspace
  const status = ['pending_review', 'approved', 'rejected'].includes(params.get('reviewStatus') ?? '') ? params.get('reviewStatus')! : 'all'
  const reviews = (query.data ?? []).filter(row => row.status !== 'draft' && latestReview(row))
  const rows = reviews.filter(row => status === 'all' || (status === 'approved' ? !['pending_review', 'rejected'].includes(row.status) : row.status === status))
  const url = (path: string) => withMigrationSearch(path, params)
  return <div className="workflow-page"><WorkflowHeading title="Persetujuan Migrasi" database={database.name} connectionId={database.id}><Link className="ds-button ds-button--secondary ds-button--sm" to={url(paths.queue(database.id))}>Antrean Eksekusi</Link></WorkflowHeading>
    <WorkflowState pending={query.isPending} error={query.isError ? query.error : null} retry={() => { void query.refetch() }}><div className="workflow-columns">
      <Card className="workflow-panel"><div className="workflow-panel-heading"><div><h2>Daftar Permintaan Review</h2><p>{reviews.filter(row => row.status === 'pending_review').length} migrasi menunggu review.</p></div><Dropdown label="Filter status review" hideLabel value={status} onChange={value => setParams(previous => { const next = new URLSearchParams(previous); if (value === 'all') next.delete('reviewStatus'); else next.set('reviewStatus', value); return next })} options={[{ value: 'all', label: 'Semua status' }, { value: 'pending_review', label: 'Menunggu review' }, { value: 'approved', label: 'Disetujui' }, { value: 'rejected', label: 'Ditolak' }]} /></div>
        {!rows.length ? <EmptyState title={reviews.length ? 'Tidak ada review yang sesuai' : 'Belum ada permintaan review'} action={<Link to={url(paths.migrations(database.id))}>Kembali ke Migrasi</Link>}>Ajukan draft migrasi untuk memulai persetujuan.</EmptyState> : <div className="workflow-list">{rows.map(row => { const round = latestReview(row)!; return <Link className="workflow-list-row" key={row.id} to={url(paths.review(database.id, row.id))}><span><strong>{row.version} · {row.name}</strong><small>{database.name} · {row.author} · <WorkflowDate value={round.submittedAt} /></small></span><span className="workflow-row-status"><WorkflowBadge status={row.status} /><small>{round.groups.filter(group => group.status === 'approved').length} / {round.groups.length} disetujui</small><span>Lihat Detail →</span></span></Link> })}</div>}
      </Card>
      <Card className="workflow-panel"><h2>Panduan Review</h2><p>Periksa SQL Up dan SQL Down sebelum memberi keputusan.</p><div className="workflow-info"><strong>Approval per grup</strong><p>Semua leader grup View dan View + Edit wajib menyetujui.</p></div><div className="workflow-info"><strong>Persetujuan dan eksekusi</strong><p>Migrasi yang disetujui masih harus dimasukkan ke antrean eksekusi.</p></div><Badge tone="info">{database.name}</Badge></Card>
    </div></WorkflowState>
  </div>
}
