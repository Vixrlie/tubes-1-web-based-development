import test from 'node:test'
import assert from 'node:assert/strict'
import { createMockDraftSource } from '../src/features/migrations/draft-source.ts'
import { activeExecutions, operationEligibility } from '../src/features/migrations/operation-model.ts'
import { batchDurationMs, jobEnd, latestJob, latestReview, reviewGroups, workflowExecutions } from '../src/features/migrations/workflow-model.ts'
import { filterExecutions } from '../src/features/migrations/migration-source.ts'

const signal = () => new AbortController().signal
const input = { name: 'add_status', sqlUp: "ALTER TABLE products ADD COLUMN status TEXT DEFAULT 'active';", sqlDown: 'ALTER TABLE products DROP COLUMN status;', irreversible: false }
function setup() {
  const values = new Map()
  const storage = { get length() { return values.size }, key: index => [...values.keys()][index] ?? null, getItem: key => values.get(key) ?? null, setItem: (key, value) => { values.set(key, value) }, removeItem: key => { values.delete(key) } }
  let clock = Date.parse('2026-11-22T12:00:00Z')
  let active = true
  const options = { ownerId: '7', author: 'arina', storage: () => storage, delay: 0, now: () => clock, isActive: () => active }
  const source = createMockDraftSource(options)
  return { source, storage, make: overrides => createMockDraftSource({ ...options, ...overrides }), setTime: time => { clock = time }, expire: () => { active = false } }
}
async function approved(source, row) {
  if (row.status === 'draft') row = await source.transition('mock-shop', row.id, { type: 'submit' }, signal())
  for (const group of reviewGroups) row = await source.transition('mock-shop', row.id, { type: 'decide', reviewId: latestReview(row).id, groupId: group.id, decision: 'approved', reason: 'SQL and checkpoint reviewed.' }, signal())
  return row
}
async function finished(state, source = state.source, value = input) {
  let row = await source.save('mock-shop', value, signal())
  row = await approved(source, row)
  row = await source.transition('mock-shop', row.id, { type: 'enqueue' }, signal())
  state.setTime(jobEnd(latestJob(row)))
  return (await source.list('mock-shop', signal())).find(item => item.id === row.id)
}
async function finishRequest(state, row) {
  row = await approved(state.source, row)
  row = await state.source.transition('mock-shop', row.id, { type: 'enqueue' }, signal())
  state.setTime(jobEnd(latestJob(row)))
  return (await state.source.list('mock-shop', signal())).find(item => item.id === row.id)
}

test('rollback uses immutable SQL Down, requires all approvals again and forward restores SQL Up', async () => {
  const state = setup()
  const main = await finished(state)
  const mainExecution = latestJob(main).executionId
  let rollback = await state.source.requestOperation('mock-shop', mainExecution, 'rollback', signal())
  assert.equal(rollback.sqlUp, input.sqlDown)
  assert.equal(rollback.sqlDown, input.sqlUp)
  assert.equal(rollback.version, main.version)
  assert.equal(rollback.status, 'pending_review')
  assert.ok(latestReview(rollback).groups.every(group => group.status === 'pending'))
  await assert.rejects(state.source.transition('mock-shop', rollback.id, { type: 'enqueue' }, signal()), /Semua grup/)
  await assert.rejects(state.source.save('mock-shop', { ...input, sqlUp: 'changed' }, signal(), rollback.id), /SQL terkunci/)
  rollback = await finishRequest(state, rollback)
  let rows = await state.source.list('mock-shop', signal())
  assert.equal(activeExecutions(rows).length, 0)
  const records = workflowExecutions(rows)
  const record = records.find(item => item.id === latestJob(rollback).executionId)
  assert.equal(record.action, 'ROLLBACK')
  assert.equal(record.rollbackTarget, mainExecution)
  assert.equal(record.sql, input.sqlDown)
  assert.equal(filterExecutions(records, true).length, 0)
  const forward = await state.source.requestOperation('mock-shop', latestJob(rollback).executionId, 'forward', signal())
  assert.equal(forward.sqlUp, input.sqlUp)
  const result = await finishRequest(state, forward)
  rows = await state.source.list('mock-shop', signal())
  assert.equal(activeExecutions(rows).at(-1).job.id, latestJob(result).id)
  assert.equal(workflowExecutions(rows).find(item => item.id === latestJob(result).executionId).action, 'FORWARD')
})

