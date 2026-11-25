import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { Alert, Button, Card, Choice, DataTable, EmptyState, SqlEditor, SqlPreview } from '../../design-system'
import { paths } from '../../app/routes'
import { databasePreview } from '../databases/mock-source'
import { useDatabaseWorkspace } from '../databases/useDatabaseWorkspace'
import { useDrafts } from './useDrafts'
import { withMigrationSearch } from './migration-source'
import { createMockSchemaTools } from './schema-tools-source'
import { createApiSchemaTools } from './api-schema-tools'
import { useSession } from '../auth/context'
import { usesMockData } from '../../shared/api/data-mode'
import { WorkflowHeading, WorkflowState } from './WorkflowUI'
import './schema-tools.css'

export default function SqlPreviewPage() {
  const { draftId, migrationId } = useParams()
  const id = draftId ?? migrationId
  const query = useDrafts()
  const draft = query.data?.find(row => row.id === id)
  return <WorkflowState pending={query.isPending} error={query.isError ? query.error : null} missing={!!id && !draft} retry={() => { void query.refetch() }}>{(!id || draft) && <PreviewContent key={id ?? query.workspace.database.id} sqlUp={draft?.sqlUp} sqlDown={draft?.sqlDown} name={draft?.name} draftId={draft?.id} />}</WorkflowState>
}
function PreviewContent({ sqlUp, sqlDown, name, draftId }: { sqlUp?: string; sqlDown?: string; name?: string; draftId?: string }) {
  const { workspace, preview } = useDatabaseWorkspace()
  const [params] = useSearchParams()
  const [sql, setSql] = useState(sqlUp ?? `SELECT * FROM public.${workspace.schemas[0]?.tables[0]?.name ?? 'products'};`)
  const [analyze, setAnalyze] = useState(false)
  const mode = import.meta.env.DEV ? databasePreview(params.get('toolsPreview')) : 'ready'
  const { session } = useSession()
  const mock = usesMockData(params)
  const source = useMemo(() => mock ? createMockSchemaTools(preview, mode) : createApiSchemaTools(session.api), [mock, preview, mode, session])
  const abort = useRef<AbortController | null>(null)
  useEffect(() => () => abort.current?.abort(), [])
  const plan = useMutation({ mutationFn: () => { abort.current = new AbortController(); return source.explain(workspace.database.id, sql, analyze, abort.current.signal) } })
  function change(value: string) { setSql(value); plan.reset() }
  return <div className="workflow-page"><WorkflowHeading title="EXPLAIN & Execution Plan" database={workspace.database.name} connectionId={workspace.database.id}>{draftId && <Link className="ds-button ds-button--secondary ds-button--sm" to={withMigrationSearch(paths.migrationDraft(workspace.database.id, draftId), params)}>Kembali ke Detail</Link>}</WorkflowHeading>
    {sqlUp && <Card className="workflow-panel"><h2>Preview SQL · {name}</h2><SqlPreview value={sqlUp} label="SQL UP" />{sqlDown && <SqlPreview value={sqlDown} label="SQL DOWN · ROLLBACK" />}</Card>}
    <Card className="workflow-panel"><h2>Analisa Rencana Eksekusi</h2><SqlEditor label="SQL UNTUK EXPLAIN" value={sql} onChange={change} readOnly={plan.isPending} /><Choice label="Dengan EXPLAIN ANALYZE" disabled={workspace.database.permission !== 'edit' || plan.isPending} checked={analyze} onChange={event => { setAnalyze(event.target.checked); plan.reset() }} />{plan.isError && <Alert tone="error" title="Execution plan belum tersedia">{plan.error.message}</Alert>}<div className="workflow-actions"><Button disabled={!sql.trim()} loading={plan.isPending} onClick={() => plan.mutate()}>Jalankan EXPLAIN</Button></div></Card>
    <Card className="workflow-panel"><h2>Hasil Execution Plan</h2>{!plan.data ? <EmptyState title="Belum ada execution plan">Jalankan EXPLAIN untuk melihat estimasi biaya dan jumlah baris.</EmptyState> : <><div className="schema-plan-metrics"><div><small>Estimasi biaya</small><strong>{plan.data.scan.startupCost.toFixed(2)}–{plan.data.scan.totalCost.toFixed(2)}</strong></div><div><small>Estimasi baris</small><strong>{plan.data.scan.rows.toLocaleString('id-ID')}</strong></div><div><small>Planning time</small><strong>{plan.data.planningMs} ms</strong></div>{plan.data.executionMs !== undefined && <div><small>Execution time</small><strong>{plan.data.executionMs} ms</strong></div>}</div><SqlPreview value={plan.data.text} label="PostgreSQL EXPLAIN" /><DataTable caption="Execution plan" rows={[plan.data.scan]} rowKey={row => row.relation} columns={[{ key: 'node', label: 'Node', render: row => row.node }, { key: 'relation', label: 'Relation', render: row => row.relation }, { key: 'cost', label: 'Total cost', render: row => row.totalCost }, { key: 'rows', label: 'Rows', render: row => row.rows }, { key: 'width', label: 'Width', render: row => row.width }]} /></>}</Card>
  </div>
}
