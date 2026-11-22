import { useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Alert, Button, Card, Dropdown, Modal, SqlPreview, Textarea } from '../../design-system'
import { paths } from '../../app/routes'
import { withMigrationSearch } from './migration-source'
import { isEditable, latestJob, latestReview } from './workflow-model'
import { useDrafts } from './useDrafts'
import { useWorkflowAction } from './useWorkflowAction'
import { WorkflowBadge, WorkflowDate, WorkflowHeading, WorkflowState } from './WorkflowUI'
import './workflow.css'

export default function ReviewDetailPage() {
  const { draftId = '' } = useParams()
  return <ReviewDetail key={draftId} draftId={draftId} />
}
function ReviewDetail({ draftId }: { draftId: string }) {
  const query = useDrafts()
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const [groupId, setGroupId] = useState('')
  const [reason, setReason] = useState('')
  const [attempted, setAttempted] = useState(false)
  const [modal, setModal] = useState<'approved' | 'rejected' | 'enqueue' | null>(null)
  const [feedback, setFeedback] = useState('')
  const [decisionTarget, setDecisionTarget] = useState<{ groupId: string; reviewId: string } | null>(null)
  const { database } = query.workspace
  const row = query.data?.find(item => item.id === draftId)
  const round = row && latestReview(row)
  const eligible = round?.groups.filter(group => group.status === 'pending' && query.source.reviewerGroupIds.includes(group.id)) ?? []
  const selected = eligible.find(group => group.id === groupId) ?? eligible[0]
  const url = (path: string) => withMigrationSearch(path, params)
  const mutation = useWorkflowAction(updated => { setModal(null); setReason(''); setAttempted(false); setFeedback('Keputusan review berhasil dicatat.'); if (updated.status === 'queued') navigate(url(paths.job(database.id, latestJob(updated)!.id))) })
  const reasonError = attempted && (!reason.trim() || reason.trim().length > 1000) ? 'Alasan wajib diisi, maksimal 1.000 karakter.' : undefined
  function decide(decision: 'approved' | 'rejected') { setAttempted(true); if (!reason.trim() || reason.trim().length > 1000 || !selected || !round) return; setDecisionTarget({ groupId: selected.id, reviewId: round.id }); mutation.reset(); setModal(decision) }
  function confirm() {
    if (!row || !round || !modal || mutation.isPending) return
    if (modal === 'enqueue') mutation.mutate({ draftId, action: { type: 'enqueue' } })
    else if (decisionTarget) mutation.mutate({ draftId, action: { type: 'decide', reviewId: decisionTarget.reviewId, groupId: decisionTarget.groupId, decision: modal, reason } })
  }
  return <div className="workflow-page"><WorkflowHeading title={`Review Migration ${row?.version ?? ''}`} database={database.name} connectionId={database.id}><Link className="ds-button ds-button--secondary ds-button--sm" to={url(paths.reviews(database.id))}>Daftar Review</Link></WorkflowHeading>
    <WorkflowState pending={query.isPending} error={query.isError ? query.error : null} missing={!row || !round} retry={() => { void query.refetch() }}>{row && round && <>
      {feedback && <Alert tone="success" title={feedback} />}
      {mutation.isError && !modal && <Alert tone="error" title="Tindakan belum berhasil">{mutation.error.message}</Alert>}
      <div className="workflow-columns"><Card className="workflow-panel"><h2>Ringkasan & Persetujuan</h2><p>{row.version} · {row.name} · {database.name}</p><SqlPreview label="SQL UP · PostgreSQL" value={row.sqlUp} />{row.irreversible ? <Alert tone="warning" title="Migrasi irreversible">Tidak tersedia SQL Down untuk rollback.</Alert> : <SqlPreview label="SQL DOWN · ROLLBACK" value={row.sqlDown} />}
        <div className="workflow-panel-heading"><h2>Approval per Group</h2><small>{round.groups.filter(group => group.status === 'approved').length} / {round.groups.length} disetujui</small></div>
        <div className="workflow-list">{round.groups.map(group => <div className="workflow-group" key={group.id}><div><strong>{group.name}</strong><small>{group.permission === 'view' ? 'View' : 'View + Edit'}{group.actor && ` · ${group.actor}`}</small></div><WorkflowBadge status={group.status === 'pending' ? 'pending_review' : group.status} />{group.reason && <p className="workflow-reason">{group.reason}</p>}{group.decidedAt && <small className="workflow-decision-date"><WorkflowDate value={group.decidedAt} /></small>}</div>)}</div>
        {row.reviews && row.reviews.length > 1 && <details className="workflow-previous"><summary>Riwayat revisi sebelumnya</summary>{row.reviews.slice(0, -1).map(previous => <div key={previous.id}><h3>Revisi {previous.revision}</h3>{previous.groups.filter(group => group.reason).map(group => <p key={group.id}><strong>{group.name}</strong> · {group.actor}: {group.reason}</p>)}</div>)}</details>}
      </Card><Card className="workflow-panel"><h2>Informasi Migration</h2><p>Database {database.name} · Revision {round.revision}</p><WorkflowBadge status={row.status} />
        <div className="workflow-info"><strong>Persetujuan lengkap</strong><p>Semua grup, termasuk grup View, wajib menyetujui. Penolakan menghentikan pengajuan ini.</p></div>
        {row.status === 'pending_review' && (selected ? <><Dropdown label="Group reviewer" value={selected.id} options={eligible.map(group => ({ value: group.id, label: group.name }))} onChange={value => { setGroupId(value); setReason(''); setAttempted(false) }} /><Textarea label="Alasan reviewer" placeholder="Jelaskan hasil pemeriksaan SQL dan perubahan schema..." required maxLength={1000} value={reason} onChange={event => { setReason(event.target.value); setFeedback('') }} error={reasonError} /><div className="workflow-actions"><Button variant="secondary" disabled={mutation.isPending} onClick={() => decide('rejected')}>Tolak</Button><Button disabled={mutation.isPending} onClick={() => decide('approved')}>Setujui Migration</Button></div></> : <Alert title="Menunggu reviewer lain">Keputusan untuk grup yang dapat Anda tinjau sudah dicatat.</Alert>)}
        {row.status === 'approved' && <><Alert tone="success" title="Semua grup telah menyetujui">Migrasi siap dimasukkan ke antrean.</Alert>{database.permission === 'edit' && <Button onClick={() => { mutation.reset(); setModal('enqueue') }}>Masukkan ke Antrean</Button>}</>}
        {row.status === 'rejected' && <Alert tone="error" title="Pengajuan ditolak">Perbaiki draft lalu ajukan review ulang. Persetujuan seluruh grup akan dimulai kembali.</Alert>}
        {isEditable(row) && database.permission === 'edit' && <Link className="ds-button ds-button--secondary ds-button--md" to={url(paths.editMigrationDraft(database.id, row.id))}>Edit Draft</Link>}
        {latestJob(row) && <Link className="ds-button ds-button--secondary ds-button--md" to={url(paths.job(database.id, latestJob(row)!.id))}>Lihat Job Terakhir</Link>}
      </Card></div>
      <Modal open={!!modal} onClose={() => { if (!mutation.isPending) setModal(null) }} title={modal === 'enqueue' ? 'Masukkan ke antrean?' : modal === 'approved' ? 'Konfirmasi persetujuan' : 'Konfirmasi penolakan'} footer={<><Button variant="secondary" disabled={mutation.isPending} onClick={() => setModal(null)}>Batal</Button><Button variant={modal === 'rejected' ? 'destructive' : 'primary'} loading={mutation.isPending} onClick={confirm}>{modal === 'enqueue' ? 'Masukkan ke Antrean' : modal === 'approved' ? 'Ya, Setujui' : 'Kirim Penolakan'}</Button></>}><div className="workflow-modal-body"><p>{modal === 'enqueue' ? `${row.version} · ${row.name} akan menunggu giliran eksekusi pada ${database.name}.` : `${selected?.name}: ${reason}`}</p>{modal !== 'enqueue' && <p>Keputusan dicatat untuk revisi {round.revision}. Persetujuan belum menjalankan migrasi.</p>}{mutation.isError && <Alert tone="error" title="Tindakan belum berhasil">{mutation.error.message}</Alert>}</div></Modal>
    </>}</WorkflowState>
  </div>
}
