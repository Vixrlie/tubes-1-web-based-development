import { ApiError } from '../../shared/api/api-error.ts'
import type { createApiClient } from '../../shared/api/client.ts'
import type {
  AdminConnection,
  AdminDashboardStats,
  AdminGroup,
  AdminGroupDetail,
  AdminUser,
  AdminUserDetail,
  ConnectionFormPayload,
  ConnectionTestResult,
  CreateUserPayload,
  UpdateUserPayload,
} from './admin-types.ts'

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function invalidResponse(label: string): never {
  throw new ApiError(`Respons ${label} dari server tidak valid.`, { kind: 'unexpected-response' })
}

function str(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

function bool(value: unknown): boolean {
  return typeof value === 'boolean' ? value : false
}

function num(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

export function parseDashboardStats(value: unknown): AdminDashboardStats {
  if (!isRecord(value)) invalidResponse('dashboard admin')
  return {
    totalUsers: num(value.totalUsers),
    activeUsers: num(value.activeUsers),
    totalGroups: num(value.totalGroups),
    totalConnections: num(value.totalConnections),
  }
}

export function parseAdminUser(value: unknown): AdminUser {
  if (!isRecord(value) || typeof value.id !== 'string' || typeof value.username !== 'string') {
    invalidResponse('pengguna')
  }
  const groups = Array.isArray(value.groups)
    ? value.groups.filter((g): g is string => typeof g === 'string')
    : []
  return {
    id: value.id,
    username: value.username,
    fullName: str(value.fullName),
    email: str(value.email),
    isPlatformAdmin: bool(value.isPlatformAdmin),
    isActive: bool(value.isActive),
    groups,
    createdAt: str(value.createdAt),
  }
}

export function parseAdminUserList(value: unknown): { items: AdminUser[]; total: number } {
  if (!isRecord(value) || !Array.isArray(value.items)) invalidResponse('daftar pengguna')
  const items = value.items.map(parseAdminUser)
  return { items, total: typeof value.total === 'number' ? value.total : items.length }
}

export function parseAdminUserDetail(value: unknown): AdminUserDetail {
  const base = parseAdminUser(value)
  if (!isRecord(value)) invalidResponse('detail pengguna')
  const effectiveAccess = Array.isArray(value.effectiveAccess)
    ? value.effectiveAccess
        .filter(isRecord)
        .map(item => ({
          connectionId: str(item.connectionId),
          connectionName: str(item.connectionName),
          groupId: str(item.groupId),
          groupName: str(item.groupName),
          level: str(item.level),
        }))
    : []
  return {
    ...base,
    isSelf: bool(value.isSelf),
    effectiveAccess,
  }
}

export function parseAdminGroupList(value: unknown): { items: AdminGroup[]; total: number } {
  if (!isRecord(value) || !Array.isArray(value.items)) invalidResponse('daftar grup')
  const items: AdminGroup[] = value.items.map(raw => {
    if (!isRecord(raw) || typeof raw.id !== 'string' || typeof raw.name !== 'string') {
      invalidResponse('grup')
    }
    const accesses = Array.isArray(raw.accesses)
      ? raw.accesses.filter(isRecord).map(a => ({
          connectionId: typeof a.connectionId === 'string' ? a.connectionId : undefined,
          connectionName: str(a.connectionName),
          level: str(a.level),
        }))
      : []
    return {
      id: raw.id,
      name: raw.name,
      memberCount: num(raw.memberCount),
      accesses,
      createdAt: str(raw.createdAt),
    }
  })
  return { items, total: typeof value.total === 'number' ? value.total : items.length }
}

export function parseAdminGroupDetail(value: unknown): AdminGroupDetail {
  if (!isRecord(value) || typeof value.id !== 'string' || typeof value.name !== 'string') {
    invalidResponse('detail grup')
  }
  const parseMember = (m: unknown) => {
    if (!isRecord(m) || typeof m.id !== 'string' || typeof m.username !== 'string') {
      invalidResponse('anggota grup')
    }
    return {
      id: m.id,
      username: m.username,
      fullName: str(m.fullName),
      email: str(m.email),
      isPlatformAdmin: bool(m.isPlatformAdmin),
      isActive: bool(m.isActive),
    }
  }
  return {
    id: value.id,
    name: value.name,
    createdAt: str(value.createdAt),
    members: Array.isArray(value.members) ? value.members.map(parseMember) : [],
    candidateUsers: Array.isArray(value.candidateUsers) ? value.candidateUsers.map(parseMember) : [],
    grants: Array.isArray(value.grants)
      ? value.grants.filter(isRecord).map(g => ({
          connectionId: str(g.connectionId),
          connectionName: str(g.connectionName),
          level: str(g.level),
        }))
      : [],
    availableConnections: Array.isArray(value.availableConnections)
      ? value.availableConnections.filter(isRecord).map(c => ({ id: str(c.id), name: str(c.name) }))
      : [],
  }
}

export function parseAdminConnection(value: unknown): AdminConnection {
  if (!isRecord(value) || typeof value.id !== 'string' || typeof value.name !== 'string') {
    invalidResponse('koneksi')
  }
  const groups = Array.isArray(value.groups)
    ? value.groups.filter(isRecord).map(g => ({
        name: str(g.name),
        memberCount: num(g.memberCount),
        level: str(g.level),
      }))
    : []
  return {
    id: value.id,
    name: value.name,
    description: str(value.description),
    dbms: str(value.dbms, 'postgres'),
    host: str(value.host),
    port: str(value.port, '5432'),
    database: str(value.database),
    username: str(value.username),
    groups,
    createdAt: str(value.createdAt),
  }
}

export function parseAdminConnectionList(value: unknown): { items: AdminConnection[]; total: number } {
  if (!isRecord(value) || !Array.isArray(value.items)) invalidResponse('daftar koneksi')
  const items = value.items.map(parseAdminConnection)
  return { items, total: typeof value.total === 'number' ? value.total : items.length }
}

export function parseConnectionTestResult(value: unknown): ConnectionTestResult {
  if (!isRecord(value) || typeof value.ok !== 'boolean') invalidResponse('hasil tes koneksi')
  return {
    ok: value.ok,
    latencyMs: num(value.latencyMs),
    message: str(value.message),
  }
}

export function createAdminApi(api: ReturnType<typeof createApiClient>) {
  return {
    async dashboard(signal?: AbortSignal) {
      return parseDashboardStats(await api.request('/admin/dashboard', { signal }))
    },
    async listUsers(signal?: AbortSignal) {
      return parseAdminUserList(await api.request('/admin/users', { signal }))
    },
    async getUser(id: string, signal?: AbortSignal) {
      return parseAdminUserDetail(await api.request(`/admin/users/${encodeURIComponent(id)}`, { signal }))
    },
    async createUser(payload: CreateUserPayload) {
      return parseAdminUser(await api.request('/admin/users', { method: 'POST', json: payload }))
    },
    async updateUser(id: string, payload: UpdateUserPayload) {
      return parseAdminUser(await api.request(`/admin/users/${encodeURIComponent(id)}`, { method: 'PATCH', json: payload }))
    },
    async activateUser(id: string) {
      return parseAdminUser(await api.request(`/admin/users/${encodeURIComponent(id)}/activate`, { method: 'POST' }))
    },
    async deactivateUser(id: string) {
      return parseAdminUser(await api.request(`/admin/users/${encodeURIComponent(id)}/deactivate`, { method: 'POST' }))
    },
    async resetUserPassword(id: string, password: string) {
      await api.request(`/admin/users/${encodeURIComponent(id)}/password`, { method: 'POST', json: { password } })
    },
    async deleteUser(id: string) {
      await api.request(`/admin/users/${encodeURIComponent(id)}`, { method: 'DELETE' })
    },
    async listGroups(signal?: AbortSignal) {
      return parseAdminGroupList(await api.request('/admin/groups', { signal }))
    },
    async getGroup(id: string, signal?: AbortSignal) {
      return parseAdminGroupDetail(await api.request(`/admin/groups/${encodeURIComponent(id)}`, { signal }))
    },
    async createGroup(name: string) {
      return await api.request<{ id: string; name: string }>('/admin/groups', { method: 'POST', json: { name } })
    },
    async renameGroup(id: string, name: string) {
      return await api.request<{ id: string; name: string }>(`/admin/groups/${encodeURIComponent(id)}`, { method: 'PATCH', json: { name } })
    },
    async deleteGroup(id: string) {
      await api.request(`/admin/groups/${encodeURIComponent(id)}`, { method: 'DELETE' })
    },
    async addGroupMembers(id: string, userIds: string[]) {
      await api.request(`/admin/groups/${encodeURIComponent(id)}/members`, { method: 'POST', json: { userIds } })
    },
    async removeGroupMember(id: string, userId: string) {
      await api.request(`/admin/groups/${encodeURIComponent(id)}/members/${encodeURIComponent(userId)}`, { method: 'DELETE' })
    },
    async updateGroupAccess(id: string, grants: { connectionId: string; level: string }[]) {
      await api.request(`/admin/groups/${encodeURIComponent(id)}/access`, { method: 'PUT', json: { grants } })
    },
    async listConnections(signal?: AbortSignal) {
      return parseAdminConnectionList(await api.request('/admin/connections', { signal }))
    },
    async getConnection(id: string, signal?: AbortSignal) {
      return parseAdminConnection(await api.request(`/admin/connections/${encodeURIComponent(id)}`, { signal }))
    },
    async createConnection(payload: ConnectionFormPayload) {
      return parseAdminConnection(await api.request('/admin/connections', { method: 'POST', json: payload }))
    },
    async updateConnection(id: string, payload: ConnectionFormPayload) {
      return parseAdminConnection(await api.request(`/admin/connections/${encodeURIComponent(id)}`, { method: 'PATCH', json: payload }))
    },
    async deleteConnection(id: string) {
      await api.request(`/admin/connections/${encodeURIComponent(id)}`, { method: 'DELETE' })
    },
    async testConnectionInput(payload: ConnectionFormPayload) {
      return parseConnectionTestResult(await api.request('/admin/connections/test', { method: 'POST', json: payload }))
    },
    async testSavedConnection(id: string) {
      return parseConnectionTestResult(await api.request(`/admin/connections/${encodeURIComponent(id)}/test`, { method: 'POST' }))
    },
  }
}
