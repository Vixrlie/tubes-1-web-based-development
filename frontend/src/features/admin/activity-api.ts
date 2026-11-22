import { ApiError } from '../../shared/api/api-error.ts'
import type { createApiClient } from '../../shared/api/client.ts'
import type { ActivityEvent, ActivityFilters } from './monitoring-source.ts'

export type ActivityPage = { items: ActivityEvent[]; total: number; page: number; pageSize: number; limited: boolean; from: string; to: string }
function object(value: unknown): value is Record<string, unknown> { return !!value && typeof value === 'object' && !Array.isArray(value) }
function invalid(): never { throw new ApiError('Respons activity logs dari server tidak valid.', { kind: 'unexpected-response' }) }
export function parseActivityEvent(value: unknown): ActivityEvent {
  if (!object(value) || typeof value.id !== 'string' || !/^[1-9]\d{0,18}-[a-f0-9]{16}$/.test(value.id) || typeof value.time !== 'string' || !Number.isFinite(Date.parse(value.time)) || typeof value.action !== 'string' || !value.action || typeof value.status !== 'string' || !['success', 'warning', 'failed', 'started'].includes(value.status) || typeof value.userId !== 'string' || typeof value.requestId !== 'string' || !object(value.detail)) invalid()
  const row: ActivityEvent = { id: value.id, time: value.time, action: value.action, status: value.status as ActivityEvent['status'], userId: value.userId, requestId: value.requestId, detail: {} }
  for (const key of ['database', 'migrationId', 'traceId', 'spanId'] as const) {
    if (value[key] !== undefined && typeof value[key] !== 'string') invalid()
    if (typeof value[key] === 'string') row[key] = value[key]
  }
  const detailFields = ['method', 'route', 'http_status', 'resource_id', 'target_user_id', 'query_fingerprint', 'parameter_count', 'duration_us', 'rows', 'command', 'sqlstate', 'batch_position', 'reported_queries', 'duration', 'parameters', 'query', 'error']
  for (const [key, detail] of Object.entries(value.detail)) {
    if (typeof detail !== 'string') invalid()
    if (detailFields.includes(key)) row.detail[key] = detail
  }
  return row
}
export function parseActivityPage(value: unknown): ActivityPage {
  if (!object(value) || !Array.isArray(value.items) || !Number.isSafeInteger(value.total) || Number(value.total) < 0 || !Number.isSafeInteger(value.page) || Number(value.page) < 1 || !Number.isSafeInteger(value.pageSize) || Number(value.pageSize) < 1 || Number(value.pageSize) > 100 || typeof value.limited !== 'boolean' || typeof value.from !== 'string' || !Number.isFinite(Date.parse(value.from)) || typeof value.to !== 'string' || !Number.isFinite(Date.parse(value.to))) invalid()
  const { total, page, pageSize } = value as { total: number; page: number; pageSize: number }
  if (value.items.length > pageSize || value.items.length !== Math.min(pageSize, Math.max(0, total - (page - 1) * pageSize)) || page > Math.max(1, Math.ceil(total / pageSize))) invalid()
  return { items: value.items.map(parseActivityEvent), total, page, pageSize, limited: value.limited, from: value.from, to: value.to }
}
export function createActivityApi(api: ReturnType<typeof createApiClient>) {
  return {
    async list(filters: ActivityFilters, page: number, signal: AbortSignal) {
      return parseActivityPage(await api.request('/admin/activity-logs', { query: { ...filters, page, pageSize: 8 }, signal }))
    },
    async get(id: string, signal: AbortSignal) {
      if (!/^[1-9]\d{0,18}-[a-f0-9]{16}$/.test(id)) throw new ApiError('Aktivitas tidak ditemukan.', { kind: 'http', status: 404 })
      return parseActivityEvent(await api.request(`/admin/activity-logs/${encodeURIComponent(id)}`, { signal }))
    },
  }
}
