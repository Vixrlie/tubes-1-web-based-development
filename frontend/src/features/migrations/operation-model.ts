import type { MigrationDraft } from './draft-source.ts'
import type { MigrationJob } from './workflow-model.ts'

export type OperationKind = 'rollback' | 'forward' | 'resume' | 'compensation'
export type OperationIntent = { kind: OperationKind; originDraftId: string; targetDraftId: string; targetJobId: string; targetExecutionId: string; checkpoint: number; totalBatches: number }
export const operationLabels = { rollback: 'Rollback', forward: 'Forward', resume: 'Resume', compensation: 'Compensation' } as const
export function findExecution(rows: MigrationDraft[], executionId: string) { const row = rows.find(item => item.jobs?.some(job => job.executionId === executionId)); const job = row?.jobs?.find(item => item.executionId === executionId); return row && job ? { row, job } : undefined }
const originId = (row: MigrationDraft) => row.intent?.originDraftId ?? row.id
function entries(rows: MigrationDraft[]) { return rows.flatMap(row => (row.jobs ?? []).map(job => ({ row, job }))).sort((a, b) => a.job.queuedAt.localeCompare(b.job.queuedAt) || a.job.attempt - b.job.attempt) }
export function activeExecutions(rows: MigrationDraft[]) {
  const active: { row: MigrationDraft; job: MigrationJob }[] = []
  for (const entry of entries(rows).filter(entry => entry.job.status === 'success')) {
    const root = originId(entry.row)
    const previous = active.findIndex(item => originId(item.row) === root)
    if (previous >= 0) active.splice(previous, 1)
    if (entry.row.intent?.kind !== 'rollback' && entry.row.intent?.kind !== 'compensation') active.push(entry)
  }
  return active
}
export function operationEligibility(rows: MigrationDraft[], executionId: string, kind: OperationKind, ignoreRequestId?: string): { allowed: boolean; reason: string } {
  const target = findExecution(rows, executionId)
  const fail = (reason: string) => ({ allowed: false, reason })
  if (!target) return fail('Snapshot eksekusi tidak tersedia pada database ini.')
  const { row, job } = target; const root = originId(row)
  if (rows.some(item => item.id !== ignoreRequestId && item.intent?.originDraftId === root && ['pending_review', 'approved', 'queued', 'running'].includes(item.status))) return fail('Masih ada permintaan pemulihan untuk migrasi ini.')
  if (rows.some(item => item.jobs?.some(candidate => candidate.status === 'queued' || candidate.status === 'running'))) return fail('Tunggu job yang sedang berjalan atau mengantre selesai.')
  const last = entries(rows).filter(entry => originId(entry.row) === root && entry.job.status !== 'canceled').at(-1)
  if (last?.job.id !== job.id) return fail('Pilih hasil eksekusi terbaru dari migrasi ini.')
  if (kind === 'rollback') {
    if (row.irreversible || !row.sqlDown.trim()) return fail('Migrasi irreversible tidak menyediakan SQL Down.')
    if (job.status !== 'success' || ['rollback', 'compensation'].includes(row.intent?.kind ?? '') || activeExecutions(rows).at(-1)?.job.id !== job.id) return fail('Rollback hanya tersedia untuk eksekusi reversible aktif terakhir.')
  } else if (kind === 'forward') {
    if (job.status !== 'success' || !['rollback', 'compensation'].includes(row.intent?.kind ?? '') || activeExecutions(rows).some(entry => originId(entry.row) === root)) return fail('Forward tersedia setelah rollback atau compensation selesai.')
  } else {
    if (job.status !== 'failed' || job.completedBatches <= 0 || job.completedBatches >= job.totalBatches || ['rollback', 'compensation'].includes(row.intent?.kind ?? '')) return fail('Recovery memerlukan eksekusi gagal sebagian dengan checkpoint tersimpan.')
    if (kind === 'compensation' && (row.irreversible || !row.sqlDown.trim())) return fail('Compensation memerlukan SQL Down asli.')
  }
  return { allowed: true, reason: '' }
}
export function validIntent(value: unknown): value is OperationIntent {
  if (!value || typeof value !== 'object') return false
  const intent = value as OperationIntent
  return Object.hasOwn(operationLabels, intent.kind) && ['originDraftId', 'targetDraftId', 'targetJobId', 'targetExecutionId'].every(key => typeof (value as Record<string, unknown>)[key] === 'string') && Number.isInteger(intent.checkpoint) && intent.checkpoint >= 0 && intent.checkpoint <= 80 && Number.isInteger(intent.totalBatches) && intent.totalBatches > 0 && intent.totalBatches <= 80
}
export function validOperationReferences(rows: MigrationDraft[]) {
  return rows.every(row => {
    if (!row.intent) return true
    const intent = row.intent
    const target = findExecution(rows, intent.targetExecutionId)
    const origin = rows.find(candidate => candidate.id === intent.originDraftId && !candidate.intent)
    if (!origin || !target || target.row.id !== intent.targetDraftId || target.job.id !== intent.targetJobId || intent.checkpoint !== target.job.completedBatches || intent.totalBatches !== target.job.totalBatches) return false
    const reverse = intent.kind === 'rollback' || intent.kind === 'compensation'
    return row.version === origin.version && row.sqlUp === (reverse ? target.row.sqlDown : origin.sqlUp) && row.sqlDown === (reverse ? origin.sqlUp : origin.sqlDown) && !!row.reviews?.length && (row.jobs ?? []).every(job => job.totalBatches === (intent.kind === 'compensation' ? intent.checkpoint : intent.totalBatches) && (job.startBatch ?? 0) === (intent.kind === 'resume' ? intent.checkpoint : 0))
  })
}
