import { useLocation, useMatch, useNavigate, useSearchParams } from 'react-router-dom'
import { DatabaseSidebar } from '../../design-system'
import { paths } from '../../app/routes'
import { selectSchema, withWorkspacePreview } from './schema-source'
import type { DatabaseWorkspace, SchemaPreview } from './schema-source'

export function WorkspaceSidebar({ workspace, preview, onNavigate }: { workspace: DatabaseWorkspace; preview: SchemaPreview; onNavigate?: () => void }) {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const [params] = useSearchParams()
  const tableMatch = useMatch('/databases/:connectionId/schemas/:schemaName/tables/:tableName')
  const activeSchema = pathname.includes('/migrations') || tableMatch ? undefined : selectSchema(workspace.schemas, params.get('schema'))?.name
  const go = (path: string) => { onNavigate?.(); navigate(withWorkspacePreview(path, preview)) }
  return <DatabaseSidebar database={workspace.database.name} variant="schema" activeSchema={activeSchema}
    schemas={workspace.schemas.map(schema => ({ id: schema.name, name: schema.name, tables: schema.tables.map(table => ({ id: table.name, name: table.name })) }))}
    selection={tableMatch?.params.tableName ? { kind: 'table', schemaId: tableMatch.params.schemaName ?? '', tableId: tableMatch.params.tableName } : pathname.includes('/migrations') ? { kind: 'migration' } : undefined}
    onSelectSchema={name => go(`${paths.schema(workspace.database.id)}?schema=${encodeURIComponent(name)}`)}
    onSelect={selection => go(selection.kind === 'migration' ? paths.migrations(workspace.database.id) : paths.table(workspace.database.id, selection.schemaId, selection.tableId))} />
}
