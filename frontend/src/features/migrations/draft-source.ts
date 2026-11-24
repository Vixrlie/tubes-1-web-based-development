import { createMockSchemaSource } from '../databases/schema-source.ts'
import type { SchemaPreview } from '../databases/schema-source.ts'
import { waitForMock } from '../databases/mock-source.ts'
import { createMockMigrationSource } from './migration-source.ts'
import { advanceWorkflow, allApproved, isEditable, jobEnd, latestJob, latestReview, reviewGroups, validJob, validReview, validWorkflow, workflowStates } from './workflow-model.ts'
import type { MigrationJob, ReviewRound, WorkflowAction, WorkflowStatus } from './workflow-model.ts'
import { findExecution, operationEligibility, operationLabels, validIntent, validOperationReferences } from './operation-model.ts'
import type { OperationIntent, OperationKind } from './operation-model.ts'

export type DraftInput = { name: string; sqlUp: string; sqlDown: string; irreversible: boolean }
export type MigrationDraft = DraftInput & { id: string; connectionId: string; ownerId: string; author: string; version: string; status: WorkflowStatus; createdAt: string; updatedAt: string; reviews?: ReviewRound[]; jobs?: MigrationJob[]; intent?: OperationIntent }
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
  return row.ownerId === ownerId && row.connectionId === connectionId && Object.hasOwn(workflowStates, row.status) && typeof row.id === 'string' && /^draft-[\w-]+$/.test(row.id)
    && typeof row.version === 'string' && /^v\d+$/.test(row.version) && Number.isSafeInteger(Number(row.version.slice(1))) && Number(row.version.slice(1)) > 0 && typeof row.author === 'string' && typeof row.name === 'string'
    && typeof row.sqlUp === 'string' && typeof row.sqlDown === 'string' && typeof row.irreversible === 'boolean'
    && typeof row.createdAt === 'string' && Number.isFinite(Date.parse(row.createdAt)) && typeof row.updatedAt === 'string' && Number.isFinite(Date.parse(row.updatedAt))
    && Object.keys(validateDraft(row)).length === 0 && (!row.irreversible || row.sqlDown === '')
    && (row.reviews === undefined || (Array.isArray(row.reviews) && row.reviews.every(validReview)))
    && (row.jobs === undefined || (Array.isArray(row.jobs) && row.jobs.every(validJob)))
    && (row.status === 'draft' || !!row.reviews?.length)
    && validWorkflow(row)
    && (row.intent === undefined || validIntent(row.intent))
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
export function createMockDraftSource({ ownerId, author, storage, preview = 'ready', delay = 250, isActive = () => true, now = Date.now, outcome = 'success', failAfter, reviewerGroupIds = preview === 'readonly' ? ['analytics'] : reviewGroups.map(group => group.id) }: {
  ownerId: string; author: string; storage: () => DraftStorage; preview?: SchemaPreview; delay?: number; isActive?: () => boolean; now?: () => number; outcome?: 'success' | 'failed'; failAfter?: number; reviewerGroupIds?: string[]
}) {
  function active() { if (!ownerId || !isActive()) throw new Error('Sesi sudah berubah. Silakan masuk kembali.') }
  function read(connectionId: string): MigrationDraft[] {
    active()
    const json = storage().getItem(storageKey(ownerId, connectionId))
    if (!json) return []
    let value: unknown
    try { value = JSON.parse(json) } catch { throw new Error('Draft tersimpan tidak dapat dibaca. Periksa penyimpanan browser.') }
    if (!Array.isArray(value) || !value.every(row => isDraft(row, ownerId, connectionId)) || !validOperationReferences(value)) throw new Error('Data draft tersimpan tidak valid.')
    const rows = advanceWorkflow(value, now())
    if (JSON.stringify(rows) !== json) write(connectionId, rows)
    return structuredClone(rows)
  }
  function write(connectionId: string, rows: MigrationDraft[]) {
    active()
    try { storage().setItem(storageKey(ownerId, connectionId), JSON.stringify(rows)) } catch { throw new Error('Draft belum dapat disimpan. Periksa ruang dan izin penyimpanan browser, lalu coba lagi.') }
  }
  async function list(connectionId: string, signal: AbortSignal) {
    await waitForMock(signal, delay)
    await createMockSchemaSource(preview, 0).load(connectionId, signal)
    return read(connectionId)
  }
  return {
    reviewerGroupIds,
    list,
    async requestOperation(connectionId: string, executionId: string, kind: OperationKind, signal: AbortSignal) {
      await waitForMock(signal, delay)
      const workspace = await createMockSchemaSource(preview, 0).load(connectionId, signal)
      if (workspace.database.permission !== 'edit') throw new Error('Pemulihan memerlukan akses Edit.')
      const rows = read(connectionId)
      const eligibility = operationEligibility(rows, executionId, kind)
      if (!eligibility.allowed) throw new Error(eligibility.reason)
      const target = findExecution(rows, executionId)!
      const originDraftId = target.row.intent?.originDraftId ?? target.row.id
      const origin = rows.find(row => row.id === originDraftId)!
      const reverse = kind === 'rollback' || kind === 'compensation'
      const timestamp = new Date(now()).toISOString()
      const row: MigrationDraft = { id: `draft-${crypto.randomUUID()}`, ownerId, connectionId, author, version: origin.version, name: `${operationLabels[kind]} · ${origin.name}`, sqlUp: reverse ? target.row.sqlDown : origin.sqlUp, sqlDown: reverse ? origin.sqlUp : origin.sqlDown, irreversible: !reverse && origin.irreversible, status: 'pending_review', createdAt: timestamp, updatedAt: timestamp, intent: { kind, originDraftId, targetDraftId: target.row.id, targetJobId: target.job.id, targetExecutionId: executionId, checkpoint: target.job.completedBatches, totalBatches: target.job.totalBatches }, reviews: [{ id: `review-${crypto.randomUUID()}`, revision: 1, submittedAt: timestamp, groups: reviewGroups.map(group => ({ ...group, status: 'pending' })) }] }
      active(); if (signal.aborted) throw signal.reason
      write(connectionId, [row, ...rows])
      return structuredClone(row)
    },
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
      if (previous && !isEditable(previous)) throw new Error('SQL terkunci setelah diajukan untuk review.')
      const timestamp = new Date(now()).toISOString()
      const draft: MigrationDraft = { ...previous, ...input, name: input.name.trim(), sqlDown: input.irreversible ? '' : input.sqlDown, id: previous?.id ?? `draft-${crypto.randomUUID()}`, connectionId, ownerId, author: previous?.author ?? author, version: previous?.version ?? version, status: 'draft', createdAt: previous?.createdAt ?? timestamp, updatedAt: timestamp }
      const next = previous ? rows.map(row => row.id === previous.id ? draft : row) : [draft, ...rows]
      active()
      if (signal.aborted) throw signal.reason
      write(connectionId, next)
      return structuredClone(draft)
    },
    async transition(connectionId: string, draftId: string, action: WorkflowAction, signal: AbortSignal) {
      await waitForMock(signal, delay)
      const workspace = await createMockSchemaSource(preview, 0).load(connectionId, signal)
      if (action.type !== 'decide' && workspace.database.permission !== 'edit') throw new Error('Anda hanya memiliki akses untuk melihat database ini.')
      const rows = read(connectionId)
      const row = rows.find(item => item.id === draftId)
      if (!row) throw new Error('Migrasi tidak ditemukan pada database ini.')
      const timestamp = new Date(now()).toISOString()
      if (action.type === 'submit') {
        if (row.status !== 'draft') throw new Error('Hanya draft yang dapat diajukan untuk review.')
        const fields = validateDraft(row)
        if (Object.keys(fields).length) throw new DraftValidationError(fields)
        row.reviews = [...(row.reviews ?? []), { id: `review-${crypto.randomUUID()}`, revision: (latestReview(row)?.revision ?? 0) + 1, submittedAt: timestamp, groups: reviewGroups.map(group => ({ ...group, status: 'pending' })) }]
        row.status = 'pending_review'
      } else if (action.type === 'decide') {
        if (action.decision !== 'approved' && action.decision !== 'rejected') throw new Error('Keputusan review tidak valid.')
        const round = latestReview(row)
        if (row.status !== 'pending_review' || round?.id !== action.reviewId) throw new Error('Review sudah berubah. Muat ulang sebelum memberi keputusan.')
        if (!reviewerGroupIds.includes(action.groupId)) throw new Error('Anda tidak memiliki izin review untuk grup ini.')
        const group = round.groups.find(item => item.id === action.groupId)
        if (!group || group.status !== 'pending') throw new Error('Keputusan grup ini sudah dicatat.')
        const reason = action.reason.trim()
        if (!reason || reason.length > 1000) throw new Error('Alasan wajib diisi, maksimal 1.000 karakter.')
        Object.assign(group, { status: action.decision, reason, actor: author, decidedAt: timestamp })
        row.status = action.decision === 'rejected' ? 'rejected' : allApproved(row) ? 'approved' : 'pending_review'
      } else if (action.type === 'cancel') {
        const job = latestJob(row)
        if (row.status !== 'queued' || job?.id !== action.jobId || job.status !== 'queued') throw new Error('Job yang sudah berjalan tidak dapat dibatalkan.')
        job.status = 'canceled'; job.finishedAt = timestamp
        row.status = 'approved'
      } else {
        if (row.status !== (action.type === 'retry' ? 'failed' : 'approved') || !allApproved(row)) throw new Error('Semua grup wajib menyetujui sebelum migrasi masuk antrean.')
        if (action.type === 'retry' && (latestJob(row)?.completedBatches ?? 0) > 0) throw new Error('Gunakan Resume atau Compensation untuk kegagalan sebagian.')
        if (row.intent) { const eligibility = operationEligibility(rows, row.intent.targetExecutionId, row.intent.kind, row.id); if (!eligibility.allowed) throw new Error(eligibility.reason) }
        const otherJobs = rows.flatMap(item => item.jobs ?? []).filter(job => job.status === 'queued' || job.status === 'running')
        const scheduledAt = new Date(Math.max(now() + 3000, ...otherJobs.map(job => jobEnd(job) + 1000))).toISOString()
        const id = crypto.randomUUID()
        const startBatch = row.intent?.kind === 'resume' ? row.intent.checkpoint : 0
        row.jobs = [...(row.jobs ?? []), { id: `JOB-${id}`, executionId: `E-${id}`, status: 'queued', queuedAt: timestamp, scheduledAt, actor: author, totalBatches: row.intent?.kind === 'compensation' ? row.intent.checkpoint : row.intent?.totalBatches ?? 80, completedBatches: startBatch, ...(startBatch ? { startBatch } : {}), ...(failAfter !== undefined ? { failAfter } : {}), outcome: row.intent ? 'success' : outcome, attempt: (latestJob(row)?.attempt ?? 0) + 1 }]
        row.status = 'queued'
      }
      row.updatedAt = timestamp
      active()
      if (signal.aborted) throw signal.reason
      write(connectionId, rows)
      return structuredClone(row)
    },
  }
}
