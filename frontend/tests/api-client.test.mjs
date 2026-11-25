import test from 'node:test'
import assert from 'node:assert/strict'
import { createApiClient } from '../src/shared/api/client.ts'
import { ApiError } from '../src/shared/api/api-error.ts'
import { paths } from '../src/app/routes.ts'

test('JSON transport sends cookies, Accept, query, and disables redirects', async () => {
  let called = false
  const api = createApiClient({ fetchImpl: async (url, init) => {
    called = true
    assert.equal(url, '/api/databases?page=2&search=shop+db')
    assert.equal(init.credentials, 'include')
    assert.equal(init.redirect, 'error')
    assert.equal(init.headers.get('Accept'), 'application/json')
    return Response.json({ items: [] })
  } })
  assert.equal(called, false, 'creating the client must not contact an unimplemented endpoint')
  assert.deepEqual(await api.request('/databases', { query: { page: 2, search: 'shop db', unused: null } }), { items: [] })
})

test('mutations fail before sending when CSRF is missing', async () => {
  const api = createApiClient({ fetchImpl: async () => assert.fail('must not send mutation') })
  await assert.rejects(api.request('/databases/1/migrations', { method: 'POST', json: { sql: 'SELECT 1' } }), error => error instanceof ApiError && error.kind === 'csrf')
})

test('mutations send current CSRF token and JSON payload', async () => {
  const api = createApiClient({ getCsrfToken: () => 'csrf-example', fetchImpl: async (_url, init) => {
    assert.equal(init.headers.get('X-CSRF-Token'), 'csrf-example')
    assert.equal(init.headers.get('Content-Type'), 'application/json')
    assert.deepEqual(JSON.parse(init.body), { sql: 'SELECT 1' })
    return Response.json({ id: '1' }, { status: 201 })
  } })
  assert.deepEqual(await api.request('/databases/1/migrations', { method: 'POST', json: { sql: 'SELECT 1' } }), { id: '1' })
})

test('CSRF exemption must be explicit for a future login contract', async () => {
  const api = createApiClient({ fetchImpl: async (_url, init) => {
    assert.equal(init.headers.has('X-CSRF-Token'), false)
    return Response.json({ user: null })
  } })
  await api.request('/auth/login', { method: 'POST', csrf: 'omit', json: { username: 'example', password: 'example' } })
})

test('HTML pages must never be mistaken for successful JSON responses', async () => {
  const api = createApiClient({ fetchImpl: async () => new Response('<html>Login</html>', { headers: { 'Content-Type': 'text/html' } }) })
  await assert.rejects(api.request('/auth/session'), error => error instanceof ApiError && error.kind === 'unexpected-response')
})

test('HTTP status, error code, and validation fields survive normalization', async () => {
  const api = createApiClient({ fetchImpl: async () => Response.json({ error: { code: 'VALIDATION_FAILED', message: 'Nama wajib diisi', fields: { name: 'Wajib diisi' } } }, { status: 422 }) })
  await assert.rejects(api.request('/databases'), error => error instanceof ApiError && error.status === 422 && error.code === 'VALIDATION_FAILED' && error.fields.name === 'Wajib diisi')
})

test('401 and 403 remain distinguishable for future session/permission handling', async () => {
  for (const status of [401, 403]) {
    const api = createApiClient({ fetchImpl: async () => Response.json({ error: { message: 'Akses ditolak' } }, { status }) })
    await assert.rejects(api.request('/databases'), error => error instanceof ApiError && error.status === status && error.kind === 'http')
  }
})

test('204 is accepted without attempting to parse JSON', async () => {
  const api = createApiClient({ fetchImpl: async () => new Response(null, { status: 204 }) })
  assert.equal(await api.request('/auth/session'), undefined)
})

test('malformed JSON is reported as a contract error', async () => {
  const api = createApiClient({ fetchImpl: async () => new Response('{', { headers: { 'Content-Type': 'application/json' } }) })
  await assert.rejects(api.request('/databases'), error => error instanceof ApiError && error.kind === 'unexpected-response')
})

test('network failures become a typed error', async () => {
  const api = createApiClient({ fetchImpl: async () => { throw new TypeError('Failed to fetch') } })
  await assert.rejects(api.request('/databases'), error => error instanceof ApiError && error.kind === 'network')
})

test('abort preserves cancellation instead of reporting a connection failure', async () => {
  const controller = new AbortController()
  controller.abort()
  const abort = new DOMException('Aborted', 'AbortError')
  const api = createApiClient({ fetchImpl: async (_url, init) => { assert.equal(init.signal, controller.signal); throw abort } })
  await assert.rejects(api.request('/databases', { signal: controller.signal }), error => error === abort)
})

test('API paths cannot escape /api or target another origin', async () => {
  const api = createApiClient({ fetchImpl: async () => assert.fail('invalid path must not be fetched') })
  for (const path of ['https://example.com', '//example.com', '/../login', '/%2e%2e/login', '/databases?unparsed=true', '/databases#hash']) await assert.rejects(api.request(path))
})

test('browser route builders keep identifier segments encoded', () => {
  assert.equal(paths.database('123'), '/databases/123')
  assert.equal(paths.table('123', 'public', 'sales/orders'), '/databases/123/schemas/public/tables/sales%2Forders')
})
