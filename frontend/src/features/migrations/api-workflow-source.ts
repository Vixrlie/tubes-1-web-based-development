import type { createApiClient } from '../../shared/api/client.ts'
import { ApiError } from '../../shared/api/api-error.ts'
import { DraftValidationError, validateDraft } from './draft-source.ts'
import type { DraftInput, MigrationDraft } from './draft-source.ts'
import type { Execution } from './migration-source.ts'
import { operationLabels } from './operation-model.ts'
import type { OperationIntent, OperationKind } from './operation-model.ts'
import type { MigrationJob, ReviewGroup, ReviewRound, WorkflowAction, WorkflowStatus } from './workflow-model.ts'

type Api = ReturnType<typeof createApiClient>

// Bentuk JSON GET /api/databases/{id}/workflow (internal/dbm/workflow_board.go).
export type ApiBatch = { table: string; column: string; size: number }
export type ApiMigration = {
  id: number; connection_id: number; version: number; name: string; up_sql?: string; down_sql?: string
  irreversible_reason?: string; batch?: ApiBatch; revision: number; created_by?: number; created_by_name?: string
  created_at: string; updated_at: string; state: string; locked_at?: string
  validation: { checked: boolean; ok: boolean; current: boolean; message?: string }
}
export type ApiReviewer = { group_id: number; group_name: string; level: 'view' | 'edit'; leader_name?: string; decision: 'pending' | 'approved' | 'rejected'; note?: string; decided_by?: string; decided_at?: string }
export type ApiExecution = {
  id: number; request_id: number; migration_id: number; migration_name: string; action: 'up' | 'rollback' | 'forward' | 'resume'
  reverts_execution_id?: number; migration_version: number; sql_executed?: string; status: 'running' | 'success' | 'failed' | 'partial'
  error?: string; attempt: number; executed_by_name?: string; started_at: string; finished_at?: string; duration_ms?: number
  rows_affected?: number; batch_size?: number; committed_batches: number; committed_rows: number; total_batches?: number
}
export type ApiRequest = {
  id: number; migration_id: number; action: 'up' | 'rollback' | 'forward' | 'resume'; target_execution_id?: number; revision: number
  status: 'pending_review' | 'approved' | 'rejected' | 'canceled' | 'queued' | 'running' | 'success' | 'failed' | 'partial'
  note?: string; error?: string; requested_by?: number; requested_by_name?: string; created_at: string; decided_at?: string
  queued_at?: string; started_at?: string; finished_at?: string; job_attempts?: number
  reviewers?: ApiReviewer[]; executions?: ApiExecution[]; can_approve: boolean; can_cancel: boolean
}
export type ApiBoard = { migrations: ApiMigration[]; requests: ApiRequest[]; leader_group_ids: number[]; can_edit: boolean }

/** Draft SPA ditambah konfigurasi batch backend (opsional). */
export type ApiDraftInput = DraftInput

export const migrationRowId = (id: number) => `draft-m${id}`
export const requestRowId = (id: number) => `draft-r${id}`
export const executionRef = (id: number) => `E${id}`
const jobRef = (requestId: number) => `JOB-${requestId}`
const versionLabel = (version: number) => `v${String(version).padStart(3, '0')}`

function parseRowId(id: string): { kind: 'migration' | 'request'; id: number } {
  const match = /^draft-([mr])(\d+)$/.exec(id)
  if (!match) throw new Error('Migrasi tidak ditemukan pada database ini.')
  return { kind: match[1] === 'm' ? 'migration' : 'request', id: Number(match[2]) }
}

function reviewRound(request: ApiRequest, revision: number): ReviewRound {
  return { id: `review-${request.id}`, revision, submittedAt: request.created_at, groups: (request.reviewers ?? []).map((group): ReviewGroup => ({
    id: String(group.group_id), name: group.group_name, permission: group.level, status: group.decision,
    ...(group.decision !== 'pending' ? { actor: group.decided_by || group.leader_name || '-', reason: group.note || '-', decidedAt: group.decided_at ?? request.decided_at ?? request.created_at } : {}),
  })) }
}

