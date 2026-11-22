import type { MigrationDraft } from './draft-source.ts'
import type { Execution } from './migration-source.ts'

export type WorkflowStatus = 'draft' | 'pending_review' | 'approved' | 'rejected' | 'queued' | 'running' | 'success' | 'failed'
export type ReviewGroup = { id: string; name: string; permission: 'view' | 'edit'; status: 'pending' | 'approved' | 'rejected'; actor?: string; reason?: string; decidedAt?: string }
export type ReviewRound = { id: string; revision: number; submittedAt: string; groups: ReviewGroup[] }
export type MigrationJob = { id: string; executionId: string; status: 'queued' | 'running' | 'success' | 'failed' | 'canceled'; queuedAt: string; scheduledAt: string; startedAt?: string; finishedAt?: string; actor: string; totalBatches: number; completedBatches: number; startBatch?: number; failAfter?: number; outcome: 'success' | 'failed'; attempt: number; error?: string }
export type WorkflowAction = { type: 'submit' } | { type: 'decide'; reviewId: string; groupId: string; decision: 'approved' | 'rejected'; reason: string } | { type: 'enqueue' | 'retry' } | { type: 'cancel'; jobId: string }
export const reviewGroups = [
  { id: 'backend', name: 'Backend Team', permission: 'edit' },
  { id: 'analytics', name: 'Analytics Team', permission: 'view' },
  { id: 'operations', name: 'Operations Team', permission: 'edit' },
] as const
export const workflowStates = {
  draft: { label: 'Draft', tone: 'neutral' }, pending_review: { label: 'Menunggu review', tone: 'warning' },
  approved: { label: 'Disetujui', tone: 'success' }, rejected: { label: 'Ditolak', tone: 'error' },
  queued: { label: 'Antrean', tone: 'warning' }, running: { label: 'Berjalan', tone: 'info' },
  success: { label: 'Selesai', tone: 'success' }, failed: { label: 'Gagal', tone: 'error' },
} as const
export const batchDurationMs = 250
export const failureBatch = 42
export function shortJobId(id: string) { return id.slice(0, 12).toUpperCase() }
export function failureCheckpoint(job: MigrationJob) { return Math.min(job.totalBatches - 1, Math.max(job.startBatch ?? 0, job.failAfter ?? failureBatch)) }
export function jobEnd(job: MigrationJob) { return Date.parse(job.scheduledAt) + ((job.outcome === 'failed' ? failureCheckpoint(job) + 1 : job.totalBatches) - (job.startBatch ?? 0)) * batchDurationMs }
export function advanceJob(job: MigrationJob, now: number): MigrationJob {
  if (job.status !== 'queued' && job.status !== 'running') return job
  const start = Date.parse(job.scheduledAt)
  if (now < start) return job
  const batches = Math.max(0, Math.floor((now - start) / batchDurationMs))
  const terminal = now >= jobEnd(job)
  return { ...job, startedAt: job.scheduledAt, status: terminal ? job.outcome : 'running', completedBatches: Math.min(Math.max(job.completedBatches, (job.startBatch ?? 0) + batches), job.outcome === 'failed' ? failureCheckpoint(job) : job.totalBatches), ...(terminal ? { finishedAt: new Date(jobEnd(job)).toISOString(), ...(job.outcome === 'failed' ? { error: `Eksekusi terhenti pada batch ${failureCheckpoint(job) + 1}. Periksa perubahan sebelum pemulihan.` } : {}) } : {}) }
}
export function advanceWorkflow(rows: MigrationDraft[], now: number): MigrationDraft[] {
  return rows.map(row => {
    if (!row.jobs?.some(job => job.status === 'queued' || job.status === 'running')) return row
    const jobs = row.jobs.map(job => advanceJob(job, now))
    const latest = jobs.at(-1)!
    return { ...row, jobs, status: latest.status === 'canceled' ? 'approved' : latest.status, updatedAt: latest.finishedAt ?? latest.startedAt ?? row.updatedAt }
  })
}
export function workflowExecutions(rows: MigrationDraft[]): Execution[] {
  return rows.flatMap(row => (row.jobs ?? []).filter(job => job.status === 'success' || job.status === 'failed').map(job => ({ id: job.executionId, version: row.version, name: row.name, action: row.intent?.kind === 'rollback' || row.intent?.kind === 'compensation' ? 'ROLLBACK' as const : row.intent?.kind === 'forward' ? 'FORWARD' as const : 'UP' as const, ...(row.intent?.kind === 'rollback' || row.intent?.kind === 'compensation' ? { rollbackTarget: row.intent.targetExecutionId } : {}), actor: job.actor, executedAt: job.finishedAt!, status: job.status as 'success' | 'failed', duration: `${((Date.parse(job.finishedAt!) - Date.parse(job.startedAt!)) / 1000).toLocaleString('id-ID')} detik`, sql: row.sqlUp, irreversible: row.irreversible, error: job.error }))).sort((a, b) => b.executedAt.localeCompare(a.executedAt))
}
export function latestReview(row: MigrationDraft) { return row.reviews?.at(-1) }
export function latestJob(row: MigrationDraft) { return row.jobs?.at(-1) }
export function isEditable(row: MigrationDraft) { return !row.intent && (row.status === 'draft' || row.status === 'rejected') }
export function allApproved(row: MigrationDraft) { const round = latestReview(row); return !!round?.groups.length && round.groups.every(group => group.status === 'approved') }

