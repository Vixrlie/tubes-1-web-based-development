import test from 'node:test'
import assert from 'node:assert/strict'
import { createMockMigrationSource, filterExecutions, parseExecutionStatus, withMigrationSearch } from '../src/features/migrations/migration-source.ts'
import { paths } from '../src/app/routes.ts'

const signal = () => new AbortController().signal

test('history matches the six Figma executions and detail SQL is scoped to the connection', async () => {
  const source = createMockMigrationSource('ready', 'ready', 0)
  const rows = await source.list('mock-shop', signal())
  assert.deepEqual(rows.map(row => row.id), ['E1050', 'E1048', 'E1047', 'E1046', 'E1045', 'E1044'])
  assert.equal(rows[0].irreversible, true)
  assert.equal(rows[2].rollbackTarget, 'E1045')
  assert.equal((await source.detail('mock-shop', 'E1048', signal())).sql, "ALTER TABLE products ADD COLUMN status TEXT DEFAULT 'active';")
  assert.deepEqual(await source.list('mock-inventory', signal()), [])
  assert.deepEqual(await source.list('mock-analytics', signal()), [])
  assert.equal(await source.detail('mock-inventory', 'E1048', signal()), undefined)
  assert.equal(await source.detail('mock-shop', 'unknown', signal()), undefined)
})

test('hiding rollback removes both successful paired executions and preserves source order', async () => {
  const rows = await createMockMigrationSource('ready', 'ready', 0).list('mock-shop', signal())
  assert.deepEqual(filterExecutions(rows, true).map(row => row.id), ['E1050', 'E1048', 'E1046', 'E1044'])
  assert.equal(filterExecutions(rows, false).length, 6)
  assert.equal(rows.length, 6)
})

test('failed, orphan and unrelated-version rollbacks do not hide valid history', async () => {
  const rows = await createMockMigrationSource('ready', 'ready', 0).list('mock-shop', signal())
  const original = rows.find(row => row.id === 'E1045')
  const rollback = rows.find(row => row.id === 'E1047')
  assert.equal(filterExecutions([original, { ...rollback, status: 'failed' }], true).length, 2)
  assert.equal(filterExecutions([rollback], true).length, 1)
  assert.equal(filterExecutions([original, { ...rollback, version: 'v999' }], true).length, 2)
  assert.equal(filterExecutions([original, { ...rollback, action: 'UP' }], true).length, 2)
})

test('status filters compose with rollback filtering and invalid URL values fall back to all', async () => {
  const rows = await createMockMigrationSource('ready', 'ready', 0).list('mock-shop', signal())
  const failed = { ...rows[0], id: 'E1051', status: 'failed' }
  assert.deepEqual(filterExecutions([...rows, failed], true, 'failed').map(row => row.id), ['E1051'])
  assert.equal(filterExecutions([...rows, failed], true, 'success').length, 4)
  assert.equal(parseExecutionStatus('invalid'), 'all')
  assert.equal(parseExecutionStatus('failed'), 'failed')
})

test('read-only history remains viewable; consumer edits cannot mutate source fixtures', async () => {
  const source = createMockMigrationSource('ready', 'readonly', 0)
  const rows = await source.list('mock-shop', signal())
  rows[0].sql = 'changed'
  rows[2].rollbackTarget = 'changed'
  const restored = await source.list('mock-shop', signal())
  assert.equal(restored[0].sql, 'DROP TABLE archive;')
  assert.equal(restored[2].rollbackTarget, 'E1045')
})

test('empty/error previews, forbidden, unknown and disconnected connections are distinct', async () => {
  assert.deepEqual(await createMockMigrationSource('empty', 'ready', 0).list('mock-shop', signal()), [])
  await assert.rejects(createMockMigrationSource('error', 'ready', 0).list('mock-shop', signal()), /Riwayat migrasi/)
  const source = createMockMigrationSource('ready', 'ready', 0)
  await assert.rejects(source.list('unknown', signal()), error => error.code === 'not-found')
  await assert.rejects(source.list('mock-hr', signal()), error => error.code === 'unavailable')
  await assert.rejects(createMockMigrationSource('ready', 'denied', 0).list('mock-shop', signal()), error => error.code === 'denied')
})

test('navigation carries history filters and previews without leaking unrelated table params', () => {
  const params = new URLSearchParams('hideRollback=1&status=failed&preview=readonly&migrationPreview=empty&tab=data&page=2')
  const path = withMigrationSearch(paths.migration('db /x', 'E/1'), params)
  assert.equal(path, '/databases/db%20%2Fx/migrations/E%2F1?preview=readonly&migrationPreview=empty&hideRollback=1&status=failed')
  assert.equal(withMigrationSearch(paths.migrations('mock-shop'), new URLSearchParams()), '/databases/mock-shop/migrations')
})

test('canceling a migration history load prevents stale results', async () => {
  const controller = new AbortController()
  const source = createMockMigrationSource('ready', 'ready', 20)
  const pending = source.list('mock-shop', controller.signal)
  controller.abort()
  await assert.rejects(pending, { name: 'AbortError' })
  await assert.rejects(source.detail('mock-shop', 'E1048', controller.signal), { name: 'AbortError' })
})
