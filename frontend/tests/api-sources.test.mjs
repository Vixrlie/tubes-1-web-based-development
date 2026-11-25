import test from 'node:test'
import assert from 'node:assert/strict'
import { createApiClient } from '../src/shared/api/client.ts'
import { ApiError } from '../src/shared/api/api-error.ts'
import { usesMockData } from '../src/shared/api/data-mode.ts'
import { createApiDatabaseSource, createApiSchemaSource, createApiTableSource } from '../src/features/databases/api-source.ts'
import { SchemaSourceError } from '../src/features/databases/schema-source.ts'
import { boardExecutions, boardRows, createApiDraftSource, executedAtLabel } from '../src/features/migrations/api-workflow-source.ts'
import { explainPlan } from '../src/features/migrations/api-schema-tools.ts'
import { executionStatus, filterExecutions } from '../src/features/migrations/migration-source.ts'
import { jobBadgeStatus } from '../src/features/migrations/workflow-model.ts'
import { operationEligibility } from '../src/features/migrations/operation-model.ts'
import { validateDraft } from '../src/features/migrations/draft-source.ts'
import { parseAdminGroupDetail } from '../src/features/admin/admin-api.ts'

const signal = new AbortController().signal

/** Fake client: mencatat panggilan dan menjawab dari tabel route. */
function fakeApi(routes) {
  const calls = []
  return { calls, async request(path, options = {}) {
    calls.push({ path, method: options.method ?? 'GET', json: options.json, query: options.query })
    const handler = routes[`${options.method ?? 'GET'} ${path}`]
    if (!handler) throw new ApiError('not found', { status: 404, kind: 'http' })
    return typeof handler === 'function' ? handler(options) : structuredClone(handler)
  } }
}

test('mock data hanya dipakai untuk pratinjau development atau VITE_DATA_SOURCE=mock', () => {
  assert.equal(usesMockData(new URLSearchParams(''), { DEV: true }), false)
  assert.equal(usesMockData(new URLSearchParams('preview=error'), { DEV: true }), true)
  assert.equal(usesMockData(new URLSearchParams('preview=error'), { DEV: false }), false)
  assert.equal(usesMockData(new URLSearchParams(''), { DEV: false, VITE_DATA_SOURCE: 'mock' }), true)
})

test('pesan error workflow API berbentuk string tetap sampai ke UI', async () => {
  const api = createApiClient({ fetchImpl: async () => Response.json({ error: 'migration lain masih partial', code: 'database_held' }, { status: 409 }) })
  await assert.rejects(api.request('/databases'), error => error instanceof ApiError && error.message === 'migration lain masih partial' && error.code === 'database_held')
})

test('sumber database memetakan JSON API dan error akses', async () => {
  const api = fakeApi({
    'GET /databases': { items: [{ id: 7, name: 'shop', engine: 'PostgreSQL', status: 'connected', permission: 'edit', schema_count: 1, table_count: 3 }] },
    'GET /databases/7/schema': { database: { id: 7, name: 'shop', engine: 'PostgreSQL', status: 'connected', permission: 'view', schema_count: 1, table_count: 1 },
      schemas: [{ name: 'public', tables: [{ name: 'orders', column_count: 3, primary_key: ['id'], kind: 'table' }], relations: [{ name: 'fk', from_table: 'orders', from_column: 'user_id', to_schema: 'public', to_table: 'users', to_column: 'id' }] }] },
    'GET /databases/8/schema': () => { throw new ApiError('akses ditolak', { status: 403, kind: 'http', code: 'forbidden' }) },
    'GET /databases/7/tables/public/orders/rows': { columns: ['id'], rows: [{ id: 1 }], total: 30, page: 2, page_size: 25, pages: 2 },
  })
  const [db] = await createApiDatabaseSource(api).list(signal)
  assert.deepEqual(db, { id: '7', name: 'shop', engine: 'PostgreSQL', status: 'connected', permission: 'edit', schemaCount: 1, tableCount: 3 })
  const workspace = await createApiSchemaSource(api).load('7', signal)
  assert.equal(workspace.schemas[0].tables[0].permission, 'view')
  assert.equal(workspace.schemas[0].relations[0].toTable, 'users')
  await assert.rejects(createApiSchemaSource(api).load('8', signal), error => error instanceof SchemaSourceError && error.code === 'denied')
  const page = await createApiTableSource(api).rows({ connectionId: '7', schemaName: 'public', tableName: 'orders', page: 2, direction: 'desc' }, signal)
  assert.equal(page.pageSize, 25)
  assert.deepEqual(api.calls.at(-1).query, { page: 2, page_size: 25, direction: 'desc' })
})

