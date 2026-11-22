import { Link, useLocation, useParams, useSearchParams } from 'react-router-dom'
import { Alert, Badge, Button, Card, EmptyState, Spinner, SqlPreview } from '../../design-system'
import { paths } from '../../app/routes'
import { withMigrationSearch } from './migration-source'
import { useDrafts } from './useDrafts'
import DraftSummary from './DraftSummary'
import './drafts.css'

export default function MigrationDraftPage() {
  const { draftId = '' } = useParams()
  const query = useDrafts()
  const [params] = useSearchParams()
  const location = useLocation()
  const { database } = query.workspace
  const back = withMigrationSearch(paths.migrations(database.id), params)
  if (query.isPending) return <Card className="schema-state"><Spinner label="Memuat draft" /></Card>
  if (query.isError) return <Card className="schema-state"><Alert tone="error" title="Draft belum dapat dimuat">{query.error.message}</Alert><Button variant="secondary" onClick={() => { void query.refetch() }}>Coba lagi</Button></Card>
  const draft = query.data?.find(row => row.id === draftId)
  if (!draft) return <Card><EmptyState title="Draft tidak ditemukan" action={<Link to={back}>Kembali ke Migrasi</Link>}>Pilih draft yang tersedia pada database ini.</EmptyState></Card>
  const saved = (location.state as { draftSaved?: string } | null)?.draftSaved === draft.id
  return <div className="draft-page"><header className="schema-heading"><h1>Draft · Menunggu Pengajuan</h1><p>{database.name} › <Link to={back}>Migrasi</Link> › Draft {draft.version}</p></header>
    {saved && <Alert tone="success" title="Draft berhasil disimpan" />}
    <div className="draft-workspace"><DraftSummary database={database.name} version={draft.version} author={draft.author} />
      <Card className="draft-detail"><div className="draft-detail-title"><h2>{draft.name}</h2><Badge tone="neutral">Draft</Badge></div>
        <p className="draft-updated">Terakhir disimpan: <time dateTime={draft.updatedAt}>{new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(draft.updatedAt))}</time></p>
        <SqlPreview value={draft.sqlUp} label="SQL UP" />
        {draft.irreversible ? <Alert tone="warning" title="Migrasi irreversible">Draft ini tidak menyediakan SQL Down untuk rollback.</Alert> : <SqlPreview value={draft.sqlDown} label="SQL DOWN · ROLLBACK" />}
        <div className="draft-actions"><Link className="ds-button ds-button--tertiary ds-button--md" to={back}>Kembali ke Migrasi</Link>{database.permission === 'edit' && <Link className="ds-button ds-button--secondary ds-button--md" to={withMigrationSearch(paths.editMigrationDraft(database.id, draft.id), params)}>Edit Draft</Link>}<Button disabled title="Pengajuan review belum tersedia">Kirim Review</Button></div>
      </Card>
    </div>
  </div>
}
