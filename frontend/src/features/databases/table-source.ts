import { waitForMock } from './mock-source.ts'
import type { DatabasePreview } from './mock-source.ts'
import { createMockSchemaSource, SchemaSourceError } from './schema-source.ts'
import type { SchemaPreview, SchemaTable } from './schema-source.ts'
import type { DatabasePermission } from './types.ts'

export type TableIdentity = { connectionId: string; schemaName: string; tableName: string }
export type TableColumn = { name: string; dataType: string; nullable: boolean; defaultExpression: string | null; primaryKey: boolean; references?: { schema: string; table: string; column: string } }
export type TableConstraint = { name: string; type: 'PRIMARY KEY' | 'UNIQUE' | 'FOREIGN KEY'; columns: string[]; definition: string }
export type TableIndex = { name: string; columns: string[]; unique: boolean; definition: string }
export type TableDescription = { columns: TableColumn[]; constraints: TableConstraint[]; indexes: TableIndex[]; permission: DatabasePermission }
export type TableCell = string | number | boolean | null
export type TableRecord = Record<string, TableCell>
export type TablePage = { columns: string[]; rows: TableRecord[]; total: number; page: number; pageSize: number; pages: number }
export type TableSource = {
  describe: (identity: TableIdentity, signal: AbortSignal) => Promise<TableDescription>
  rows: (request: TableIdentity & { page: number; direction: 'asc' | 'desc' }, signal: AbortSignal) => Promise<TablePage>
}
export const TABLE_PAGE_SIZE = 4
export type TableTab = 'columns' | 'constraints' | 'data'
export function parseTableTab(value: string | null): TableTab { return value === 'data' || value === 'constraints' ? value : 'columns' }
export function parseTablePage(value: string | null): number {
  if (!value || !/^[1-9]\d*$/.test(value)) return 1
  const number = Number(value)
  return Number.isSafeInteger(number) ? number : 1
}
export function formatTableCell(value: TableCell): string { return value === null ? 'NULL' : value === '' ? '""' : String(value) }

const definitions: Record<string, Record<string, [string, string, boolean?, string?][]>> = {
  'mock-shop': {
    users: [['id', 'integer'], ['email', 'varchar(255)'], ['created_at', 'timestamp', true, 'now()']],
    products: [['id', 'integer'], ['name', 'varchar(255)'], ['price', 'numeric(12,2)'], ['stock', 'integer', false, '0'], ['description', 'text', true], ['created_at', 'timestamp', false, 'now()']],
    orders: [['id', 'integer'], ['user_id', 'integer'], ['product_id', 'integer'], ['quantity', 'integer', false, '1'], ['created_at', 'timestamp', false, 'now()']],
  },
  'mock-inventory': {
    items: [['id', 'integer'], ['name', 'varchar(255)'], ['sku', 'varchar(50)'], ['price', 'numeric(12,2)'], ['description', 'text', true], ['created_at', 'timestamp', false, 'now()']],
    warehouses: [['id', 'integer'], ['name', 'varchar(255)'], ['location', 'text', true]],
    stock: [['id', 'integer'], ['item_id', 'integer'], ['warehouse_id', 'integer'], ['quantity', 'integer', false, '0'], ['updated_at', 'timestamp', false, 'now()']],
  },
  'mock-analytics': {
    events: [['id', 'integer'], ['type_id', 'integer'], ['user_id', 'integer'], ['payload', 'jsonb', true], ['created_at', 'timestamp', false, 'now()'], ['processed', 'boolean', false, 'false']],
    event_types: [['id', 'integer'], ['name', 'varchar(255)'], ['description', 'text', true]],
    users: [['id', 'integer'], ['username', 'varchar(255)'], ['email', 'varchar(255)'], ['is_active', 'boolean', false, 'true'], ['created_at', 'timestamp', false, 'now()']],
  },
}
const totals: Record<string, Record<string, number>> = {
  'mock-shop': { users: 1250, products: 27, orders: 18 },
  'mock-inventory': { items: 24, warehouses: 6, stock: 32 },
  'mock-analytics': { events: 101, event_types: 8, users: 17 },
}

