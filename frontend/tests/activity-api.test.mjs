import test from 'node:test'
import assert from 'node:assert/strict'
import { createApiClient } from '../src/shared/api/client.ts'
import { createActivityApi, parseActivityEvent, parseActivityPage } from '../src/features/admin/activity-api.ts'

const entry = { id: '1795344000000000000-1234567890abcdef', time: '2026-11-22T12:00:00Z', action: 'sql.result', status: 'failed', userId: '42', requestId: 'req-real', database: 'target', migrationId: '14', detail: { query: 'ALTER', parameters: '[redacted]', sqlstate: '42501' } }
const page = { items: [entry], total: 1, page: 1, pageSize: 8, limited: false, from: '2026-11-15T12:00:00Z', to: '2026-11-22T12:00:00Z' }
const filters = { search: 'req-real', status: 'failed', category: 'sql', from: '2026-11-22', to: '2026-11-22' }

test('live activity list sends filters, paging and cookie transport to the admin JSON endpoint', async () => {
  const controller = new AbortController()
  const api = createActivityApi(createApiClient({ fetchImpl: async (url, options) => {
    const request = new URL(url, 'http://localhost')
    assert.equal(request.pathname, '/api/admin/activity-logs')
    for (const [key, value] of Object.entries(filters)) assert.equal(request.searchParams.get(key), value)
    assert.equal(request.searchParams.get('page'), '2')
    assert.equal(request.searchParams.get('pageSize'), '8')
    assert.equal(options.credentials, 'include')
    assert.equal(options.cache, 'no-store')
    assert.equal(options.signal, controller.signal)
    return Response.json(page) // Server may clamp an out-of-range page.
  } }))
  assert.equal((await api.list(filters, 2, controller.signal)).page, 1)
})

test('activity detail is loaded independently from the filtered list and uses a stable backend ID', async () => {
  let called = false
  const api = createActivityApi(createApiClient({ fetchImpl: async url => {
    called = true
    assert.equal(url, `/api/admin/activity-logs/${entry.id}`)
    return Response.json(entry)
  } }))
  assert.deepEqual(await api.get(entry.id, new AbortController().signal), entry)
  called = false
  await assert.rejects(api.get('evt-014', new AbortController().signal), error => error.status === 404)
  assert.equal(called, false)
})

test('activity DTO rejects malformed pages and projects only supported fields', () => {
  const clean = parseActivityEvent({ ...entry, password: 'never-copy', detail: { ...entry.detail, token: 'never-copy' } })
  assert.equal('password' in clean, false)
  assert.equal('token' in clean.detail, false)
  for (const bad of [{ ...page, total: 0 }, { ...page, page: 0 }, { ...page, pageSize: 500 }, { ...page, limited: 'false' }, { ...page, items: [{ ...entry, status: 'unknown' }] }, { ...page, items: [{ ...entry, time: 'bad' }] }]) assert.throws(() => parseActivityPage(bad), /tidak valid/)
  assert.equal(parseActivityPage({ ...page, items: [], total: 0 }).total, 0)
})

test('live activity failures remain failures and cannot fall back to fixtures', async () => {
  for (const status of [401, 403, 503]) {
    const api = createActivityApi(createApiClient({ fetchImpl: async () => Response.json({ error: { code: 'ACTIVITY_UNAVAILABLE', message: 'Layanan belum tersedia' } }, { status }) }))
    await assert.rejects(api.list(filters, 1, new AbortController().signal), error => error.status === status)
  }
  const api = createActivityApi(createApiClient({ fetchImpl: async () => { throw new DOMException('Cancelled', 'AbortError') } }))
  await assert.rejects(api.list(filters, 1, new AbortController().signal), { name: 'AbortError' })
})
