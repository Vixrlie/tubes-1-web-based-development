import { Link } from 'react-router-dom'
import { Badge, Card, DataTable } from '../../design-system'
import { ChevronRightIcon } from '../../design-system/control-icons'
import { paths } from '../../app/routes'
import { withWorkspacePreview } from './schema-source'
import type { SchemaPreview } from './schema-source'
import type { TableDescription, TableIdentity } from './table-source'

export function TableColumnsPanel({ description, identity, preview }: { description: TableDescription; identity: TableIdentity; preview: SchemaPreview }) {
  return <><div className="table-detail-mobile table-column-list">{description.columns.map(column => <details className="table-detail-row" key={column.name}><summary><span><strong>{column.name}</strong><span>{column.dataType}{column.primaryKey ? ' · PK' : ''}{column.references ? ' · FK' : ''}</span></span><ChevronRightIcon /></summary><dl><div><dt>Nullable</dt><dd>{column.nullable ? 'YES' : 'NO'}</dd></div><div><dt>Default</dt><dd><code>{column.defaultExpression ?? '—'}</code></dd></div>{column.references && <div><dt>Foreign Key</dt><dd><Link to={withWorkspacePreview(paths.table(identity.connectionId, column.references.schema, column.references.table), preview)}>{column.references.schema}.{column.references.table}.{column.references.column}</Link></dd></div>}</dl></details>)}</div>
    <Card className="table-detail-desktop table-structure-card"><h2>Column</h2><DataTable caption="Struktur kolom tabel" rows={description.columns} rowKey={column => column.name} columns={[
      { key: 'name', label: 'Nama', render: column => <span className="table-column-name">{column.name}{column.primaryKey && <Badge>PK</Badge>}{column.references && <Link to={withWorkspacePreview(paths.table(identity.connectionId, column.references.schema, column.references.table), preview)}><Badge>FK</Badge></Link>}</span> },
      { key: 'type', label: 'Tipe', render: column => column.dataType },
      { key: 'nullable', label: 'Nullable', render: column => column.nullable ? 'YES' : 'NO' },
      { key: 'default', label: 'Default', render: column => <code>{column.defaultExpression ?? '—'}</code> },
    ]} /></Card></>
}

export function TableConstraintsPanel({ description }: { description: TableDescription }) {
  return <><div className="table-detail-mobile table-column-list">{[
    ...description.constraints.map(item => ({ key: `constraint-${item.name}`, name: item.name, detail: `Constraint · ${item.type} · ${item.columns.join(', ')}`, definition: item.definition })),
    ...description.indexes.map(item => ({ key: `index-${item.name}`, name: item.name, detail: `Index · ${item.columns.join(', ')}${item.unique ? ' · Unique' : ''}`, definition: item.definition })),
  ].map(item => <details key={item.key} className="table-detail-row"><summary><span><strong>{item.name}</strong><span>{item.detail}</span></span><ChevronRightIcon /></summary><pre>{item.definition}</pre></details>)}</div>
    <div className="table-detail-desktop table-constraint-grid"><Card className="table-structure-card"><h2>Constraint</h2><DataTable caption="Constraint tabel" rows={description.constraints} rowKey={item => item.name} columns={[
      { key: 'name', label: 'Nama', render: item => item.name },
      { key: 'type', label: 'Jenis', render: item => <Badge tone={item.type === 'PRIMARY KEY' ? 'info' : 'neutral'}>{item.type}</Badge> },
      { key: 'columns', label: 'Kolom', render: item => item.columns.join(', ') },
    ]} /></Card><Card className="table-structure-card"><h2>Index</h2><DataTable caption="Index tabel" rows={description.indexes} rowKey={item => item.name} columns={[
      { key: 'name', label: 'Nama', render: item => item.name },
      { key: 'columns', label: 'Kolom', render: item => item.columns.join(', ') },
      { key: 'unique', label: 'Unik', render: item => item.unique ? 'Ya' : 'Tidak' },
    ]} /></Card></div></>
}