export function createMockTableSource({ preview = 'ready', workspacePreview = 'ready', delay = 250 }: { preview?: DatabasePreview; workspacePreview?: SchemaPreview; delay?: number } = {}): TableSource {
  async function describe(identity: TableIdentity, signal: AbortSignal): Promise<TableDescription> {
    await waitForMock(signal, delay)
    const workspace = await createMockSchemaSource(workspacePreview, 0).load(identity.connectionId, signal)
    const schema = workspace.schemas.find(schema => schema.name === identity.schemaName)
    const table: SchemaTable | undefined = schema?.tables.find(table => table.name === identity.tableName)
    if (!table || !schema) throw new SchemaSourceError('Tabel tidak ditemukan.', 'not-found')
    if (preview === 'error') throw new SchemaSourceError('Detail tabel belum dapat dimuat. Silakan coba lagi.', 'unavailable')
    const columns = (definitions[identity.connectionId]?.[identity.tableName] ?? []).map(([name, dataType, nullable = false, defaultExpression]) => {
      const foreignKey = schema.relations.find(relation => relation.fromTable === table.name && relation.fromColumn === name)
      return { name, dataType, nullable, primaryKey: table.primaryKey.includes(name), defaultExpression: name === 'id' ? `nextval('${table.name}_id_seq'::regclass)` : defaultExpression ?? null,
        ...(foreignKey ? { references: { schema: foreignKey.toSchema, table: foreignKey.toTable, column: foreignKey.toColumn } } : {}) }
    })
    const constraints: TableConstraint[] = [{ name: `${table.name}_pkey`, type: 'PRIMARY KEY', columns: [...table.primaryKey], definition: `PRIMARY KEY (${table.primaryKey.join(', ')})` }]
    if (columns.some(column => column.name === 'email')) constraints.push({ name: `${table.name}_email_key`, type: 'UNIQUE', columns: ['email'], definition: 'UNIQUE (email)' })
    for (const column of columns) if (column.references) constraints.push({ name: `${table.name}_${column.name}_fkey`, type: 'FOREIGN KEY', columns: [column.name], definition: `FOREIGN KEY (${column.name}) REFERENCES ${column.references.schema}.${column.references.table}(${column.references.column})` })
    const indexes: TableIndex[] = constraints.filter(item => item.type !== 'FOREIGN KEY').map(item => ({ name: item.name, columns: [...item.columns], unique: true, definition: `CREATE UNIQUE INDEX ${item.name} ON ${schema.name}.${table.name} USING btree (${item.columns.join(', ')})` }))
    if (columns.some(column => column.name === 'created_at')) indexes.push({ name: `idx_${table.name}_created`, columns: ['created_at'], unique: false, definition: `CREATE INDEX idx_${table.name}_created ON ${schema.name}.${table.name} USING btree (created_at)` })
    return { columns, constraints, indexes, permission: table.permission }
  }
  return { describe, async rows(request, signal) {
    const description = await describe(request, signal)
    const total = preview === 'empty' ? 0 : totals[request.connectionId]?.[request.tableName] ?? 0
    const pages = Math.max(1, Math.ceil(total / TABLE_PAGE_SIZE))
    const page = Math.min(pages, Math.max(1, Number.isSafeInteger(request.page) ? request.page : 1))
    const offset = (page - 1) * TABLE_PAGE_SIZE
    const count = Math.max(0, Math.min(TABLE_PAGE_SIZE, total - offset))
    const rows = Array.from({ length: count }, (_, index) => {
      const id = request.direction === 'desc' ? total - offset - index : offset + index + 1
      return Object.fromEntries(description.columns.map(column => [column.name, mockCell(column, id, request)]))
    })
    return { columns: description.columns.map(column => column.name), rows, total, page, pageSize: TABLE_PAGE_SIZE, pages }
  } }
}

function mockCell(column: TableColumn, id: number, identity: TableIdentity): TableCell {
  const names = ['ani', 'budi', 'citra', 'dedi']
  if (column.name === 'id') return id
  if (column.references) return (id - 1) % (totals[identity.connectionId]?.[column.references.table] ?? 1) + 1
  if (column.nullable && id % 5 === 0) return null
  if (column.name === 'email') return identity.connectionId === 'mock-shop' ? `${names[id - 1] ?? `pengguna${id}`}@mail.com` : `analyst${id}@mail.com`
  if (column.name === 'username') return `analyst${id}`
  if (column.name === 'name') return `${identity.tableName === 'products' ? 'Produk' : identity.tableName === 'warehouses' ? 'Gudang' : 'Item'} ${id}`
  if (column.name === 'sku') return `SKU-${String(id).padStart(4, '0')}`
  if (column.name === 'price') return `${id * 12500}.00`
  if (column.name === 'quantity' || column.name === 'stock') return id * 2
  if (column.dataType === 'boolean') return id % 2 === 1
  if (column.dataType === 'jsonb') return JSON.stringify({ event: id, source: 'web' })
  if (column.dataType === 'timestamp') return ['2026-09-01 09:12', '2026-09-02 14:30', '2026-09-05 08:45', '2026-09-07 17:03'][id - 1] ?? `2026-09-${String((id - 1) % 28 + 1).padStart(2, '0')} 10:00`
  return `${column.name} ${id}`
}