test('only latest active reversible execution can roll back, and stale approved rollback cannot enqueue', async () => {
  const state = setup()
  const first = await finished(state)
  let request = await state.source.requestOperation('mock-shop', latestJob(first).executionId, 'rollback', signal())
  request = await approved(state.source, request)
  const second = await finished(state, state.source, { ...input, name: 'second_change' })
  const rows = await state.source.list('mock-shop', signal())
  assert.equal(operationEligibility(rows, latestJob(first).executionId, 'rollback', request.id).allowed, false)
  await assert.rejects(state.source.transition('mock-shop', request.id, { type: 'enqueue' }, signal()), /aktif terakhir/)
  assert.equal(operationEligibility(rows, latestJob(second).executionId, 'rollback').allowed, true)
})

test('irreversible rollback and duplicate or foreign recovery requests are rejected', async () => {
  const state = setup()
  const irreversible = await finished(state, state.source, { ...input, irreversible: true, sqlDown: '' })
  await assert.rejects(state.source.requestOperation('mock-shop', latestJob(irreversible).executionId, 'rollback', signal()), /irreversible/)
  const main = await finished(state)
  const execution = latestJob(main).executionId
  await assert.rejects(state.make({ preview: 'readonly' }).requestOperation('mock-shop', execution, 'rollback', signal()), /akses Edit/)
  await assert.rejects(state.source.requestOperation('mock-inventory', execution, 'rollback', signal()), /tidak tersedia/)
  await assert.rejects(state.make({ ownerId: '8' }).requestOperation('mock-shop', execution, 'rollback', signal()), /tidak tersedia/)
  await state.source.requestOperation('mock-shop', execution, 'rollback', signal())
  await assert.rejects(state.source.requestOperation('mock-shop', execution, 'rollback', signal()), /Masih ada permintaan/)
})

test('partial failure resumes from checkpoint without replaying committed batches or reusing approval', async () => {
  const state = setup()
  const failed = await finished(state, state.make({ outcome: 'failed' }))
  assert.equal(latestJob(failed).completedBatches, 42)
  await assert.rejects(state.source.transition('mock-shop', failed.id, { type: 'retry' }, signal()), /Resume atau Compensation/)
  let request = await state.source.requestOperation('mock-shop', latestJob(failed).executionId, 'resume', signal())
  assert.equal(request.intent.checkpoint, 42)
  assert.equal(request.sqlUp, input.sqlUp)
  request = await approved(state.source, request)
  request = await state.source.transition('mock-shop', request.id, { type: 'enqueue' }, signal())
  assert.equal(latestJob(request).completedBatches, 42)
  const start = Date.parse(latestJob(request).scheduledAt)
  state.setTime(start + batchDurationMs)
  request = (await state.source.list('mock-shop', signal())).find(row => row.id === request.id)
  assert.equal(latestJob(request).completedBatches, 43)
  state.setTime(jobEnd(latestJob(request)))
  request = (await state.source.list('mock-shop', signal())).find(row => row.id === request.id)
  assert.equal(request.status, 'success')
  assert.equal(latestJob(request).completedBatches, 80)
  await assert.rejects(state.source.requestOperation('mock-shop', latestJob(failed).executionId, 'compensation', signal()), /terbaru/)
})

test('compensation reverses only committed batches and permits forward once completed', async () => {
  const state = setup()
  const failed = await finished(state, state.make({ outcome: 'failed' }))
  let compensation = await state.source.requestOperation('mock-shop', latestJob(failed).executionId, 'compensation', signal())
  assert.equal(compensation.sqlUp, input.sqlDown)
  compensation = await finishRequest(state, compensation)
  assert.equal(latestJob(compensation).totalBatches, 42)
  assert.equal(latestJob(compensation).completedBatches, 42)
  const forward = await state.source.requestOperation('mock-shop', latestJob(compensation).executionId, 'forward', signal())
  assert.equal(forward.sqlUp, input.sqlUp)
})

test('aborted recovery and expired sessions do not persist a request', async () => {
  const state = setup()
  const main = await finished(state)
  const execution = latestJob(main).executionId
  const controller = new AbortController()
  const request = state.make({ delay: 20 }).requestOperation('mock-shop', execution, 'rollback', controller.signal)
  controller.abort()
  await assert.rejects(request, { name: 'AbortError' })
  assert.equal((await state.source.list('mock-shop', signal())).length, 1)
  state.expire()
  await assert.rejects(state.source.requestOperation('mock-shop', execution, 'rollback', signal()), /Sesi sudah berubah/)
})
