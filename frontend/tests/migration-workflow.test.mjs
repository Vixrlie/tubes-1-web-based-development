import test from 'node:test'
import assert from 'node:assert/strict'
import { createMockDraftSource, clearMockDraftStorage } from '../src/features/migrations/draft-source.ts'
import { allApproved, batchDurationMs, jobEnd, latestJob, latestReview, reviewGroups, workflowExecutions } from '../src/features/migrations/workflow-model.ts'
import { paths } from '../src/app/routes.ts'
import { withMigrationSearch } from '../src/features/migrations/migration-source.ts'

function setup(overrides = {}) {
  const values = new Map()
  const storage = { get length() { return values.size }, key: i => [...values.keys()][i] ?? null, getItem: key => values.get(key) ?? null, setItem: (key, value) => { values.set(key, value) }, removeItem: key => { values.delete(key) } }
  let clock = Date.parse('2026-11-22T12:00:00Z')
  let active = true
  const options = { ownerId: '7', author: 'arina', storage: () => storage, delay: 0, now: () => clock, isActive: () => active, ...overrides }
  return { storage, values, source: createMockDraftSource(options), make: other => createMockDraftSource({ ...options, ...other }), setTime: value => { clock = value }, expire: () => { active = false } }
}
const signal = () => new AbortController().signal
const input = { name: 'add_product_status', sqlUp: "ALTER TABLE products ADD COLUMN status TEXT DEFAULT 'active';", sqlDown: 'ALTER TABLE products DROP COLUMN status;', irreversible: false }
async function submitted(source, connectionId = 'mock-shop') {
  const row = await source.save(connectionId, input, signal())
  return source.transition(connectionId, row.id, { type: 'submit' }, signal())
}
async function approve(source, row, connectionId = 'mock-shop') {
  for (const group of reviewGroups) row = await source.transition(connectionId, row.id, { type: 'decide', reviewId: latestReview(row).id, groupId: group.id, decision: 'approved', reason: `SQL reviewed by ${group.name}` }, signal())
  return row
}

test('submit locks SQL and starts all group approvals without silently approving any group', async () => {
  const { source } = setup()
  const row = await submitted(source)
  assert.equal(row.status, 'pending_review')
  assert.equal(latestReview(row).revision, 1)
  assert.equal(latestReview(row).groups.length, 3)
  assert.ok(latestReview(row).groups.every(group => group.status === 'pending'))
  await assert.rejects(source.save('mock-shop', { ...input, sqlUp: 'changed' }, signal(), row.id), /SQL terkunci/)
  await assert.rejects(source.transition('mock-shop', row.id, { type: 'submit' }, signal()), /Hanya draft/)
  await assert.rejects(source.transition('mock-shop', row.id, { type: 'enqueue' }, signal()), /Semua grup/)
})

test('approve and reject require a reason and decisions cannot be overwritten or queued early', async () => {
  const { source } = setup()
  let row = await submitted(source)
  const action = { type: 'decide', reviewId: latestReview(row).id, groupId: 'backend', decision: 'approved', reason: '' }
  for (const decision of ['approved', 'rejected']) await assert.rejects(source.transition('mock-shop', row.id, { ...action, decision }, signal()), /Alasan wajib/)
  await assert.rejects(source.transition('mock-shop', row.id, { ...action, reason: 'x'.repeat(1001) }, signal()), /Alasan wajib/)
  row = await source.transition('mock-shop', row.id, { ...action, reason: '  SQL checked  ' }, signal())
  assert.equal(row.status, 'pending_review')
  assert.equal(latestReview(row).groups[0].reason, 'SQL checked')
  await assert.rejects(source.transition('mock-shop', row.id, { ...action, reason: 'overwrite' }, signal()), /sudah dicatat/)
  await assert.rejects(source.transition('mock-shop', row.id, { type: 'enqueue' }, signal()), /Semua grup/)
})

test('rejection stops the round; revision resubmission resets every approval and retains reasons', async () => {
  const { source } = setup()
  let row = await submitted(source)
  const oldReview = latestReview(row).id
  row = await source.transition('mock-shop', row.id, { type: 'decide', reviewId: oldReview, groupId: 'analytics', decision: 'rejected', reason: 'Add a safer rollback.' }, signal())
  assert.equal(row.status, 'rejected')
  await assert.rejects(source.transition('mock-shop', row.id, { type: 'enqueue' }, signal()), /Semua grup/)
  row = await source.save('mock-shop', { ...input, name: 'revised_change' }, signal(), row.id)
  assert.equal(row.status, 'draft')
  row = await source.transition('mock-shop', row.id, { type: 'submit' }, signal())
  assert.equal(latestReview(row).revision, 2)
  assert.ok(latestReview(row).groups.every(group => group.status === 'pending'))
  assert.equal(row.reviews[0].groups[1].reason, 'Add a safer rollback.')
  await assert.rejects(source.transition('mock-shop', row.id, { type: 'decide', reviewId: oldReview, groupId: 'backend', decision: 'approved', reason: 'stale modal' }, signal()), /Review sudah berubah/)
})