const t0 = '2026-11-24T10:00:00Z'
const migration = (id, extra = {}) => ({ id, connection_id: 1, version: 20261124100000 + id, name: `m${id}`, up_sql: `UPDATE t SET v = v + 1 WHERE id BETWEEN $1 AND $2`, down_sql: 'UPDATE t SET v = v - 1 WHERE id BETWEEN $1 AND $2',
  revision: 1, created_by: 5, created_by_name: 'editor', created_at: t0, updated_at: t0, state: 'draft', validation: { checked: true, ok: true, current: true }, ...extra })
const reviewers = decision => [{ group_id: 1, group_name: 'tim-a', level: 'edit', decision, ...(decision !== 'pending' ? { note: 'ok', decided_by: 'leader', decided_at: t0 } : {}) }]
const execution = (id, requestId, extra = {}) => ({ id, request_id: requestId, migration_id: 2, migration_name: 'm2', action: 'up', migration_version: 20261124100002, sql_executed: 'UPDATE ...', status: 'success', attempt: 1,
  executed_by_name: 'editor', started_at: t0, finished_at: '2026-11-24T10:00:05Z', duration_ms: 5000, committed_batches: 0, committed_rows: 0, ...extra })

const board = {
  can_edit: true, leader_group_ids: [1],
  migrations: [migration(1), migration(2, { state: 'partial', batch: { table: 't', column: 'id', size: 10 } }), migration(3, { irreversible_reason: 'drop', down_sql: '' })],
  requests: [
    { id: 13, migration_id: 2, action: 'rollback', target_execution_id: 21, revision: 1, status: 'pending_review', created_at: '2026-11-24T10:10:00Z', reviewers: reviewers('pending'), executions: [], can_approve: true, can_cancel: true },
    { id: 12, migration_id: 2, action: 'up', revision: 1, status: 'partial', created_at: '2026-11-24T10:01:00Z', queued_at: '2026-11-24T10:01:01Z', requested_by_name: 'editor', reviewers: reviewers('approved'),
      executions: [execution(21, 12, { status: 'partial', batch_size: 10, committed_batches: 2, total_batches: 5, error: 'gagal di batch 3' })], can_approve: false, can_cancel: false },
    { id: 11, migration_id: 1, action: 'up', revision: 1, status: 'pending_review', created_at: '2026-11-24T10:00:30Z', reviewers: reviewers('pending'), executions: [], can_approve: true, can_cancel: true },
  ],
}

test('board backend dipetakan ke baris workflow SPA', () => {
  const rows = boardRows(board, '1', 'u9')
  const byId = Object.fromEntries(rows.map(row => [row.id, row]))
  assert.equal(byId['draft-m3'].status, 'draft')
  assert.equal(byId['draft-m3'].irreversible, true)
  assert.equal(byId['draft-m3'].sqlDown, '')
  assert.equal(byId['draft-m1'].status, 'pending_review')
  assert.equal(byId['draft-m1'].reviews[0].groups[0].status, 'pending')
  const partial = byId['draft-m2']
  assert.equal(partial.status, 'partial', 'partial tampil sebagai status sendiri, bukan gagal')
  assert.equal(partial.jobs[0].status, 'failed', 'logika recovery tetap memakai job gagal + checkpoint')
  assert.equal(partial.jobs[0].partial, 'applied')
  assert.equal(jobBadgeStatus(partial.jobs[0]), 'partial')
  assert.deepEqual(partial.batch, { table: 't', column: 'id', size: 10 })
  assert.equal(partial.jobs[0].completedBatches, 2)
  assert.equal(partial.jobs[0].totalBatches, 5)
  assert.equal(partial.jobs[0].executionId, 'E21')
  const rollback = byId['draft-r13']
  assert.equal(rollback.intent.kind, 'compensation', 'rollback atas eksekusi partial = kompensasi per batch')
  assert.equal(rollback.intent.targetExecutionId, 'E21')
  assert.equal(rollback.intent.targetDraftId, 'draft-m2')
  assert.equal(rollback.sqlUp, migration(2).down_sql)
  assert.ok(rows.every(row => row.ownerId === 'u9'), 'ownerId = user sesi agar hasil mutasi diterapkan')
})

