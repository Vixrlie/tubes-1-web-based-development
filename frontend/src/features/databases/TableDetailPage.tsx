import { useCallback, useId, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { Alert, Badge, Button, Card, EmptyState, Spinner, Tabs } from '../../design-system'
import { paths } from '../../app/routes'
import { databasePreview } from './mock-source'
import { selectSchema, withWorkspacePreview } from './schema-source'
import { createMockTableSource, parseTablePage, parseTableTab } from './table-source'
import type { TableIdentity } from './table-source'
import { useDatabaseWorkspace } from './useDatabaseWorkspace'
import { TableColumnsPanel, TableConstraintsPanel } from './TableStructurePanels'
import TableDataPanel from './TableDataPanel'
import './table-detail.css'

export default function TableDetailPage() {
  const { schemaName = '', tableName = '' } = useParams()
  const { workspace, preview } = useDatabaseWorkspace()
  const schema = selectSchema(workspace.schemas, schemaName)
  const table = schema?.tables.find(table => table.name === tableName)
  if (!schema || !table) return <Card><EmptyState title="Tabel tidak ditemukan" action={<Link to={withWorkspacePreview(paths.schema(workspace.database.id), preview)}>Kembali ke Schema</Link>}>Pilih tabel yang tersedia pada schema.</EmptyState></Card>
  return <TableDetailContent key={`${workspace.database.id}/${schemaName}/${tableName}`} identity={{ connectionId: workspace.database.id, schemaName, tableName }} />
}

function TableDetailContent({ identity }: { identity: TableIdentity }) {
  const { workspace, preview } = useDatabaseWorkspace()
  const [params, setParams] = useSearchParams()
  const tab = parseTableTab(params.get('tab'))
  const page = parseTablePage(params.get('page'))
  const direction = params.get('direction') === 'desc' ? 'desc' : 'asc'
  const tablePreview = import.meta.env.DEV ? databasePreview(params.get('tablePreview')) : 'ready'
  const source = useMemo(() => createMockTableSource({ preview: tablePreview, workspacePreview: preview }), [preview, tablePreview])
  const cacheKey = [identity.connectionId, identity.schemaName, identity.tableName, preview, tablePreview]
  const description = useQuery({ queryKey: ['table-description', ...cacheKey], queryFn: ({ signal }) => source.describe(identity, signal), enabled: tablePreview !== 'loading', retry: false })
  const id = useId()
  const onPage = useCallback((value: number, replace = false) => { setParams(previous => { const next = new URLSearchParams(previous); next.set('tab', 'data'); next.set('page', String(value)); return next }, { replace }) }, [setParams])
  function changeTab(value: string) { setParams(previous => { const next = new URLSearchParams(previous); next.set('tab', value); next.delete('page'); return next }) }
  function sort() { setParams(previous => { const next = new URLSearchParams(previous); next.set('direction', direction === 'asc' ? 'desc' : 'asc'); next.set('page', '1'); return next }) }
  return <div className="table-detail-page"><header className="schema-heading"><h1>Tabel {identity.tableName}</h1><p><Link to={paths.databases}>{workspace.database.name}</Link> › <Link to={withWorkspacePreview(`${paths.schema(identity.connectionId)}?schema=${encodeURIComponent(identity.schemaName)}`, preview)}>{identity.schemaName}</Link> › {identity.tableName}</p></header>
    {tab === 'columns' && <Card className="table-mobile-summary"><div><small>DATABASE</small><Badge tone="success">Terhubung</Badge></div><h2>Tabel {identity.tableName}</h2><p>{identity.schemaName}/{identity.tableName}</p></Card>}
    <Tabs idPrefix={id} label="Detail tabel" value={tab} onChange={changeTab} items={[{ value: 'columns', label: 'Column' }, { value: 'constraints', label: 'Constraint & Index' }, { value: 'data', label: 'Data' }]} />
    <section role="tabpanel" id={`${id}-panel-${tab}`} aria-labelledby={`${id}-tab-${tab}`} tabIndex={0} className="table-detail-panel">
      {description.isPending ? <Card className="schema-state"><Spinner label="Memuat detail tabel" /><p>Memuat detail tabel…</p></Card>
        : description.isError || !description.data ? <Card className="schema-state"><Alert tone="error" title="Detail tabel belum dapat dimuat">{description.error?.message}</Alert><Button variant="secondary" loading={description.isFetching} onClick={() => { void description.refetch() }}>Coba lagi</Button></Card>
        : tab === 'columns' ? <TableColumnsPanel description={description.data} identity={identity} preview={preview} />
        : tab === 'constraints' ? <TableConstraintsPanel description={description.data} />
        : <TableDataPanel source={source} identity={identity} description={description.data} page={page} direction={direction} cacheKey={cacheKey} loading={tablePreview === 'loading'} onPage={onPage} onSort={sort} />}
    </section>
  </div>
}
