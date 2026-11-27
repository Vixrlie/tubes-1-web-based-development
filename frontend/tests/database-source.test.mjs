import test from 'node:test'
import assert from 'node:assert/strict'
import { createMockDatabaseSource, databasePreview, connectionErrorMessage } from '../src/features/databases/mock-source.ts'
import { databaseDescription } from '../src/features/databases/types.ts'

test('database fixtures preserve connection metadata and stay isolated between reads', async () => {
  const source = createMockDatabaseSource('ready', 0)
  const signal = new AbortController().signal
  const databases = await source.list(signal)
  assert.equal(databases.length, 4)
  assert.equal(databases.filter(item => item.status === 'connected').length, 3)
  const unavailable = databases.find(item => item.id === 'mock-hr')
  assert.equal(unavailable.permission, 'view')
  assert.equal(databaseDescription(unavailable), 'PostgreSQL')
  assert.equal(databaseDescription(databases[0]), 'PostgreSQL · 1 schema · 3 tabel')
  databases[0].name = 'changed by consumer'
  assert.equal((await source.list(signal))[0].name, 'shop_db')
})

test('mock reconnect never fabricates a successful connection for an unavailable database', async () => {
  const source = createMockDatabaseSource('ready', 0)
  const signal = new AbortController().signal
  await assert.rejects(source.reconnect('mock-hr', signal), { message: connectionErrorMessage })
  assert.equal((await source.list(signal)).find(item => item.id === 'mock-hr').status, 'disconnected')
})

test('mock data source cancels on navigation and never returns a cancelled list', async () => {
  const source = createMockDatabaseSource('ready', 20)
  const controller = new AbortController()
  const request = source.list(controller.signal)
  controller.abort()
  await assert.rejects(request, { name: 'AbortError' })
  await assert.rejects(source.list(controller.signal), { name: 'AbortError' })
})

test('empty/error preview states and unknown preview parameters are explicit', async () => {
  const signal = new AbortController().signal
  assert.deepEqual(await createMockDatabaseSource('empty', 0).list(signal), [])
  await assert.rejects(createMockDatabaseSource('error', 0).list(signal))
  assert.equal(databasePreview('loading'), 'loading')
  assert.equal(databasePreview('anything'), 'ready')
  assert.equal(databasePreview(null), 'ready')
})