test('execution backend menjadi riwayat; partial diberi label sendiri dengan pesan checkpoint', () => {
  const rows = boardExecutions(board)
  assert.equal(rows.length, 1)
  assert.equal(rows[0].status, 'failed')
  assert.equal(rows[0].partial, 'applied')
  assert.equal(executionStatus(rows[0].status, rows[0].partial), 'Partial')
  assert.equal(rows[0].error, 'gagal di batch 3')
  const withRollback = boardExecutions({ ...board, requests: [
    { ...board.requests[1], status: 'success', executions: [execution(21, 12)] },
    { ...board.requests[0], status: 'success', executions: [execution(22, 13, { action: 'rollback', reverts_execution_id: 21, finished_at: '2026-11-24T10:20:00Z' })] },
  ] })
  assert.equal(withRollback[0].action, 'ROLLBACK')
  assert.equal(withRollback[0].rollbackTarget, 'E21')
  assert.equal(filterExecutions(withRollback, true).length, 0, 'pasangan rollback disembunyikan')
})

test('eksekusi batch partial memenuhi syarat resume di SPA', () => {
  const rows = boardRows({ ...board, requests: board.requests.filter(r => r.id !== 13) }, '1', 'u9')
  assert.equal(operationEligibility(rows, 'E21', 'resume').allowed, true)
})

test('submit memvalidasi dry-run dulu lalu membuat request up', async () => {
  const api = fakeApi({
    'GET /databases/1/workflow': board,
    'POST /databases/1/migrations/3/validate': { ok: true },
    'POST /databases/1/migrations/3/requests': { id: 99 },
  })
  const source = createApiDraftSource({ api, ownerId: 'u9' })
  await source.transition('1', 'draft-m3', { type: 'submit' }, signal)
  assert.deepEqual(api.calls.filter(c => c.method === 'POST').map(c => c.path), ['/databases/1/migrations/3/validate', '/databases/1/migrations/3/requests'])
  assert.deepEqual(source.reviewerGroupIds, ['1'])

  const failing = fakeApi({ 'GET /databases/1/workflow': board, 'POST /databases/1/migrations/3/validate': { ok: false, message: 'syntax error' } })
  await assert.rejects(createApiDraftSource({ api: failing, ownerId: 'u9' }).transition('1', 'draft-m3', { type: 'submit' }, signal), { message: 'syntax error' })
  assert.equal(failing.calls.some(c => c.path.endsWith('/requests')), false)
})

test('keputusan review dan recovery memakai request backend yang tepat', async () => {
  const api = fakeApi({
    'GET /databases/1/workflow': board,
    'POST /migration-requests/11/approve': { id: 11 },
    'POST /databases/1/migrations/2/requests': ({ json }) => ({ id: 13, action: json.action }),
  })
  const source = createApiDraftSource({ api, ownerId: 'u9' })
  await source.transition('1', 'draft-m1', { type: 'decide', reviewId: 'review-11', groupId: '1', decision: 'approved', reason: 'aman' }, signal)
  assert.deepEqual(api.calls.find(c => c.path === '/migration-requests/11/approve').json, { note: 'aman' })
  const row = await source.requestOperation('1', 'E21', 'compensation', signal)
  assert.equal(api.calls.find(c => c.path === '/databases/1/migrations/2/requests').json.action, 'rollback')
  assert.equal(row.id, 'draft-r13')
})

