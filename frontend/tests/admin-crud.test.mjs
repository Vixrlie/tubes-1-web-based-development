import test from 'node:test'
import assert from 'node:assert/strict'
import { createApiClient } from '../src/shared/api/client.ts'
import { paths } from '../src/app/routes.ts'
import {
  createAdminApi,
  parseAdminConnection,
  parseAdminGroupDetail,
  parseAdminUser,
  parseConnectionTestResult,
  parseDashboardStats,
} from '../src/features/admin/admin-api.ts'

test('admin DTO parsers project allowed fields and reject malformed payloads', () => {
  const stats = parseDashboardStats({ totalUsers: 5, activeUsers: 4, totalGroups: 2, totalConnections: 3 })
  assert.deepEqual(stats, { totalUsers: 5, activeUsers: 4, totalGroups: 2, totalConnections: 3 })
  assert.throws(() => parseDashboardStats(null), /tidak valid/)

  const user = parseAdminUser({
    id: '10',
    username: 'alice',
    fullName: 'Alice',
    email: 'alice@example.com',
    isPlatformAdmin: false,
    isActive: true,
    groups: ['backend'],
    createdAt: '2026-11-23T00:00:00Z',
    passwordHash: '$2a$12$never-copy',
  })
  assert.equal('passwordHash' in user, false)
  assert.deepEqual(user.groups, ['backend'])
  assert.throws(() => parseAdminUser({ id: 10, username: 'alice' }), /tidak valid/)

  const conn = parseAdminConnection({
    id: '2',
    name: 'target-demo',
    dbms: 'postgres',
    host: '127.0.0.1',
    port: '5433',
    database: 'sample',
    username: 'target',
    password: 'never-copy',
    connectionUrl: 'postgres://target:secret@127.0.0.1:5433/sample',
    groups: [{ name: 'backend', memberCount: 2, level: 'edit' }],
    createdAt: '2026-11-23T00:00:00Z',
  })
  assert.equal('password' in conn, false)
  assert.equal('connectionUrl' in conn, false)
  assert.equal(conn.groups.length, 1)

  const groupDetail = parseAdminGroupDetail({
    id: '1',
    name: 'backend',
    createdAt: '2026-11-23T00:00:00Z',
    members: [{ id: '2', username: 'alice', fullName: 'Alice', email: '', isPlatformAdmin: false, isActive: true }],
    candidateUsers: [],
    grants: [{ connectionId: '10', connectionName: 'target-demo', level: 'edit' }],
    availableConnections: [{ id: '11', name: 'analytics' }],
  })
  assert.equal(groupDetail.members.length, 1)
  assert.equal(groupDetail.grants[0].level, 'edit')

  assert.deepEqual(parseConnectionTestResult({ ok: true, latencyMs: 12, message: 'OK' }), {
    ok: true,
    latencyMs: 12,
    message: 'OK',
  })
  assert.throws(() => parseConnectionTestResult({ ok: 'yes' }), /tidak valid/)
})

test('admin API sends CSRF token on mutations and handles 204/errors cleanly', async () => {
  const seen = []
  const client = createApiClient({
    getCsrfToken: () => 'csrf-secret',
    fetchImpl: async (url, options) => {
      seen.push({ url, method: options.method, csrf: options.headers.get('X-CSRF-Token'), body: options.body })
      if (url === '/api/admin/users' && options.method === 'POST') {
        return Response.json(
          { id: '9', username: 'budi', fullName: 'Budi', email: '', isPlatformAdmin: false, isActive: true, groups: [], createdAt: '2026-11-23T00:00:00Z' },
          { status: 201 },
        )
      }
      if (url === '/api/admin/users/9' && options.method === 'DELETE') {
        return new Response(null, { status: 204 })
      }
      if (url === '/api/admin/groups/1/access' && options.method === 'PUT') {
        return Response.json({ updated: true })
      }
      if (url === '/api/admin/connections/10/test' && options.method === 'POST') {
        return Response.json({ ok: false, latencyMs: 15, message: 'connection refused' })
      }
      return Response.json({ error: { code: 'SELF_DELETE_FORBIDDEN', message: 'Tidak dapat menghapus akun sendiri.' } }, { status: 409 })
    },
  })

  const api = createAdminApi(client)

  const created = await api.createUser({
    username: 'budi',
    fullName: 'Budi',
    email: '',
    password: 'password123',
    isPlatformAdmin: false,
  })
  assert.equal(created.id, '9')

  await api.deleteUser('9')
  await api.updateGroupAccess('1', [{ connectionId: '10', level: 'edit' }])
  const testRes = await api.testSavedConnection('10')
  assert.equal(testRes.ok, false)

  await assert.rejects(
    api.deleteUser('1'),
    err => err.status === 409 && err.code === 'SELF_DELETE_FORBIDDEN',
  )

  for (const req of seen) {
    assert.equal(req.csrf, 'csrf-secret')
  }
})

test('admin CRUD route builders encode path segments safely', () => {
  assert.equal(paths.userDetail('42'), '/admin/users/42')
  assert.equal(paths.groupDetail('team/1'), '/admin/groups/team%2F1')
  assert.equal(paths.connectionDetail('10'), '/admin/connections/10')
  assert.equal(paths.editConnection('10'), '/admin/connections/10/edit')
})
