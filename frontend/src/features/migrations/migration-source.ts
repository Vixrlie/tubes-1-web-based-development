import { waitForMock } from '../databases/mock-source.ts'
import type { DatabasePreview } from '../databases/mock-source.ts'
import { createMockSchemaSource } from '../databases/schema-source.ts'
import type { SchemaPreview } from '../databases/schema-source.ts'

export type ExecutionStatus = 'success' | 'failed'
export type Execution = {
  id: string; version: string; name: string; action: 'UP' | 'ROLLBACK' | 'FORWARD'
  actor: string; executedAt: string; status: ExecutionStatus; duration: string; sql: string
  rollbackTarget?: string; irreversible?: boolean; error?: string
  /** Eksekusi batch yang berhenti setelah sebagian batch tersimpan. */
  partial?: 'applied' | 'reverted'
}
export type MigrationSource = {
  list: (connectionId: string, signal: AbortSignal) => Promise<Execution[]>
  detail: (connectionId: string, executionId: string, signal: AbortSignal) => Promise<Execution | undefined>
}

// Execution history fixtures, isolated from the server and from SQL execution.
const shopHistory: readonly Execution[] = [
  { id: 'E1050', version: 'v014', name: 'delete_archive', action: 'UP', actor: 'Budi S.', executedAt: '2026-11-07 09:42', status: 'success', duration: '0,18 detik', irreversible: true, sql: 'DROP TABLE archive;' },
  { id: 'E1048', version: 'v013', name: 'add_product_status', action: 'UP', actor: 'Budi S.', executedAt: '2026-11-07 09:42', status: 'success', duration: '0,42 detik', sql: "ALTER TABLE products ADD COLUMN status TEXT DEFAULT 'active';" },
  { id: 'E1047', version: 'v012', name: 'backfill_users', action: 'ROLLBACK', rollbackTarget: 'E1045', actor: 'Nia', executedAt: '2026-11-07 09:10', status: 'success', duration: '0,24 detik', sql: 'UPDATE users SET email = backup.email\nFROM migration_user_backup AS backup\nWHERE users.id = backup.id;' },
  { id: 'E1046', version: 'v011', name: 'add_users', action: 'FORWARD', actor: 'Nia', executedAt: '2026-11-07 08:54', status: 'success', duration: '0,31 detik', sql: 'CREATE TABLE users (\n  id BIGSERIAL PRIMARY KEY,\n  email TEXT NOT NULL,\n  created_at TIMESTAMPTZ DEFAULT now()\n);' },
  { id: 'E1045', version: 'v012', name: 'backfill_users', action: 'UP', actor: 'Nia', executedAt: '2026-11-07 08:25', status: 'success', duration: '0,28 detik', sql: 'UPDATE users SET email = lower(email);' },
  { id: 'E1044', version: 'v010', name: 'index_orders', action: 'UP', actor: 'Nia', executedAt: '2026-11-07 07:33', status: 'success', duration: '0,36 detik', sql: 'CREATE INDEX orders_user_id_idx ON orders (user_id);' },
]

export function executionAction(execution: Execution) {
  return execution.action === 'ROLLBACK' ? `ROLLBACK ${execution.rollbackTarget ?? ''}`.trim() : execution.action
}
export function executionStatus(status: ExecutionStatus, partial?: Execution['partial']) {
  if (partial) return partial === 'reverted' ? 'Partially reverted' : 'Partial'
  return status === 'success' ? 'Sukses' : 'Gagal'
}
export function executionTone(row: Pick<Execution, 'status' | 'partial'>) { return row.partial ? 'warning' as const : row.status === 'success' ? 'success' as const : 'error' as const }
export function parseExecutionStatus(value: string | null): 'all' | ExecutionStatus {
  return value === 'success' || value === 'failed' ? value : 'all'
}
export function filterExecutions(rows: Execution[], hideRollback: boolean, status: 'all' | ExecutionStatus = 'all') {
  const hidden = new Set<string>()
  if (hideRollback) for (const rollback of rows) {
    const original = rows.find(row => row.id === rollback.rollbackTarget)
    if (rollback.action === 'ROLLBACK' && rollback.status === 'success' && original?.status === 'success' && original.action !== 'ROLLBACK' && original.version === rollback.version) {
      hidden.add(rollback.id); hidden.add(original.id)
    }
  }
  return rows.filter(row => !hidden.has(row.id) && (status === 'all' || row.status === status))
}

export function createMockMigrationSource(preview: DatabasePreview = 'ready', workspacePreview: SchemaPreview = 'ready', delay = 300): MigrationSource {
  async function list(connectionId: string, signal: AbortSignal) {
    await waitForMock(signal, delay)
    await createMockSchemaSource(workspacePreview, 0).load(connectionId, signal)
    if (preview === 'error') throw new Error('Riwayat migrasi belum dapat dimuat. Silakan coba lagi.')
    return preview === 'empty' || connectionId !== 'mock-shop' ? [] : shopHistory.map(row => ({ ...row }))
  }
  return { list, async detail(connectionId, executionId, signal) { return (await list(connectionId, signal)).find(row => row.id === executionId) } }
}

/** Preserve history filters and development previews across detail/back navigation. */
export function withMigrationSearch(path: string, params: URLSearchParams) {
  const next = new URLSearchParams()
  for (const key of ['preview', 'migrationPreview', 'hideRollback', 'status', 'jobOutcome', 'reviewStatus', 'queueStatus']) {
    const value = params.get(key)
    if (value) next.set(key, value)
  }
  return `${path}${next.size ? `?${next}` : ''}`
}