export function validReview(value: unknown): value is ReviewRound {
  if (!value || typeof value !== 'object') return false
  const row = value as ReviewRound
  return typeof row.id === 'string' && Number.isSafeInteger(row.revision) && row.revision > 0 && typeof row.submittedAt === 'string' && Number.isFinite(Date.parse(row.submittedAt)) && Array.isArray(row.groups) && row.groups.length === reviewGroups.length && new Set(row.groups.map(group => group?.id)).size === reviewGroups.length && row.groups.every(group => group && reviewGroups.some(item => item.id === group.id && item.name === group.name && item.permission === group.permission) && ['pending', 'approved', 'rejected'].includes(group.status) && (group.status === 'pending' || (typeof group.reason === 'string' && !!group.reason.trim() && group.reason.length <= 1000 && typeof group.actor === 'string' && typeof group.decidedAt === 'string' && Number.isFinite(Date.parse(group.decidedAt)))))
}
export function validJob(value: unknown): value is MigrationJob {
  if (!value || typeof value !== 'object') return false
  const job = value as MigrationJob
  return typeof job.id === 'string' && /^JOB-[\da-f-]{36}$/.test(job.id) && job.executionId === `E-${job.id.slice(4)}` && ['queued', 'running', 'success', 'failed', 'canceled'].includes(job.status) && ['success', 'failed'].includes(job.outcome) && typeof job.actor === 'string' && Number.isFinite(Date.parse(job.queuedAt)) && Number.isFinite(Date.parse(job.scheduledAt)) && Number.isInteger(job.totalBatches) && job.totalBatches > 0 && job.totalBatches <= 80 && Number.isSafeInteger(job.completedBatches) && Number.isInteger(job.startBatch ?? 0) && (job.startBatch ?? 0) >= 0 && (job.startBatch ?? 0) < job.totalBatches && job.completedBatches >= (job.startBatch ?? 0) && job.completedBatches <= job.totalBatches && Number.isSafeInteger(job.attempt) && job.attempt > 0
    && (!['running', 'success', 'failed'].includes(job.status) || (!!job.startedAt && Number.isFinite(Date.parse(job.startedAt))))
    && (!['success', 'failed', 'canceled'].includes(job.status) || (!!job.finishedAt && Number.isFinite(Date.parse(job.finishedAt))))
    && (job.status !== 'success' || (job.outcome === 'success' && job.completedBatches === job.totalBatches))
    && (job.status !== 'failed' || (job.outcome === 'failed' && job.completedBatches === failureCheckpoint(job) && typeof job.error === 'string'))
    && (!['queued', 'canceled'].includes(job.status) || job.completedBatches === (job.startBatch ?? 0))
}
export function validWorkflow(row: MigrationDraft) {
  const reviews = row.reviews ?? []
  const jobs = row.jobs ?? []
  if (new Set(reviews.map(review => review.id)).size !== reviews.length || reviews.some((review, index) => review.revision !== index + 1)) return false
  if (new Set(jobs.map(job => job.id)).size !== jobs.length || jobs.some((job, index) => job.attempt !== index + 1)) return false
  const round = latestReview(row)
  if (row.status === 'draft') return jobs.length === 0 && (!round || round.groups.some(group => group.status === 'rejected'))
  if (row.status === 'pending_review') return jobs.length === 0 && !!round?.groups.some(group => group.status === 'pending') && !round.groups.some(group => group.status === 'rejected')
  if (row.status === 'rejected') return jobs.length === 0 && !!round?.groups.some(group => group.status === 'rejected')
  if (!allApproved(row)) return false
  if (row.status === 'approved') return !jobs.length || latestJob(row)?.status === 'canceled'
  return latestJob(row)?.status === row.status
}
