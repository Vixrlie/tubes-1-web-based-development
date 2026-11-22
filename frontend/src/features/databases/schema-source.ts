import { createMockDatabaseSource, waitForMock } from './mock-source.ts'
import type { DatabaseConnection, DatabasePermission } from './types.ts'

export type SchemaTable = { name: string; columnCount: number; primaryKey: string[]; kind: 'table' | 'view'; permission: DatabasePermission }
export type ForeignKey = { name: string; fromTable: string; fromColumn: string; toSchema: string; toTable: string; toColumn: string }
export type SchemaData = { name: string; tables: SchemaTable[]; relations: ForeignKey[] }
export type DatabaseWorkspace = { database: DatabaseConnection; schemas: SchemaData[] }
export type SchemaSource = { load: (connectionId: string, signal: AbortSignal) => Promise<DatabaseWorkspace> }
export type SchemaPreview = 'ready' | 'loading' | 'empty' | 'error' | 'readonly' | 'noschema' | 'multiple' | 'denied'
export function schemaPreview(value: string | null): SchemaPreview {
  return ['loading', 'empty', 'error', 'readonly', 'noschema', 'multiple', 'denied'].includes(value ?? '') ? value as SchemaPreview : 'ready'
}
export class SchemaSourceError extends Error {
  code: 'not-found' | 'unavailable' | 'denied'
  constructor(message: string, code: 'not-found' | 'unavailable' | 'denied') { super(message); this.name = 'SchemaSourceError'; this.code = code }
}
const tablesByConnection: Record<string, [string, number][]> = {
  'mock-shop': [['products', 6], ['users', 3], ['orders', 5]],
  'mock-inventory': [['items', 6], ['warehouses', 3], ['stock', 5]],
  'mock-analytics': [['events', 6], ['event_types', 3], ['users', 5]],
}
const relationsByConnection: Record<string, ForeignKey[]> = {
  'mock-shop': [
    { name: 'orders_user_id_fkey', fromTable: 'orders', fromColumn: 'user_id', toSchema: 'public', toTable: 'users', toColumn: 'id' },
    { name: 'orders_product_id_fkey', fromTable: 'orders', fromColumn: 'product_id', toSchema: 'public', toTable: 'products', toColumn: 'id' },
  ],
  'mock-inventory': [
    { name: 'stock_item_id_fkey', fromTable: 'stock', fromColumn: 'item_id', toSchema: 'public', toTable: 'items', toColumn: 'id' },
    { name: 'stock_warehouse_id_fkey', fromTable: 'stock', fromColumn: 'warehouse_id', toSchema: 'public', toTable: 'warehouses', toColumn: 'id' },
  ],
  'mock-analytics': [
    { name: 'events_type_id_fkey', fromTable: 'events', fromColumn: 'type_id', toSchema: 'public', toTable: 'event_types', toColumn: 'id' },
    { name: 'events_user_id_fkey', fromTable: 'events', fromColumn: 'user_id', toSchema: 'public', toTable: 'users', toColumn: 'id' },
  ],
}
export function createMockSchemaSource(preview: SchemaPreview = 'ready', delay = 300): SchemaSource {
  return { async load(connectionId, signal) {
    await waitForMock(signal, delay)
    const connections = await createMockDatabaseSource('ready', 0).list(signal)
    const database = connections.find(connection => connection.id === connectionId)
    if (!database) throw new SchemaSourceError('Database tidak ditemukan.', 'not-found')
    if (preview === 'denied') throw new SchemaSourceError('Anda tidak memiliki akses ke database ini.', 'denied')
    if (database.status !== 'connected') throw new SchemaSourceError('Tidak dapat terhubung ke database. Coba lagi beberapa saat.', 'unavailable')
    if (preview === 'error') throw new SchemaSourceError('Schema belum dapat dimuat. Silakan coba lagi.', 'unavailable')
    if (preview === 'readonly') database.permission = 'view'
    if (preview === 'empty') database.tableCount = 0
    if (preview === 'noschema') { database.schemaCount = 0; database.tableCount = 0 }
    if (preview === 'multiple') database.schemaCount = 2
    const schema: SchemaData = {
      name: 'public',
      tables: preview === 'empty' ? [] : (tablesByConnection[connectionId] ?? []).map(([name, columnCount]) => ({ name, columnCount, primaryKey: ['id'], kind: 'table', permission: database.permission })),
      relations: preview === 'empty' ? [] : (relationsByConnection[connectionId] ?? []).map(relation => ({ ...relation })),
    }
    return { database, schemas: preview === 'noschema' ? [] : preview === 'multiple' ? [schema, { name: 'audit', tables: [], relations: [] }] : [schema] }
  } }
}

export function selectSchema(schemas: SchemaData[], name: string | null | undefined) {
  return name ? schemas.find(schema => schema.name === name) : schemas[0]
}
export function withWorkspacePreview(path: string, preview: SchemaPreview) {
  return preview === 'ready' ? path : `${path}${path.includes('?') ? '&' : '?'}preview=${encodeURIComponent(preview)}`
}
