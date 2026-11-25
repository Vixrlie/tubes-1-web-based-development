import { useState } from 'react'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Alert, Button, Card, EmptyState, Modal, Spinner, SqlPreview } from '../../design-system'
import { paths } from '../../app/routes'
import { withMigrationSearch } from './migration-source'
import { useDrafts } from './useDrafts'
import DraftSummary from './DraftSummary'
import { isEditable, latestJob, latestReview, workflowStates } from './workflow-model'
import { WorkflowBadge } from './WorkflowUI'
import { useWorkflowAction } from './useWorkflowAction'
import OperationNotice from './OperationNotice'
import './drafts.css'

export default function MigrationDraftPage() {
  const { draftId = '' } = useParams()
  const query = useDrafts()
  const [params] = useSearchParams()
  const location = useLocation()
  const navigate = useNavigate()
  const [submitOpen, setSubmitOpen] = useState(false)
  const { database } = query.workspace
  const mutation = useWorkflowAction(updated => { setSubmitOpen(false); navigate(withMigrationSearch(paths.review(database.id, updated.id), params)) })
  const back = withMigrationSearch(paths.migrations(database.id), params)
  if (query.isPending) return <Card className="schema-state"><Spinner label="Memuat draft" /></Card>
  if (query.isError) return <Card className="schema-state"><Alert tone="error" title="Draft belum dapat dimuat">{query.error.message}</Alert><Button variant="secondary" onClick={() => { void query.refetch() }}>Coba lagi</Button></Card>
  const draft = query.data?.find(row => row.id === draftId)
  if (!draft) return <Card><EmptyState title="Draft tidak ditemukan" action={<Link to={back}>Kembali ke Migrasi</Link>}>Pilih draft yang tersedia pada database ini.</EmptyState></Card>
  const saved = (location.state as { draftSaved?: string } | null)?.draftSaved === draft.id
  return <div className="draft-page"><header className="schema-heading"><h1>{draft.status === 'draft' ? 'Draft · Menunggu Pengajuan' : `Migration ${draft.version} · ${workflowStates[draft.status].label}`}</h1><p>{database.name} › <Link to={back}>Migrasi</Link> › {draft.version}</p></header>
    {saved && <Alert tone="success" title="Draft berhasil disimpan" />}
    <div className="draft-workspace"><DraftSummary database={database.name} version={draft.version} author={draft.author} status={draft.status} />
      <Card className="draft-detail"><div className="draft-detail-title"><h2>{draft.name}</h2><WorkflowBadge status={draft.status} /></div>
        <p className="draft-updated">Terakhir disimpan: <time dateTime={draft.updatedAt}>{new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(draft.updatedAt))}</time></p>
        <OperationNotice intent={draft.intent} />
        <Link className="ds-button ds-button--secondary ds-button--sm" to={withMigrationSearch(paths.previewMigration(database.id, draft.id), params)}>Preview SQL & EXPLAIN</Link>
        <SqlPreview value={draft.sqlUp} label="SQL UP" />
        {draft.irreversible ? <Alert tone="warning" title="Migrasi irreversible">Draft ini tidak menyediakan SQL Down untuk rollback.</Alert> : <SqlPreview value={draft.sqlDown} label="SQL DOWN · ROLLBACK" />}
        {draft.status === 'rejected' && <Alert tone="error" title="Pengajuan ditolak">Perbaiki draft lalu ajukan review ulang.</Alert>}
        <div className="draft-actions"><Link className="ds-button ds-button--tertiary ds-button--md" to={back}>Kembali ke Migrasi</Link>{database.permission === 'edit' && isEditable(draft) && <Link className="ds-button ds-button--secondary ds-button--md" to={withMigrationSearch(paths.editMigrationDraft(database.id, draft.id), params)}>Edit Draft</Link>}{draft.status === 'draft' && database.permission === 'edit' && <Button onClick={() => { mutation.reset(); setSubmitOpen(true) }}>Ajukan Review</Button>}{latestReview(draft) && <Link className="ds-button ds-button--secondary ds-button--md" to={withMigrationSearch(paths.review(database.id, draft.id), params)}>Lihat Review</Link>}{latestJob(draft) && <Link className="ds-button ds-button--secondary ds-button--md" to={withMigrationSearch(paths.job(database.id, latestJob(draft)!.id), params)}>Lihat Job</Link>}</div>
      </Card>
    </div>
    <Modal open={submitOpen} title="Ajukan review migrasi?" onClose={() => { if (!mutation.isPending) setSubmitOpen(false) }} footer={<><Button variant="secondary" disabled={mutation.isPending} onClick={() => setSubmitOpen(false)}>Batal</Button><Button loading={mutation.isPending} onClick={() => { if (!mutation.isPending) mutation.mutate({ draftId: draft.id, action: { type: 'submit' } }) }}>Ajukan Review</Button></>}><div className="schema-state"><p>SQL Up dan SQL Down akan dikunci untuk ditinjau seluruh grup. Migrasi belum dijalankan.</p>{mutation.isError && <Alert tone="error" title="Review belum dapat diajukan">{mutation.error.message}</Alert>}</div></Modal>
  </div>
}
