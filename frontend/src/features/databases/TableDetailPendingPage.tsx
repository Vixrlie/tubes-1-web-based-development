import { Link, useParams } from 'react-router-dom'
import { Card, EmptyState } from '../../design-system'
import { paths } from '../../app/routes'
import { selectSchema, withWorkspacePreview } from './schema-source'
import { useDatabaseWorkspace } from './useDatabaseWorkspace'

export default function TableDetailPendingPage() {
  const { schemaName, tableName } = useParams()
  const { workspace, preview } = useDatabaseWorkspace()
  const schema = selectSchema(workspace.schemas, schemaName)
  const table = schema?.tables.find(table => table.name === tableName)
  const back = withWorkspacePreview(`${paths.schema(workspace.database.id)}${schema ? `?schema=${encodeURIComponent(schema.name)}` : ''}`, preview)
  return <Card className="schema-pending-page">{table ? <><h1>Tabel {table.name}</h1><p>{workspace.database.name} › {schema?.name} › {table.name}</p><p>Detail tabel sedang disiapkan.</p><Link className="ds-button ds-button--secondary" to={back}>Kembali ke Schema</Link></> : <EmptyState title="Tabel tidak ditemukan" action={<Link to={back}>Kembali ke Schema</Link>}>Pilih tabel yang tersedia pada schema.</EmptyState>}</Card>
}
