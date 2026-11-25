import { ApiError } from '../../shared/api/api-error.ts'
import type { createApiClient } from '../../shared/api/client.ts'
import { connectionErrorMessage } from './mock-source.ts'
import { SchemaSourceError } from './schema-source.ts'
import type { DatabaseWorkspace, SchemaSource } from './schema-source.ts'
import type { TableCell, TableConstraint, TableDescription, TablePage, TableSource } from './table-source.ts'
import type { DatabaseConnection, DatabaseSource } from './types.ts'

type Api = ReturnType<typeof createApiClient>

// Bentuk JSON dari internal/dbm/database_api.go.
export type ApiDatabase = { id: number; name: string; engine: string; status: 'connected' | 'disconnected'; permission: 'view' | 'edit'; schema_count: number | null; table_count: number | null }
type ApiSchema = { name: string; tables: { name: string; column_count: number; primary_key: string[]; kind: 'table' | 'view' }[]; relations: { name: string; from_table: string; from_column: string; to_schema: string; to_table: string; to_column: string }[] }
export type ApiTable = {
  schema: string; name: string; permission: 'view' | 'edit'
  columns: { name: string; data_type: string; nullable: boolean; default: string | null; primary_key: boolean; references?: { schema: string; table: string; column: string } }[]
  constraints: { name: string; type: string; columns: string[]; definition: string }[]
  indexes: { name: string; columns: string[]; unique: boolean; definition: string }[]
}
type ApiPage = { columns: string[]; rows: Record<string, TableCell>[]; total: number; page: number; page_size: number; pages: number }

export const API_PAGE_SIZE = 25

export function toConnection(row: ApiDatabase): DatabaseConnection {
  return { id: String(row.id), name: row.name, engine: 'PostgreSQL', status: row.status, permission: row.permission, schemaCount: row.schema_count, tableCount: row.table_count }
}

export function toTableDescription(row: ApiTable): TableDescription {
  return {
    permission: row.permission,
    columns: row.columns.map(column => ({ name: column.name, dataType: column.data_type, nullable: column.nullable, defaultExpression: column.default, primaryKey: column.primary_key, ...(column.references ? { references: { ...column.references } } : {}) })),
    // CHECK constraint tidak punya tampilan khusus di SPA; ditampilkan bersama UNIQUE.
    constraints: row.constraints.map(item => ({ name: item.name, type: item.type as TableConstraint['type'], columns: [...item.columns], definition: item.definition })),
    indexes: row.indexes.map(item => ({ ...item, columns: [...item.columns] })),
  }
}

/** Petakan error HTTP ke kode yang dipakai halaman workspace. */
export function schemaError(error: unknown): unknown {
  if (!(error instanceof ApiError)) return error
  if (error.status === 403) return new SchemaSourceError('Anda tidak memiliki akses ke database ini.', 'denied')
  if (error.status === 404) return new SchemaSourceError(error.message || 'Database tidak ditemukan.', 'not-found')
  if (error.status === 503) return new SchemaSourceError(connectionErrorMessage, 'unavailable')
  return new SchemaSourceError(error.message, 'unavailable')
}

async function mapped<T>(work: Promise<T>): Promise<T> {
  try { return await work } catch (error) { throw schemaError(error) }
}

const segment = (value: string) => encodeURIComponent(value)

export function createApiDatabaseSource(api: Api): DatabaseSource {
  return {
    async list(signal) {
      const body = await api.request<{ items: ApiDatabase[] }>('/databases', { signal })
      return body.items.map(toConnection)
    },
    async reconnect(id, signal) {
      return toConnection(await api.request<ApiDatabase>(`/databases/${segment(id)}/reconnect`, { method: 'POST', signal }))
    },
  }
}

export function createApiSchemaSource(api: Api): SchemaSource {
  return { async load(connectionId, signal): Promise<DatabaseWorkspace> {
    const body = await mapped(api.request<{ database: ApiDatabase; schemas: ApiSchema[] }>(`/databases/${segment(connectionId)}/schema`, { signal }))
    const database = toConnection(body.database)
    return { database, schemas: body.schemas.map(schema => ({
      name: schema.name,
      tables: schema.tables.map(table => ({ name: table.name, columnCount: table.column_count, primaryKey: [...table.primary_key], kind: table.kind, permission: database.permission })),
      relations: schema.relations.map(rel => ({ name: rel.name, fromTable: rel.from_table, fromColumn: rel.from_column, toSchema: rel.to_schema, toTable: rel.to_table, toColumn: rel.to_column })),
    })) }
  } }
}

export function createApiTableSource(api: Api, pageSize = API_PAGE_SIZE): TableSource {
  const base = (id: { connectionId: string; schemaName: string; tableName: string }) => `/databases/${segment(id.connectionId)}/tables/${segment(id.schemaName)}/${segment(id.tableName)}`
  return {
    async describe(identity, signal) {
      return toTableDescription(await mapped(api.request<ApiTable>(base(identity), { signal })))
    },
    async rows(request, signal): Promise<TablePage> {
      const page = Number.isSafeInteger(request.page) && request.page > 0 ? request.page : 1
      const body = await mapped(api.request<ApiPage>(`${base(request)}/rows`, { signal, query: { page, page_size: pageSize, direction: request.direction } }))
      return { columns: body.columns, rows: body.rows, total: body.total, page: body.page, pageSize: body.page_size, pages: body.pages }
    },
  }
}
