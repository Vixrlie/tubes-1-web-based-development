import { useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Alert, BatchProgress, Button, Card, Modal, SqlPreview } from '../../design-system'
import { paths } from '../../app/routes'
import { withMigrationSearch } from './migration-source'
import { useDrafts } from './useDrafts'
import { useWorkflowAction } from './useWorkflowAction'
import { jobBadgeStatus, latestJob, shortJobId } from './workflow-model'
import { WorkflowBadge, WorkflowDate, WorkflowHeading, WorkflowState } from './WorkflowUI'
import RecoveryActions from './RecoveryActions'
import OperationNotice from './OperationNotice'
import { operationLabels } from './operation-model'
import './workflow.css'

export default function JobDetailPage() {
  const { jobId = '' } = useParams()
  return <JobDetail key={jobId} jobId={jobId} />
}
function JobDetail({ jobId }: { jobId: string }) {
  const query = useDrafts()
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const [modal, setModal] = useState<'retry' | 'cancel' | null>(null)
  const { database } = query.workspace
  const row = query.data?.find(item => item.jobs?.some(job => job.id === jobId))
  const job = row?.jobs?.find(item => item.id === jobId)
  const url = (path: string) => withMigrationSearch(path, params)
  const mutation = useWorkflowAction(updated => { const next = latestJob(updated)!; setModal(null); navigate(url(next.status === 'canceled' ? paths.queue(database.id) : paths.job(database.id, next.id))) })
  const title = row?.intent ? `${operationLabels[row.intent.kind]} · ${job?.status === 'success' ? 'Selesai' : 'Progress'}` : job?.status === 'failed' && job.completedBatches > 0 ? 'Batch Terhenti · Recovery' : job?.status === 'failed' ? 'Job Eksekusi Gagal' : job?.status === 'success' ? 'Eksekusi Selesai' : job?.status === 'running' ? 'Progress Batch Migration' : job?.status === 'canceled' ? 'Job Dibatalkan' : 'Migration Masuk Queue'
  const activeJobs = (query.data ?? []).flatMap(item => item.jobs ?? []).filter(item => item.status === 'queued' || item.status === 'running').sort((a, b) => a.scheduledAt.localeCompare(b.scheduledAt))
  return <div className="workflow-page"><WorkflowHeading title={title} database={database.name} connectionId={database.id}><Link className="ds-button ds-button--secondary ds-button--sm" to={url(paths.queue(database.id))}>Kembali ke Antrean</Link></WorkflowHeading>
    <WorkflowState pending={query.isPending} error={query.isError ? query.error : null} missing={!row || !job} retry={() => { void query.refetch() }}>{row && job && <>
      {job.status === 'success' && <Alert tone="success" title="Eksekusi selesai">Seluruh batch telah selesai diproses.</Alert>}
      {job.status === 'failed' && <Alert tone={jobBadgeStatus(job) === 'failed' ? 'error' : 'warning'} title={jobBadgeStatus(job) === 'partially_reverted' ? 'Partially reverted: pembatalan berhenti sebagian' : jobBadgeStatus(job) === 'partial' ? 'Partial: eksekusi berhenti sebagian' : 'Eksekusi gagal'}>{job.error}</Alert>}
      <OperationNotice intent={row.intent} />
      {job.status === 'canceled' && <Alert title="Job dibatalkan">Job ini belum dijalankan dan dapat dimasukkan kembali ke antrean.</Alert>}
      <div className="workflow-columns"><Card className="workflow-panel"><h2>{job.status === 'queued' ? 'Execution Timeline' : 'Checkpoint per Batch'}</h2><p>{row.version} · {row.name} · {database.name}</p>
        <BatchProgress value={job.completedBatches} total={job.totalBatches} detail={job.status === 'queued' ? 'Menunggu giliran eksekusi.' : `${job.completedBatches.toLocaleString('id-ID')} batch ${row.intent?.kind === 'compensation' ? 'dikembalikan' : 'selesai'}.`} />
        <ol className="workflow-timeline">{[
          { label: 'Persetujuan seluruh grup', done: true, detail: 'Semua grup menyetujui SQL pada revisi ini.' },
          { label: `Job masuk antrean · ${shortJobId(job.id)}`, done: true, detail: <WorkflowDate value={job.queuedAt} /> },
          { label: 'Menunggu giliran → Berjalan', done: !!job.startedAt, detail: job.startedAt ? <WorkflowDate value={job.startedAt} /> : 'Menunggu giliran pada database ini.' },
          { label: job.status === 'failed' ? 'Eksekusi gagal' : job.status === 'canceled' ? 'Job dibatalkan' : 'Eksekusi selesai', done: !!job.finishedAt, detail: job.finishedAt ? <WorkflowDate value={job.finishedAt} /> : 'Hasil akan muncul setelah eksekusi selesai.' },
        ].map((event, index) => <li key={index}><span aria-hidden="true">{event.done ? '✓' : '○'}</span><div><strong>{event.label}</strong><small>{event.detail}</small></div></li>)}</ol>
        {job.completedBatches > 0 && <div className="workflow-list" aria-label="Checkpoint batch terakhir">{Array.from({ length: Math.min(3, job.completedBatches) }, (_, index) => row.intent?.kind === 'compensation' ? job.totalBatches - job.completedBatches + index + 1 : job.completedBatches - index).map(batch => <div className="workflow-batch" key={batch}><strong>{row.intent?.kind === 'compensation' ? 'Reverse' : 'Batch'} #{String(batch).padStart(3, '0')}</strong><WorkflowBadge status="success" /></div>)}</div>}
        {job.status === 'running' && <p role="status">Batch aktif #{String(row.intent?.kind === 'compensation' ? job.totalBatches - job.completedBatches : job.completedBatches + 1).padStart(3, '0')} · Berjalan</p>}
        <SqlPreview value={row.sqlUp} label={row.intent?.kind === 'rollback' || row.intent?.kind === 'compensation' ? 'SQL SNAPSHOT · DOWN ASLI' : 'SQL SNAPSHOT · UP ASLI'} />
      </Card><Card className="workflow-panel"><h2>Detail Job</h2><p>{shortJobId(job.id)} · {database.name}</p><WorkflowBadge status={jobBadgeStatus(job)} /><dl className="workflow-metadata"><div><dt>Diajukan oleh</dt><dd>{job.actor}</dd></div><div><dt>Percobaan</dt><dd>{job.attempt}</dd></div><div><dt>Batch selesai</dt><dd>{job.completedBatches} dari {job.totalBatches}</dd></div>{job.status === 'queued' && <div><dt>Posisi job</dt><dd>{activeJobs.findIndex(item => item.id === job.id) + 1}</dd></div>}<div><dt>Antrean sejak</dt><dd><WorkflowDate value={job.queuedAt} /></dd></div></dl>
        {(job.status === 'success' || job.status === 'failed') && <Link className="ds-button ds-button--secondary ds-button--md" to={url(paths.migration(database.id, job.executionId))}>Lihat Hasil Eksekusi</Link>}
        <Link to={url(paths.review(database.id, row.id))}>Lihat Persetujuan</Link>
        {database.permission === 'edit' && latestJob(row)?.id === job.id && (job.status === 'queued' || (job.status === 'failed' && job.completedBatches === 0)) && <Button variant="secondary" onClick={() => { mutation.reset(); setModal(job.status === 'failed' ? 'retry' : 'cancel') }}>{job.status === 'failed' ? 'Ajukan Retry' : 'Batalkan Antrean'}</Button>}
      </Card></div>
      {(job.status === 'success' || job.status === 'failed') && <RecoveryActions executionId={job.executionId} />}
      <Modal open={!!modal} title={modal === 'retry' ? 'Ajukan retry?' : 'Batalkan antrean?'} onClose={() => { if (!mutation.isPending) setModal(null) }} footer={<><Button variant="secondary" disabled={mutation.isPending} onClick={() => setModal(null)}>Kembali</Button><Button variant={modal === 'cancel' ? 'destructive' : 'primary'} loading={mutation.isPending} onClick={() => { if (!modal || mutation.isPending) return; mutation.mutate({ draftId: row.id, action: modal === 'retry' ? { type: 'retry' } : { type: 'cancel', jobId: job.id } }) }}>{modal === 'retry' ? 'Ajukan Retry' : 'Batalkan Antrean'}</Button></>}><div className="workflow-modal-body"><p>{modal === 'retry' ? 'Job baru akan dibuat dengan SQL dan persetujuan yang sama. Hasil percobaan sebelumnya tetap tersimpan.' : 'Job yang masih menunggu akan dibatalkan. Persetujuan migrasi tetap tersimpan.'}</p>{mutation.isError && <Alert tone="error" title="Tindakan belum berhasil">{mutation.error.message}</Alert>}</div></Modal>
    </>}</WorkflowState>
  </div>
}