const queuedStatuses = new Set(['queued', 'running', 'success', 'failed', 'partial'])

function jobFor(request: ApiRequest, attempt: number): MigrationJob | undefined {
  const executions = request.executions ?? []
  const last = executions.at(-1)
  if (!queuedStatuses.has(request.status) && !(request.status === 'canceled' && request.queued_at)) return undefined
  const status: MigrationJob['status'] = request.status === 'partial' ? 'failed' : request.status === 'canceled' ? 'canceled' : request.status as MigrationJob['status']
  const batched = !!last?.batch_size
  const total = batched ? Math.max(last?.total_batches ?? 0, last?.committed_batches ?? 0, 1) : 1
  const completed = batched ? last?.committed_batches ?? 0 : status === 'success' ? 1 : 0
  return {
    id: jobRef(request.id), executionId: last ? executionRef(last.id) : `E-pending-${request.id}`, status,
    queuedAt: request.queued_at ?? request.created_at, scheduledAt: request.queued_at ?? request.created_at,
    ...(request.started_at ?? last?.started_at ? { startedAt: request.started_at ?? last?.started_at } : {}),
    ...(request.finished_at ?? last?.finished_at ? { finishedAt: request.finished_at ?? last?.finished_at } : {}),
    actor: request.requested_by_name ?? '-', totalBatches: total, completedBatches: completed,
    outcome: status === 'success' ? 'success' : 'failed', attempt,
    ...(request.error || last?.error ? { error: request.error || last?.error } : {}),
  }
}

function requestStatus(request: ApiRequest | undefined): WorkflowStatus {
  if (!request || request.status === 'canceled') return 'draft'
  return request.status === 'partial' ? 'failed' : request.status
}

function draftInput(m: ApiMigration): DraftInput {
  const irreversible = !!m.irreversible_reason
  return { name: m.name, sqlUp: m.up_sql ?? '', sqlDown: irreversible ? '' : m.down_sql ?? '', irreversible, ...(m.batch ? { batch: { ...m.batch } } : {}) }
}

/**
 * Board backend → baris workflow SPA. Satu baris per migration (pengajuan
 * "up") dan satu baris per pengajuan rollback/forward/resume.
 */
export function boardRows(board: ApiBoard, connectionId: string, ownerId: string): MigrationDraft[] {
  const byMigration = new Map(board.migrations.map(m => [m.id, m]))
  const chronological = [...board.requests].sort((a, b) => a.id - b.id)
  const executionOwner = new Map<number, { row: string; job: string }>()
  const rows: MigrationDraft[] = []
  for (const m of board.migrations) {
    const ups = chronological.filter(r => r.migration_id === m.id && r.action === 'up')
    const jobs = ups.map(r => jobFor(r, 0)).filter((job): job is MigrationJob => !!job).map((job, index) => ({ ...job, attempt: index + 1 }))
    for (const r of ups) for (const e of r.executions ?? []) executionOwner.set(e.id, { row: migrationRowId(m.id), job: jobRef(r.id) })
    const latest = ups.at(-1)
    rows.push({ ...draftInput(m), id: migrationRowId(m.id), connectionId, ownerId, author: m.created_by_name ?? '-', version: versionLabel(m.version),
      status: requestStatus(latest), createdAt: m.created_at, updatedAt: latest?.finished_at ?? latest?.created_at ?? m.updated_at,
      ...(ups.length ? { reviews: ups.map((r, index) => reviewRound(r, index + 1)) } : {}), ...(jobs.length ? { jobs } : {}) })
  }
  for (const r of chronological.filter(item => item.action !== 'up')) {
    const m = byMigration.get(r.migration_id)
    if (!m) continue
    const target = r.target_execution_id ? board.requests.flatMap(item => item.executions ?? []).find(e => e.id === r.target_execution_id) : undefined
    const owner = target ? executionOwner.get(target.id) : undefined
    const kind: OperationKind = r.action === 'rollback' ? 'rollback' : r.action === 'resume' ? 'resume' : 'forward'
    const input = draftInput(m)
    const reverse = kind === 'rollback'
    const job = jobFor(r, 1)
    const intent: OperationIntent = { kind, originDraftId: migrationRowId(m.id), targetDraftId: owner?.row ?? migrationRowId(m.id), targetJobId: owner?.job ?? '', targetExecutionId: target ? executionRef(target.id) : '',
      checkpoint: target?.committed_batches ?? 0, totalBatches: Math.max(target?.total_batches ?? 1, 1) }
    for (const e of r.executions ?? []) executionOwner.set(e.id, { row: requestRowId(r.id), job: jobRef(r.id) })
    rows.push({ id: requestRowId(r.id), connectionId, ownerId, author: r.requested_by_name ?? '-', version: versionLabel(m.version),
      name: `${operationLabels[kind]} · ${m.name}`, sqlUp: reverse ? input.sqlDown : input.sqlUp, sqlDown: reverse ? input.sqlUp : input.sqlDown, irreversible: !reverse && input.irreversible,
      status: r.status === 'canceled' ? 'rejected' : requestStatus(r), createdAt: r.created_at, updatedAt: r.finished_at ?? r.created_at, intent,
      reviews: [reviewRound(r, 1)], ...(job ? { jobs: [job] } : {}) })
  }
  return rows.sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
}

