import { createMockSchemaSource } from '../databases/schema-source.ts'
import type { SchemaPreview } from '../databases/schema-source.ts'
import { waitForMock } from '../databases/mock-source.ts'
import { createMockMigrationSource } from './migration-source.ts'

export type DraftInput = { name: string; sqlUp: string; sqlDown: string; irreversible: boolean }
export type MigrationDraft = DraftInput & { id: string; connectionId: string; ownerId: string; author: string; version: string; status: 'draft'; createdAt: string; updatedAt: string }
export type DraftErrors = Partial<Record<keyof DraftInput, string>>
export type DraftStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem' | 'key' | 'length'>
const prefix = 'pathgres:mock-migration-drafts:v1:'
export function validateDraft(input: DraftInput): DraftErrors {
  const errors: DraftErrors = {}
  if (!input.name.trim()) errors.name = 'Nama migrasi wajib diisi.'
  else if (input.name.trim().length > 120) errors.name = 'Nama migrasi maksimal 120 karakter.'
  if (!input.sqlUp.trim()) errors.sqlUp = 'SQL Up wajib diisi.'
  else if (input.sqlUp.length > 50000) errors.sqlUp = 'SQL Up maksimal 50.000 karakter.'
  if (!input.irreversible && !input.sqlDown.trim()) errors.sqlDown = 'Isi SQL Down atau tandai migrasi sebagai irreversible.'
  else if (!input.irreversible && input.sqlDown.length > 50000) errors.sqlDown = 'SQL Down maksimal 50.000 karakter.'
  return errors
}
export class DraftValidationError extends Error {
  fields: DraftErrors
  constructor(fields: DraftErrors) { super('Periksa kembali field migrasi.'); this.fields = fields }
}
function storageKey(ownerId: string, connectionId: string) { return `${prefix}${encodeURIComponent(ownerId)}:${encodeURIComponent(connectionId)}` }
function isDraft(value: unknown, ownerId: string, connectionId: string): value is MigrationDraft {
  if (!value || typeof value !== 'object') return false
  const row = value as MigrationDraft
  return row.ownerId === ownerId && row.connectionId === connectionId && row.status === 'draft' && typeof row.id === 'string' && /^draft-[\w-]+$/.test(row.id)
    && typeof row.version === 'string' && /^v\d+$/.test(row.version) && Number.isSafeInteger(Number(row.version.slice(1))) && Number(row.version.slice(1)) > 0 && typeof row.author === 'string' && typeof row.name === 'string'
    && typeof row.sqlUp === 'string' && typeof row.sqlDown === 'string' && typeof row.irreversible === 'boolean'
    && typeof row.createdAt === 'string' && Number.isFinite(Date.parse(row.createdAt)) && typeof row.updatedAt === 'string' && Number.isFinite(Date.parse(row.updatedAt))
    && Object.keys(validateDraft(row)).length === 0 && (!row.irreversible || row.sqlDown === '')
}
export function clearMockDraftStorage(storage: DraftStorage, exceptOwnerId?: string) {
  const keepPrefix = exceptOwnerId ? `${prefix}${encodeURIComponent(exceptOwnerId)}:` : undefined
  const keys = Array.from({ length: storage.length }, (_, index) => storage.key(index))
  for (const key of keys) if (key?.startsWith(prefix) && (!keepPrefix || !key.startsWith(keepPrefix))) storage.removeItem(key)
}
export function browserDraftStorage(): DraftStorage {
  try { return window.sessionStorage } catch { throw new Error('Penyimpanan draft tidak tersedia. Izinkan penyimpanan browser lalu coba lagi.') }
}
export function clearBrowserMockDrafts(exceptOwnerId?: string) {
  try { clearMockDraftStorage(browserDraftStorage(), exceptOwnerId) } catch { /* Storage can be unavailable; saving reports its own error. */ }
}
export function createMockDraftSource({ ownerId, author, storage, preview = 'ready', delay = 250, isActive = () => true }: {
  ownerId: string; author: string; storage: () => DraftStorage; preview?: SchemaPreview; delay?: number; isActive?: () => boolean
}) {
  function active() { if (!ownerId || !isActive()) throw new Error('Sesi sudah berubah. Silakan masuk kembali.') }
  function read(connectionId: string): MigrationDraft[] {
    active()
    const json = storage().getItem(storageKey(ownerId, connectionId))
    if (!json) return []
    let value: unknown
    try { value = JSON.parse(json) } catch { throw new Error('Draft tersimpan tidak dapat dibaca. Periksa penyimpanan browser.') }
    if (!Array.isArray(value) || !value.every(row => isDraft(row, ownerId, connectionId))) throw new Error('Data draft tersimpan tidak valid.')
    return value.map(row => ({ ...row }))
  }
  async function list(connectionId: string, signal: AbortSignal) {
    await waitForMock(signal, delay)
    await createMockSchemaSource(preview, 0).load(connectionId, signal)
    return read(connectionId)
  }
  return {
    list,
    async nextVersion(connectionId: string, signal: AbortSignal) {
      const drafts = await list(connectionId, signal)
      const history = await createMockMigrationSource('ready', preview, 0).list(connectionId, signal)
      return `v${String(Math.max(0, ...[...drafts, ...history].map(row => Number(row.version.slice(1)))) + 1).padStart(3, '0')}`
    },
    async save(connectionId: string, input: DraftInput, signal: AbortSignal, draftId?: string) {
      const fields = validateDraft(input)
      if (Object.keys(fields).length) throw new DraftValidationError(fields)
      await waitForMock(signal, delay)
      const workspace = await createMockSchemaSource(preview, 0).load(connectionId, signal)
      if (workspace.database.permission !== 'edit') throw new Error('Anda hanya memiliki akses untuk melihat database ini.')
      // Compute the version before the final synchronous read/write to avoid stale writes.
      const nextVersion = await this.nextVersion(connectionId, signal)
      const rows = read(connectionId)
      const version = `v${String(Math.max(Number(nextVersion.slice(1)), ...rows.map(row => Number(row.version.slice(1)) + 1))).padStart(3, '0')}`
      const previous = draftId ? rows.find(row => row.id === draftId) : undefined
      if (draftId && !previous) throw new Error('Draft tidak ditemukan pada database ini.')
      const now = new Date().toISOString()
      const draft: MigrationDraft = { ...input, name: input.name.trim(), sqlDown: input.irreversible ? '' : input.sqlDown, id: previous?.id ?? `draft-${crypto.randomUUID()}`, connectionId, ownerId, author: previous?.author ?? author, version: previous?.version ?? version, status: 'draft', createdAt: previous?.createdAt ?? now, updatedAt: now }
      const next = previous ? rows.map(row => row.id === previous.id ? draft : row) : [draft, ...rows]
      active()
      if (signal.aborted) throw signal.reason
      try { storage().setItem(storageKey(ownerId, connectionId), JSON.stringify(next)) } catch { throw new Error('Draft belum dapat disimpan. Periksa ruang dan izin penyimpanan browser, lalu coba lagi.') }
      return { ...draft }
    },
  }
}
