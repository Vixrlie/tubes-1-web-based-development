import type { DatabaseConnection, DatabaseSource } from './types.ts'

// UI fixtures only: these are never real connection IDs or effective server grants.
const fixtures: readonly DatabaseConnection[] = [
  { id: 'mock-shop', name: 'shop_db', engine: 'PostgreSQL', status: 'connected', permission: 'edit', schemaCount: 1, tableCount: 3 },
  { id: 'mock-inventory', name: 'inventory_db', engine: 'PostgreSQL', status: 'connected', permission: 'edit', schemaCount: 1, tableCount: 3 },
  { id: 'mock-analytics', name: 'analytics_db', engine: 'PostgreSQL', status: 'connected', permission: 'edit', schemaCount: 1, tableCount: 3 },
  { id: 'mock-hr', name: 'hr_db', engine: 'PostgreSQL', status: 'disconnected', permission: 'view', schemaCount: null, tableCount: null },
]
export const connectionErrorMessage = 'Tidak dapat terhubung ke database. Coba lagi beberapa saat.'
export type DatabasePreview = 'ready' | 'loading' | 'empty' | 'error'
export function databasePreview(value: string | null): DatabasePreview {
  return value === 'loading' || value === 'empty' || value === 'error' ? value : 'ready'
}
export function waitForMock(signal: AbortSignal, delay: number) {
  return new Promise<void>((resolve, reject) => {
    if (signal.aborted) { reject(signal.reason ?? new DOMException('Dibatalkan', 'AbortError')); return }
    const abort = () => { clearTimeout(timer); reject(signal.reason ?? new DOMException('Dibatalkan', 'AbortError')) }
    const timer = setTimeout(() => { signal.removeEventListener('abort', abort); resolve() }, delay)
    signal.addEventListener('abort', abort, { once: true })
  })
}
/** Replace this adapter with JSON API later; page components consume the same type. */
export function createMockDatabaseSource(preview: DatabasePreview = 'ready', delay = 350): DatabaseSource {
  return {
    async list(signal) {
      await waitForMock(signal, delay)
      if (preview === 'error') throw new Error('Daftar database belum dapat dimuat. Silakan coba lagi.')
      return preview === 'empty' ? [] : fixtures.map(database => ({ ...database }))
    },
    async reconnect(id, signal) {
      await waitForMock(signal, delay + 300)
      const database = fixtures.find(item => item.id === id)
      if (!database) throw new Error('Database tidak ditemukan.')
      if (database.status === 'disconnected') throw new Error(connectionErrorMessage)
      return { ...database }
    },
  }
}