test('validasi draft mode batch mewajibkan placeholder dan ukuran wajar', () => {
  const base = { name: 'x', sqlUp: 'UPDATE t SET v = 1', sqlDown: 'UPDATE t SET v = 0', irreversible: false }
  assert.ok(validateDraft({ ...base, batch: { table: 't', column: 'id', size: 10 } }).batch)
  assert.ok(validateDraft({ ...base, sqlUp: 'UPDATE t SET v = 1 WHERE id BETWEEN $1 AND $2', batch: { table: 't', column: 'id', size: 0 } }).batch)
  assert.equal(validateDraft({ ...base, sqlUp: 'UPDATE t SET v = 1 WHERE id BETWEEN $1 AND $2', batch: { table: 't', column: 'id', size: 10 } }).batch, undefined)
})

test('hasil EXPLAIN backend dipetakan ke metrik halaman', () => {
  const plan = explainPlan('SELECT 1', true, [{ n: 1, statement: 'SELECT 1', kind: 'select', plan: 'Seq Scan', analyze: true,
    root: { node: 'Seq Scan', relation: 'public.t', startup_cost: 0, total_cost: 12.5, rows: 100, width: 8 }, planning_ms: 0.2, execution_ms: 1.4 }])
  assert.equal(plan.scan.totalCost, 12.5)
  assert.equal(plan.executionMs, 1.4)
  assert.throws(() => explainPlan('ALTER TABLE t ADD c int', false, [{ n: 1, statement: 'ALTER', kind: 'ddl', note: 'Statement DDL tidak memiliki execution plan.', analyze: false, root: null, planning_ms: null, execution_ms: null }]), /DDL/)
})

test('detail grup admin memuat leader', () => {
  assert.equal(parseAdminGroupDetail({ id: '1', name: 'tim', leaderId: '5' }).leaderId, '5')
  assert.equal(parseAdminGroupDetail({ id: '1', name: 'tim', leaderId: null }).leaderId, null)
})

test('waktu eksekusi ditampilkan sebagai YYYY-MM-DD HH:mm', () => {
  assert.match(executedAtLabel('2026-11-24T10:00:05Z'), /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/)
  assert.equal(executedAtLabel('bukan tanggal'), 'bukan tanggal')
})

test('rollback batch yang berhenti di tengah tampil sebagai partially reverted', () => {
  const reverted = { ...board, requests: [
    { ...board.requests[0], status: 'partial', executions: [execution(22, 13, { action: 'rollback', status: 'partial', reverts_execution_id: 21, committed_batches: 1, total_batches: 2, batch_size: 10 })] },
    { ...board.requests[1], status: 'partial' },
  ] }
  const rows = boardRows(reverted, '1', 'u9')
  const rollback = rows.find(row => row.id === 'draft-r13')
  assert.equal(rollback.status, 'partially_reverted')
  assert.equal(jobBadgeStatus(rollback.jobs[0]), 'partially_reverted')
  const history = boardExecutions(reverted).find(row => row.id === 'E22')
  assert.equal(executionStatus(history.status, history.partial), 'Partially reverted')
})

test('migration partial yang sudah di-resume sampai selesai tidak lagi berlabel partial', () => {
  const resumed = { ...board, migrations: board.migrations.map(m => m.id === 2 ? { ...m, state: 'active' } : m) }
  const row = boardRows(resumed, '1', 'u9').find(item => item.id === 'draft-m2')
  assert.equal(row.status, 'success')
  assert.equal(row.jobs[0].partial, 'applied', 'job lama tetap tercatat partial di riwayat')
})

test('kompensasi yang berhenti sebagian dapat dilanjutkan, bukan di-resume', () => {
  const reverted = { ...board, requests: [
    { ...board.requests[0], status: 'partial', executions: [execution(22, 13, { action: 'rollback', status: 'partial', reverts_execution_id: 21, committed_batches: 1, total_batches: 2, batch_size: 10 })] },
    { ...board.requests[1], status: 'partial' },
  ] }
  const rows = boardRows(reverted, '1', 'u9')
  assert.equal(rows.find(row => row.id === 'draft-r13').intent.kind, 'compensation')
  assert.equal(operationEligibility(rows, 'E22', 'compensation').allowed, true)
  assert.equal(operationEligibility(rows, 'E22', 'resume').allowed, false)
  assert.equal(operationEligibility(rows, 'E21', 'resume').allowed, false, 'hanya eksekusi terbaru yang dapat dipulihkan')
})