test('all groups must approve; duplicate enqueue and out-of-state retries are rejected', async () => {
  const { source } = setup()
  let row = await submitted(source)
  row = await approve(source, row)
  assert.equal(row.status, 'approved')
  assert.equal(allApproved(row), true)
  assert.equal(row.jobs, undefined)
  row = await source.transition('mock-shop', row.id, { type: 'enqueue' }, signal())
  assert.equal(row.status, 'queued')
  assert.equal(latestJob(row).completedBatches, 0)
  await assert.rejects(source.transition('mock-shop', row.id, { type: 'enqueue' }, signal()), /Semua grup/)
  await assert.rejects(source.transition('mock-shop', row.id, { type: 'retry' }, signal()), /Semua grup/)
})

test('FIFO queue serializes jobs per database and refresh reconstructs progress without creating another job', async () => {
  const setupState = setup()
  const { source, setTime, make } = setupState
  const first = await approve(source, await submitted(source))
  const second = await approve(source, await submitted(source))
  const a = await source.transition('mock-shop', first.id, { type: 'enqueue' }, signal())
  const b = await source.transition('mock-shop', second.id, { type: 'enqueue' }, signal())
  assert.ok(Date.parse(latestJob(b).scheduledAt) > jobEnd(latestJob(a)))
  setTime(Date.parse(latestJob(a).scheduledAt) + 10 * batchDurationMs)
  let rows = await make({}).list('mock-shop', signal())
  assert.equal(rows.find(row => row.id === first.id).status, 'running')
  assert.equal(latestJob(rows.find(row => row.id === first.id)).completedBatches, 10)
  assert.equal(rows.find(row => row.id === second.id).status, 'queued')
  assert.equal(workflowExecutions(rows).length, 0)
  setTime(jobEnd(latestJob(b)))
  rows = await make({}).list('mock-shop', signal())
  assert.ok(rows.every(row => row.status === 'success'))
  assert.equal(rows.flatMap(row => row.jobs).length, 2)
  assert.equal(workflowExecutions(rows).length, 2)
  assert.equal(new Set(workflowExecutions(rows).map(row => row.id)).size, 2)
  assert.ok(workflowExecutions(rows).every(row => row.sql === input.sqlUp))
  assert.equal(workflowExecutions(await source.list('mock-shop', signal())).length, 2)
})

test('failure reaches a bounded checkpoint; retry preserves prior result and completes with a new ID', async () => {
  const state = setup({ outcome: 'failed', failAfter: 0 })
  const { source, setTime, make } = state
  let row = await approve(source, await submitted(source))
  row = await source.transition('mock-shop', row.id, { type: 'enqueue' }, signal())
  const failedId = latestJob(row).id
  setTime(jobEnd(latestJob(row)))
  row = (await source.list('mock-shop', signal()))[0]
  assert.equal(row.status, 'failed')
  assert.equal(latestJob(row).completedBatches, 0)
  assert.equal(workflowExecutions([row])[0].status, 'failed')
  assert.ok(latestJob(row).error)
  const retrySource = make({ outcome: 'success' })
  row = await retrySource.transition('mock-shop', row.id, { type: 'retry' }, signal())
  assert.equal(row.jobs.length, 2)
  assert.notEqual(latestJob(row).id, failedId)
  assert.equal(latestJob(row).attempt, 2)
  setTime(jobEnd(latestJob(row)))
  row = (await retrySource.list('mock-shop', signal()))[0]
  assert.equal(row.status, 'success')
  assert.deepEqual(row.jobs.map(job => job.status), ['failed', 'success'])
})

test('cancel affects only queued jobs; a running job cannot be canceled from a stale dialog', async () => {
  const { source, setTime } = setup()
  let row = await approve(source, await submitted(source))
  row = await source.transition('mock-shop', row.id, { type: 'enqueue' }, signal())
  const canceledId = latestJob(row).id
  row = await source.transition('mock-shop', row.id, { type: 'cancel', jobId: canceledId }, signal())
  assert.equal(row.status, 'approved')
  assert.equal(latestJob(row).status, 'canceled')
  assert.equal(workflowExecutions([row]).length, 0)
  row = await source.transition('mock-shop', row.id, { type: 'enqueue' }, signal())
  setTime(Date.parse(latestJob(row).scheduledAt))
  await assert.rejects(source.transition('mock-shop', row.id, { type: 'cancel', jobId: latestJob(row).id }, signal()), /sudah berjalan/)
})