function durationLabel(ms?: number) { return ms === undefined ? '-' : `${(ms / 1000).toLocaleString('id-ID')} detik` }

/** Execution backend → baris riwayat SPA (terbaru dulu). */
export function boardExecutions(board: ApiBoard): Execution[] {
  const byMigration = new Map(board.migrations.map(m => [m.id, m]))
  return board.requests.flatMap(r => r.executions ?? []).filter(e => e.status !== 'running').map((e): Execution => {
    const m = byMigration.get(e.migration_id)
    const action = e.action === 'rollback' ? 'ROLLBACK' as const : e.action === 'forward' ? 'FORWARD' as const : 'UP' as const
    return { id: executionRef(e.id), version: versionLabel(e.migration_version), name: e.migration_name, action,
      ...(e.reverts_execution_id ? { rollbackTarget: executionRef(e.reverts_execution_id) } : {}),
      actor: e.executed_by_name ?? '-', executedAt: e.finished_at ?? e.started_at, status: e.status === 'success' ? 'success' : 'failed',
      duration: durationLabel(e.duration_ms), sql: e.sql_executed ?? '', irreversible: !!m?.irreversible_reason,
      ...(e.error ? { error: e.error } : e.status === 'partial' ? { error: `Berhenti setelah ${e.committed_batches} batch tersimpan; lanjutkan dengan Resume atau batalkan dengan Rollback.` } : {}) }
  }).sort((a, b) => b.executedAt.localeCompare(a.executedAt))
}

function friendly(error: unknown): never {
  if (error instanceof ApiError && error.kind === 'http' && error.status === 403 && !error.code) throw new Error('Anda tidak memiliki izin untuk aksi ini.')
  throw error
}

