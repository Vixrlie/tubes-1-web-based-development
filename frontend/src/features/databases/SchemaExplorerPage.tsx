import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { Badge, Card, DataTable, EmptyState, TableCard } from '../../design-system'
import { ChevronRightIcon } from '../../design-system/control-icons'
import { paths } from '../../app/routes'
import { selectSchema, withWorkspacePreview } from './schema-source'
import type { ForeignKey, SchemaTable } from './schema-source'
import { useDatabaseWorkspace } from './useDatabaseWorkspace'

export default function SchemaExplorerPage() {
  const { workspace, preview } = useDatabaseWorkspace()
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const schema = selectSchema(workspace.schemas, params.get('schema'))
  const { database } = workspace
  const url = (path: string) => withWorkspacePreview(path, preview)
  const tableUrl = (name: string, schemaName = schema?.name ?? 'public') => url(paths.table(database.id, schemaName, name))
  if (!schema) return <Card><EmptyState title={workspace.schemas.length ? 'Schema tidak ditemukan' : 'Belum ada schema'} action={<Link className="ds-button ds-button--secondary" to={workspace.schemas.length ? url(paths.schema(database.id)) : paths.databases}>{workspace.schemas.length ? 'Buka schema tersedia' : 'Kembali ke daftar database'}</Link>}>Pilih schema yang tersedia untuk melihat tabel.</EmptyState></Card>
  const permission = (table: SchemaTable) => table.permission === 'edit' ? 'View + Edit' : 'View Only'
  const relationTarget = (relation: ForeignKey) => tableUrl(relation.toTable, relation.toSchema)
  return <div className="schema-explorer"><header className="schema-heading"><h1>Schema {schema.name}</h1><p><Link to={paths.databases}>{database.name}</Link><span aria-hidden="true"> › </span>{schema.name}</p></header>
    <section className="schema-tables-section" aria-labelledby="schema-tables-title"><h2 id="schema-tables-title">Daftar Tabel</h2>
      {!schema.tables.length ? <EmptyState title="Belum ada tabel">Tabel pada schema ini akan muncul di sini.</EmptyState> : <>
        <div className="schema-mobile-rows">{schema.tables.map(table => <TableCard key={table.name} name={table.name} columnCount={table.columnCount} primaryKey={table.primaryKey.join(', ')} permission={permission(table)} onOpen={() => navigate(tableUrl(table.name))} />)}</div>
        <div className="schema-desktop-table"><DataTable caption={`Daftar tabel schema ${schema.name}`} rowKey={table => table.name} rows={schema.tables} columns={[
          { key: 'name', label: 'Nama', render: table => <Link className="schema-table-name" to={tableUrl(table.name)}>{table.name}</Link> },
          { key: 'columns', label: 'Kolom', render: table => table.columnCount },
          { key: 'primaryKey', label: 'Primary Key', render: table => table.primaryKey.join(', ') || '—' },
          { key: 'kind', label: 'Jenis', render: table => <Badge tone="success">{table.kind === 'table' ? 'Tabel' : 'View'}</Badge> },
          { key: 'permission', label: 'Akses', render: table => <Badge tone={table.permission === 'edit' ? 'info' : 'neutral'}>{permission(table)}</Badge> },
          { key: 'action', label: 'Tindakan', render: table => <Link className="schema-open-link" to={tableUrl(table.name)} aria-label={`Buka tabel ${table.name}`}>Buka</Link> },
        ]} /></div>
      </>}
    </section>
    <section className="schema-relations-section" aria-labelledby="schema-relations-title"><h2 id="schema-relations-title">Relasi (Foreign Key)</h2>
      {!schema.relations.length ? <EmptyState title="Belum ada relasi">Tidak ada foreign key pada schema ini.</EmptyState> : <>
        <div className="schema-mobile-rows">{schema.relations.map(relation => <Link key={relation.name} className="schema-relation-card" to={relationTarget(relation)}><span><strong>{relation.name}</strong><span>{relation.fromTable}.{relation.fromColumn} → {relation.toTable}.{relation.toColumn}</span></span><ChevronRightIcon /></Link>)}</div>
        <div className="schema-desktop-table"><DataTable caption="Relasi foreign key" rowKey={relation => relation.name} rows={schema.relations} columns={[
          { key: 'name', label: 'Nama', render: relation => relation.name },
          { key: 'from', label: 'Dari', render: relation => `${relation.fromTable}.${relation.fromColumn}` },
          { key: 'to', label: 'Mengacu ke', render: relation => <Link className="schema-reference" to={relationTarget(relation)}><Badge>{relation.toTable}.{relation.toColumn}</Badge></Link> },
        ]} /></div>
      </>}
    </section>
    <section className="schema-mobile-actions" aria-labelledby="schema-actions-title"><h2 id="schema-actions-title">Aksi Schema</h2><p>Kelola riwayat atau bandingkan struktur database.</p><Link className="ds-button ds-button--secondary" to={url(paths.migrations(database.id))}>Riwayat Migration</Link><Link className="schema-relation-card schema-compare-link" to={url(`${paths.migrations(database.id)}/diff`)}><span><strong>Bandingkan Schema</strong><span>Buka layar bandingkan schema</span></span><ChevronRightIcon /></Link></section>
  </div>
}
