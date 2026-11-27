import test from 'node:test'
import assert from 'node:assert/strict'
import { createMockActivitySource, filterActivity, observabilityLinks } from '../src/features/admin/monitoring-source.ts'

const all = { search: '', status: 'all', category: 'all', from: '', to: '' }
const signal = () => new AbortController().signal

test('activity filtering correlates SQL results by request/trace/migration without changing the source', async () => {
  const rows = await createMockActivitySource('ready', 0).list(signal())
  const originalIds = rows.map(row => row.id)
  const related = filterActivity(rows, { ...all, search: '  REQ-MIGRATION-014  ' })
  assert.equal(related.length, 2)
  assert.ok(related.every(row => row.migrationId === '14'))
  assert.equal(filterActivity(rows, { ...all, search: related[0].traceId }).length, 2)
  assert.deepEqual(filterActivity(rows, { ...all, search: 'req-migration-014', status: 'failed' }).map(row => row.action), ['sql.result'])
  assert.deepEqual(rows.map(row => row.id), originalIds)
  assert.ok(filterActivity(rows, { ...all, category: 'user' }).every(row => row.action.startsWith('user.')))
  assert.ok(filterActivity(rows, { ...all, category: 'admin' }).some(row => row.action === 'admin.connection.create'))
})

test('date filtering uses WIB across midnight and includes both date boundaries', async () => {
  const rows = await createMockActivitySource('ready', 0).list(signal())
  const on22 = filterActivity(rows, { ...all, from: '2026-11-22', to: '2026-11-22' })
  assert.ok(on22.some(row => row.id === 'evt-004')) // Nov 21 18:15 UTC = Nov 22 01:15 WIB.
  assert.ok(!on22.some(row => row.id === 'evt-003')) // Nov 21 16:59 UTC = Nov 21 23:59 WIB.
  assert.equal(filterActivity(rows, { ...all, from: '2026-11-23', to: '2026-11-22' }).length, 0)
  assert.equal(filterActivity(rows, { ...all, search: 'missing-request' }).length, 0)
})

test('activity source clones detail fields and preserves redaction of SQL parameters', async () => {
  const source = createMockActivitySource('ready', 0)
  const rows = await source.list(signal())
  for (const row of rows) {
    assert.ok(!['password', 'token', 'secret', 'dsn', 'connection_url'].some(key => key in row.detail))
    if ('parameters' in row.detail) assert.equal(row.detail.parameters, '[redacted]')
  }
  rows[0].detail.method = 'changed'
  assert.equal((await source.list(signal()))[0].detail.method, 'POST')
})

test('activity adapter distinguishes empty, failed and cancelled requests', async () => {
  assert.deepEqual(await createMockActivitySource('empty', 0).list(signal()), [])
  await assert.rejects(createMockActivitySource('error', 0).list(signal()), /belum dapat dimuat/)
  const controller = new AbortController()
  const request = createMockActivitySource('ready', 100).list(controller.signal)
  controller.abort()
  await assert.rejects(request, { name: 'AbortError' })
})

test('observability links use the provisioned dashboard UID and support a Grafana subpath', () => {
  assert.deepEqual(observabilityLinks('http://localhost:3000/'), { dashboard: 'http://localhost:3000/d/pathgres-observability', explore: 'http://localhost:3000/explore' })
  assert.equal(observabilityLinks('https://monitor.example.com/grafana/').dashboard, 'https://monitor.example.com/grafana/d/pathgres-observability')
})

test('invalid or credential-bearing Grafana addresses never become external links', () => {
  for (const value of ['', '/grafana', 'javascript:alert(1)', 'ftp://example.com', 'https://admin:secret@example.com', 'https://example.com?token=secret', 'https://example.com/#token']) assert.equal(observabilityLinks(value), null)
})
