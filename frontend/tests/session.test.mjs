import test from 'node:test'
import assert from 'node:assert/strict'
import { SessionController, parseSession, loginDestination } from '../src/features/auth/session.ts'

const sessionData = (admin = false, id = '7') => ({ user: { id, username: 'arina', isPlatformAdmin: admin }, csrfToken: 'csrf-secret' })
const unauthorized = () => Response.json({ error: { code: 'UNAUTHENTICATED', message: 'Sesi berakhir' } }, { status: 401 })
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r }); return { promise, resolve } }

test('session DTO rejects malformed identities and permissions', () => {
  assert.deepEqual(parseSession(sessionData()), sessionData())
  for (const invalid of [null, { user: null }, { ...sessionData(), csrfToken: '' }, { ...sessionData(), user: { ...sessionData().user, id: 7 } }, { ...sessionData(), user: { ...sessionData().user, isPlatformAdmin: 'true' } }]) assert.throws(() => parseSession(invalid))
})

test('login destinations accept only product routes', () => {
  for (const invalid of ['//evil.example', 'https://evil.example', '/login', '/home\\evil', '/design-system', null, '/home\n']) assert.equal(loginDestination(invalid), '/home')
  assert.equal(loginDestination('/databases/2/schema?tab=data'), '/databases/2/schema?tab=data')
})

test('401 restores anonymous state; infrastructure errors remain retryable errors', async () => {
  let status = 401
  const controller = new SessionController(() => {}, async () => status === 401 ? unauthorized() : Response.json({ error: { message: 'Database unavailable' } }, { status }))
  await controller.refresh()
  assert.equal(controller.getSnapshot().status, 'anonymous')
  status = 503
  await controller.refresh()
  assert.equal(controller.getSnapshot().status, 'error')
  assert.equal(controller.getSnapshot().user, null)
})

test('login, CSRF logout, and private cache cleanup use the JSON contract', async () => {
  let cleared = 0
  const controller = new SessionController(() => cleared++, async (url, init) => {
    if (url === '/api/auth/login') {
      assert.deepEqual(JSON.parse(init.body), { username: 'arina', password: 'example' })
      assert.equal(init.headers.has('X-CSRF-Token'), false)
      return Response.json(sessionData(true))
    }
    assert.equal(url, '/api/auth/logout')
    assert.equal(init.headers.get('X-CSRF-Token'), 'csrf-secret')
    return new Response(null, { status: 204 })
  })
  await controller.login('arina', 'example')
  assert.equal(controller.getSnapshot().user.isPlatformAdmin, true)
  await controller.logout()
  assert.equal(controller.getSnapshot().status, 'anonymous')
  assert.equal(cleared, 2)
  assert.equal(JSON.stringify(controller.getSnapshot()).includes('csrf-secret'), false)
})

test('failed credentials never authenticate; passwords never enter session state', async () => {
  const controller = new SessionController(() => {}, async () => unauthorized())
  await assert.rejects(controller.login('arina', 'secret-password'))
  assert.equal(controller.getSnapshot().status, 'anonymous')
  assert.equal(controller.getSnapshot().busy, false)
  assert.equal(JSON.stringify(controller.getSnapshot()).includes('secret-password'), false)
})

test('late session refresh cannot restore a user after logout', async () => {
  const pending = deferred()
  const controller = new SessionController(() => {}, async url => url.endsWith('/session') ? pending.promise : url.endsWith('/logout') ? new Response(null, { status: 204 }) : Response.json(sessionData()))
  await controller.login('arina', 'example')
  const refresh = controller.refresh()
  await controller.logout()
  pending.resolve(Response.json(sessionData()))
  await refresh
  assert.equal(controller.getSnapshot().status, 'anonymous')
})

test('business 401 expires the session; stale requests cannot clear a new account', async () => {
  let delayed = false
  const pending = deferred()
  const controller = new SessionController(() => {}, async url => url.includes('/auth/login') ? Response.json(sessionData()) : delayed ? pending.promise : unauthorized())
  await controller.login('arina', 'example')
  await assert.rejects(controller.api.request('/databases'))
  assert.equal(controller.getSnapshot().status, 'anonymous')
  await controller.login('arina', 'example')
  delayed = true
  const oldRequest = controller.api.request('/databases')
  await controller.login('another', 'example')
  pending.resolve(unauthorized())
  await assert.rejects(oldRequest)
  assert.equal(controller.getSnapshot().status, 'authenticated')
})

test('logout network failure preserves session for retry; explicit cookie removal clears it', async () => {
  let networkFailure = true
  const controller = new SessionController(() => {}, async url => {
    if (url.endsWith('/login')) return Response.json(sessionData())
    if (networkFailure) throw new TypeError('offline')
    return Response.json({ error: { code: 'LOGOUT_FAILED', message: 'Cookie dihapus' } }, { status: 500 })
  })
  await controller.login('arina', 'example')
  await controller.logout()
  assert.equal(controller.getSnapshot().status, 'authenticated')
  assert.equal(controller.getSnapshot().busy, false)
  networkFailure = false
  await controller.logout()
  assert.equal(controller.getSnapshot().status, 'anonymous')
  assert.equal(controller.getSnapshot().message, 'Cookie dihapus')
})