test('review capability is independent of SQL edit permission; view leader can approve only the assigned group', async () => {
  const { source, make } = setup()
  const row = await submitted(source)
  const viewer = make({ preview: 'readonly' })
  const base = { type: 'decide', reviewId: latestReview(row).id, decision: 'approved', reason: 'Reviewed from analytics.' }
  await assert.rejects(viewer.transition('mock-shop', row.id, { ...base, groupId: 'backend' }, signal()), /izin review/)
  const reviewed = await viewer.transition('mock-shop', row.id, { ...base, groupId: 'analytics' }, signal())
  assert.equal(latestReview(reviewed).groups[1].status, 'approved')
  await assert.rejects(viewer.transition('mock-shop', row.id, { type: 'enqueue' }, signal()), /hanya memiliki akses/)
})

test('foreign accounts, connections, denied access and expired sessions cannot act on a review', async () => {
  const { source, make, expire } = setup()
  const row = await submitted(source)
  const action = { type: 'decide', reviewId: latestReview(row).id, groupId: 'backend', decision: 'approved', reason: 'Review checked.' }
  await assert.rejects(make({ ownerId: '8' }).transition('mock-shop', row.id, action, signal()), /tidak ditemukan/)
  await assert.rejects(source.transition('mock-inventory', row.id, action, signal()), /tidak ditemukan/)
  await assert.rejects(make({ preview: 'denied' }).transition('mock-shop', row.id, action, signal()), error => error.code === 'denied')
  expire()
  await assert.rejects(source.transition('mock-shop', row.id, action, signal()), /Sesi sudah berubah/)
})

test('aborted requests, concurrent duplicate decisions and failed storage writes cannot produce extra transitions', async () => {
  const state = setup()
  const { source, make, storage } = state
  const row = await submitted(source)
  const action = { type: 'decide', reviewId: latestReview(row).id, groupId: 'backend', decision: 'approved', reason: 'Checked.' }
  const controller = new AbortController()
  const pending = make({ delay: 20 }).transition('mock-shop', row.id, action, controller.signal)
  controller.abort()
  await assert.rejects(pending, { name: 'AbortError' })
  const broken = make({ storage: () => ({ ...storage, setItem() { throw new Error('quota') } }) })
  await assert.rejects(broken.transition('mock-shop', row.id, action, signal()), /belum dapat disimpan/)
  assert.ok(latestReview((await source.list('mock-shop', signal()))[0]).groups.every(group => group.status === 'pending'))
  const results = await Promise.allSettled([source.transition('mock-shop', row.id, action, signal()), source.transition('mock-shop', row.id, action, signal())])
  assert.equal(results.filter(result => result.status === 'fulfilled').length, 1)
})

test('workflow storage cleanup removes reviews and jobs together with their draft', async () => {
  const { source, storage } = setup()
  const row = await approve(source, await submitted(source))
  await source.transition('mock-shop', row.id, { type: 'enqueue' }, signal())
  clearMockDraftStorage(storage)
  assert.deepEqual(await source.list('mock-shop', signal()), [])
})

test('inconsistent persisted workflow cannot bypass group approval or forge a finished execution', async () => {
  const { source, values } = setup()
  await submitted(source)
  const key = [...values.keys()][0]
  const original = values.get(key)
  const rows = JSON.parse(original)
  rows[0].status = 'approved'
  values.set(key, JSON.stringify(rows))
  await assert.rejects(source.list('mock-shop', signal()), /tidak valid/)
  values.set(key, original)
  let row = await approve(source, (await source.list('mock-shop', signal()))[0])
  row = await source.transition('mock-shop', row.id, { type: 'enqueue' }, signal())
  const forged = JSON.parse(values.get(key))
  forged[0].status = 'success'
  forged[0].jobs[0].status = 'success'
  values.set(key, JSON.stringify(forged))
  await assert.rejects(source.list('mock-shop', signal()), /tidak valid/)
})

test('workflow routes encode IDs and carry independent list filters and failure preview', () => {
  assert.equal(paths.review('db /1', 'draft /x'), '/databases/db%20%2F1/migrations/reviews/draft%20%2Fx')
  assert.equal(paths.job('db /1', 'JOB /x'), '/databases/db%20%2F1/migrations/queue/JOB%20%2Fx')
  const params = new URLSearchParams('queueStatus=running&reviewStatus=approved&jobOutcome=failed')
  assert.equal(withMigrationSearch(paths.queue('mock-shop'), params), '/databases/mock-shop/migrations/queue?jobOutcome=failed&reviewStatus=approved&queueStatus=running')
})