export function createApiDraftSource({ api, ownerId }: { api: Api; ownerId: string }) {
  const reviewerGroupIds: string[] = []
  const base = (connectionId: string) => `/databases/${encodeURIComponent(connectionId)}`
  async function board(connectionId: string, signal: AbortSignal) {
    const data = await api.request<ApiBoard>(`${base(connectionId)}/workflow`, { signal })
    reviewerGroupIds.splice(0, reviewerGroupIds.length, ...data.leader_group_ids.map(String))
    return data
  }
  async function list(connectionId: string, signal: AbortSignal) { return boardRows(await board(connectionId, signal), connectionId, ownerId) }
  async function find(connectionId: string, rowId: string, signal: AbortSignal) {
    const row = (await list(connectionId, signal)).find(item => item.id === rowId)
    if (!row) throw new Error('Migrasi tidak ditemukan pada database ini.')
    return row
  }
  async function latestRequest(connectionId: string, rowId: string, signal: AbortSignal) {
    const ref = parseRowId(rowId)
    if (ref.kind === 'request') return ref.id
    const data = await board(connectionId, signal)
    const request = data.requests.filter(r => r.migration_id === ref.id && r.action === 'up').sort((a, b) => b.id - a.id)[0]
    if (!request) throw new Error('Migrasi ini belum diajukan.')
    return request.id
  }
  async function submitUp(connectionId: string, migrationId: number, signal: AbortSignal, note = '') {
    const path = `${base(connectionId)}/migrations/${migrationId}`
    const validation = await api.request<{ ok: boolean; message?: string }>(`${path}/validate`, { method: 'POST', json: {}, signal })
    if (!validation.ok) throw new Error(validation.message || 'Dry-run gagal; perbaiki SQL sebelum diajukan.')
    await api.request(`${path}/requests`, { method: 'POST', json: { action: 'up', note }, signal })
  }
  return {
    reviewerGroupIds,
    list,
    history: async (connectionId: string, signal: AbortSignal) => boardExecutions(await board(connectionId, signal)),
    // Versi diberikan server saat migration dibuat (urut waktu pembuatan).
    async nextVersion() { return 'otomatis' },
    async save(connectionId: string, input: ApiDraftInput, signal: AbortSignal, draftId?: string) {
      const fields = validateDraft(input)
      if (Object.keys(fields).length) throw new DraftValidationError(fields)
      const body = { name: input.name.trim(), up_sql: input.sqlUp, down_sql: input.irreversible ? '' : input.sqlDown, irreversible: input.irreversible,
        irreversible_reason: input.irreversible ? 'Ditandai irreversible oleh pembuat migrasi.' : '', ...(input.batch ? { batch: input.batch } : {}) }
      try {
        const saved = draftId
          ? await api.request<ApiMigration>(`${base(connectionId)}/migrations/${parseRowId(draftId).id}`, { method: 'PUT', json: body, signal })
          : await api.request<ApiMigration>(`${base(connectionId)}/migrations`, { method: 'POST', json: body, signal })
        return await find(connectionId, migrationRowId(saved.id), signal)
      } catch (error) { return friendly(error) }
    },
    async transition(connectionId: string, draftId: string, action: WorkflowAction, signal: AbortSignal) {
      try {
        const ref = parseRowId(draftId)
        if (action.type === 'submit' || action.type === 'retry') {
          if (ref.kind !== 'migration') throw new Error('Ajukan ulang pemulihan dari riwayat eksekusi.')
          await submitUp(connectionId, ref.id, signal)
        } else if (action.type === 'decide') {
          const requestId = await latestRequest(connectionId, draftId, signal)
          const note = action.reason.trim()
          if (!note || note.length > 1000) throw new Error('Alasan wajib diisi, maksimal 1.000 karakter.')
          await api.request(`/migration-requests/${requestId}/${action.decision === 'approved' ? 'approve' : 'reject'}`, { method: 'POST', json: { note }, signal })
        } else if (action.type === 'cancel') {
          await api.request(`/migration-requests/${await latestRequest(connectionId, draftId, signal)}/cancel`, { method: 'POST', json: {}, signal })
        } else {
          throw new Error('Migrasi otomatis masuk antrean setelah semua grup menyetujui.')
        }
        return await find(connectionId, draftId, signal)
      } catch (error) { return friendly(error) }
    },
    async requestOperation(connectionId: string, executionId: string, kind: OperationKind, signal: AbortSignal) {
      try {
        const data = await board(connectionId, signal)
        const target = data.requests.flatMap(r => r.executions ?? []).find(e => executionRef(e.id) === executionId)
        if (!target) throw new Error('Snapshot eksekusi tidak tersedia pada database ini.')
        // Compensation = membalik batch yang sudah tersimpan, yaitu rollback execution partial.
        const action = kind === 'compensation' ? 'rollback' : kind
        const created = await api.request<ApiRequest>(`${base(connectionId)}/migrations/${target.migration_id}/requests`, { method: 'POST', json: { action }, signal })
        return await find(connectionId, requestRowId(created.id), signal)
      } catch (error) { return friendly(error) }
    },
  }
}
export type ApiDraftSource = ReturnType<typeof createApiDraftSource>
