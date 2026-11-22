export type DatabasePermission = 'view' | 'edit'
export type DatabaseConnection = {
  id: string
  name: string
  engine: 'PostgreSQL'
  status: 'connected' | 'disconnected'
  permission: DatabasePermission
  schemaCount: number | null
  tableCount: number | null
}
export type DatabaseSource = {
  list: (signal: AbortSignal) => Promise<DatabaseConnection[]>
  reconnect: (id: string, signal: AbortSignal) => Promise<DatabaseConnection>
}
export function databaseDescription(database: DatabaseConnection) {
  return database.schemaCount === null || database.tableCount === null
    ? database.engine
    : `${database.engine} · ${database.schemaCount} schema · ${database.tableCount} tabel`
}
