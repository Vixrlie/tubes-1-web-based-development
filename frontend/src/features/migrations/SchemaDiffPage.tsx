import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { Alert, Badge, Button, Card, Choice, Dropdown, EmptyState, Modal, Spinner, SqlPreview } from '../../design-system'
import { paths } from '../../app/routes'
import { createMockDatabaseSource, databasePreview } from '../databases/mock-source'
import { useDatabaseWorkspace } from '../databases/useDatabaseWorkspace'
import { withMigrationSearch } from './migration-source'
import { useDrafts } from './useDrafts'
import { useSession } from '../auth/context'
import { comparisonDraft, createMockSchemaTools } from './schema-tools-source'
import { createApiSchemaTools } from './api-schema-tools'
import { createApiDatabaseSource } from '../databases/api-source'
import { usesMockData } from '../../shared/api/data-mode'
import type { SchemaComparison } from './schema-tools-source'
import { WorkflowHeading, WorkflowState } from './WorkflowUI'
import './schema-tools.css'

export default function SchemaDiffPage() {
  const { workspace, preview } = useDatabaseWorkspace()
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const referenceId = params.get('reference') ?? ''
  const mode = import.meta.env.DEV ? databasePreview(params.get('toolsPreview')) : 'ready'
  const { session } = useSession()
  const mock = usesMockData(params)
  const source = useMemo(() => mock ? createMockSchemaTools(preview, mode) : createApiSchemaTools(session.api), [mock, preview, mode, session])
  const databaseSource = useMemo(() => mock ? createMockDatabaseSource() : createApiDatabaseSource(session.api), [mock, session])
  const databases = useQuery({ queryKey: ['diff-connections', mock ? 'mock' : 'api'], queryFn: ({ signal }) => databaseSource.list(signal), retry: false })
  const comparison = useQuery({ queryKey: ['schema-diff', mock ? 'mock' : 'api', workspace.database.id, referenceId, preview, mode], queryFn: ({ signal }) => source.compare(workspace.database.id, referenceId, signal), enabled: !!referenceId && mode !== 'loading', retry: false })
  const options = (databases.data ?? []).map(database => ({ value: database.id, label: database.name, disabled: database.status !== 'connected' }))
  return <div className="workflow-page"><WorkflowHeading title="Preview Schema Diff" database={workspace.database.name} connectionId={workspace.database.id}><Link className="ds-button ds-button--secondary ds-button--sm" to={withMigrationSearch(paths.migrations(workspace.database.id), params)}>Kembali ke Migrasi</Link></WorkflowHeading>
    <Card className="workflow-panel"><h2>Bandingkan Koneksi</h2><p>Database target akan disesuaikan dengan struktur database pembanding.</p>
      <WorkflowState pending={databases.isPending} error={databases.isError ? databases.error : null} retry={() => { void databases.refetch() }}><div className="schema-tools-selectors"><Dropdown label="Database target" value={workspace.database.id} options={options} onChange={id => { const next = new URLSearchParams(params); if (next.get('reference') === id) next.delete('reference'); navigate(`${paths.schemaDiff(id)}?${next}`) }} /><Dropdown label="Database pembanding" placeholder="Pilih database pembanding" value={referenceId} options={options.filter(option => option.value !== workspace.database.id)} onChange={value => setParams(previous => { const next = new URLSearchParams(previous); next.set('reference', value); return next })} /><Button variant="secondary" disabled={!referenceId} loading={comparison.isFetching} onClick={() => { void comparison.refetch() }}>Bandingkan Ulang</Button></div></WorkflowState>
    </Card>
    {!referenceId ? <Card><EmptyState title="Pilih database pembanding">Pilih dua koneksi untuk melihat perbedaan tabel dan kolom.</EmptyState></Card> : comparison.isPending ? <Card><Spinner label="Membandingkan schema" /></Card> : comparison.isError ? <Card className="schema-state"><Alert tone="error" title="Schema belum dapat dibandingkan">{comparison.error.message}</Alert><Button variant="secondary" onClick={() => { void comparison.refetch() }}>Coba lagi</Button></Card> : comparison.data && <DiffResult key={`${workspace.database.id}/${referenceId}/${mode}`} comparison={comparison.data} />}
  </div>
}
function DiffResult({ comparison }: { comparison: SchemaComparison }) {
  const query = useDrafts()
  const { session } = useSession()
  const client = useQueryClient()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [selected, setSelected] = useState(() => comparison.changes.filter(change => change.supported && !change.destructive).map(change => change.id))
  const [confirm, setConfirm] = useState(false)
  const abort = useRef<AbortController | null>(null)
  useEffect(() => () => abort.current?.abort(), [])
  let generated: ReturnType<typeof comparisonDraft> | undefined; let generationError = ''
  try { if (selected.length) generated = comparisonDraft(comparison, selected) } catch (error) { generationError = (error as Error).message }
  const save = useMutation({ mutationFn: async () => { if (!generated) throw new Error('Pilih perubahan terlebih dahulu.'); abort.current = new AbortController(); return query.source.save(comparison.target.id, generated, abort.current.signal) }, onSuccess: row => { if (session.getSnapshot().status !== 'authenticated' || session.getSnapshot().user?.id !== row.ownerId) return; void client.invalidateQueries({ queryKey: ['migration-drafts'] }); navigate(withMigrationSearch(paths.migrationDraft(row.connectionId, row.id), params), { state: { draftSaved: row.id } }) } })
  return <>
    <Card className="workflow-panel"><h2>Perbandingan Struktur Schema</h2><p>{comparison.changes.length} perbedaan · {selected.length} dipilih</p>
      {!comparison.changes.length ? <EmptyState title="Struktur schema sama">Tidak ada perubahan tabel atau kolom untuk dibuat menjadi draft.</EmptyState> : <div className="schema-change-list">{comparison.changes.map(change => <div className="schema-change" key={change.id}><div className="schema-change-heading"><Choice label={`${change.table}${change.column ? ` · ${change.column}` : ''}`} checked={selected.includes(change.id)} disabled={!change.supported || save.isPending} onChange={event => setSelected(previous => event.target.checked ? [...previous, change.id] : previous.filter(id => id !== change.id))} /><Badge tone={change.destructive ? 'error' : 'info'}>{change.kind.startsWith('table') ? 'Tabel' : 'Kolom'}</Badge></div><div className="schema-before-after"><div><small>BEFORE · {comparison.target.name}</small><p>{change.before}</p></div><div><small>AFTER · {comparison.reference.name}</small><p>{change.after}</p></div></div>{change.destructive && <small className="schema-change-risk">Berpotensi menghapus atau mengubah data.</small>}{!change.supported && <small>Perubahan ini memerlukan penyesuaian constraint atau backfill secara manual.</small>}</div>)}</div>}
    </Card>
    {generationError && <Alert tone="warning" title="Pilihan belum dapat dijadikan draft">{generationError}</Alert>}
    {generated && <Card className="workflow-panel"><h2>SQL Up · SQL Down · Reversibility</h2><SqlPreview value={generated.sqlUp} label="SQL UP" />{generated.irreversible ? <Alert tone="warning" title="Draft irreversible">Pilihan ini berpotensi menghapus atau mengubah data. SQL Down tidak dibuat otomatis.</Alert> : <SqlPreview value={generated.sqlDown} label="SQL DOWN · ROLLBACK" />}{save.isError && <Alert tone="error" title="Draft belum tersimpan">{save.error.message}</Alert>}<div className="workflow-actions"><Button disabled={query.workspace.database.permission !== 'edit'} loading={save.isPending} onClick={() => generated.irreversible ? setConfirm(true) : save.mutate()}>Jadikan Draft Migrasi</Button></div></Card>}
    <Modal open={confirm} title="Buat draft irreversible?" onClose={() => { if (!save.isPending) setConfirm(false) }} footer={<><Button variant="secondary" disabled={save.isPending} onClick={() => setConfirm(false)}>Batal</Button><Button variant="destructive" loading={save.isPending} onClick={() => { if (!save.isPending) save.mutate() }}>Buat Draft</Button></>}><div className="schema-state"><p>Perubahan yang dipilih berpotensi menghapus data. Periksa SQL sebelum mengajukan review.</p>{save.isError && <Alert tone="error" title="Draft belum tersimpan">{save.error.message}</Alert>}</div></Modal>
  </>
}
