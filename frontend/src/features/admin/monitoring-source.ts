import { waitForMock } from '../databases/mock-source.ts'

export type ActivityStatus = 'success' | 'warning' | 'failed' | 'started'
export type ActivityEvent = {
  id: string; time: string; action: string; status: ActivityStatus; userId: string
  requestId: string; traceId?: string; spanId?: string; database?: string
  migrationId?: string; detail: Record<string, string>
}
export type ActivityFilters = { search: string; status: string; category: string; from: string; to: string }
export const activityStatus = { success: 'Sukses', warning: 'Peringatan', failed: 'Gagal', started: 'Dimulai' } as const
// Fixtures match the redacted shape in internal/activity, not a fabricated JSON API.
const events: readonly ActivityEvent[] = [
  { id: 'evt-014', time: '2026-11-22T12:40:00Z', action: 'admin.connection.create', status: 'success', userId: '42', requestId: 'req-admin-014', detail: { method: 'POST', route: '/admin/connections', http_status: '303' } },
  { id: 'evt-013', time: '2026-11-22T12:38:00Z', action: 'user.login', status: 'failed', userId: 'budi', requestId: 'req-login-013', detail: { method: 'POST', path: '/api/auth/login', error: 'operation failed' } },
  { id: 'evt-012', time: '2026-11-22T12:35:24Z', action: 'sql.result', status: 'failed', userId: '42', requestId: 'req-migration-014', traceId: 'e7a2f60e36c74d48af994cfd805d0014', spanId: '990a25e6cfc10014', database: 'target', migrationId: '14', detail: { query: 'ALTER', query_fingerprint: '72812bf1c2c31f637407d68b5e1b56aa50c3047a1dc81a94931e3f416b6a84f3', parameter_count: '0', parameters: '[redacted]', duration_us: '124', sqlstate: '42501', error: 'operation failed' } },
  { id: 'evt-011', time: '2026-11-22T12:35:23Z', action: 'sql.start', status: 'started', userId: '42', requestId: 'req-migration-014', traceId: 'e7a2f60e36c74d48af994cfd805d0014', spanId: '990a25e6cfc10014', database: 'target', migrationId: '14', detail: { query: 'ALTER', parameters: '[redacted]', parameter_count: '0' } },
  { id: 'evt-010', time: '2026-11-22T12:30:15Z', action: 'sql.commit', status: 'success', userId: '42', requestId: 'req-migration-013', database: 'target', migrationId: '13', detail: { query: 'COMMIT', duration_us: '520', parameters: '[redacted]' } },
  { id: 'evt-009', time: '2026-11-22T12:30:14Z', action: 'sql.result', status: 'success', userId: '42', requestId: 'req-migration-013', database: 'target', migrationId: '13', detail: { query: 'UPDATE', rows: '250', duration_us: '8120', parameter_count: '1', parameters: '[redacted]' } },
  { id: 'evt-008', time: '2026-11-22T12:24:00Z', action: 'http.request', status: 'success', userId: '', requestId: 'req-schema-008', detail: { method: 'GET', route: '/databases/{id}/schema', duration: '18ms' } },
  { id: 'evt-007', time: '2026-11-22T12:18:00Z', action: 'sql.rollback', status: 'success', userId: '42', requestId: 'req-migration-012', database: 'target', migrationId: '12', detail: { query: 'ROLLBACK', parameters: '[redacted]', duration_us: '350' } },
  { id: 'evt-006', time: '2026-11-22T11:45:00Z', action: 'sql.batch.result', status: 'failed', userId: '42', requestId: 'req-batch-006', database: 'target', migrationId: '11', detail: { reported_queries: '42', duration_us: '2800000', error: 'operation failed' } },
  { id: 'evt-005', time: '2026-11-22T10:30:00Z', action: 'http.request', status: 'failed', userId: '', requestId: 'req-access-005', detail: { method: 'GET', route: '/admin/users', duration: '2ms' } },
  { id: 'evt-004', time: '2026-11-21T18:15:00Z', action: 'sql.result', status: 'warning', userId: 'unattributed', requestId: 'req-sql-004', database: 'platform', detail: { query: 'SELECT', parameters: '[redacted]', duration_us: '1200000' } },
  { id: 'evt-003', time: '2026-11-21T16:59:00Z', action: 'sql.result', status: 'success', userId: '42', requestId: 'req-sql-003', database: 'platform', detail: { query: 'SELECT', rows: '4', parameters: '[redacted]', duration_us: '850' } },
  { id: 'evt-002', time: '2026-11-21T09:30:00Z', action: 'http.request', status: 'success', userId: '', requestId: 'req-login-002', detail: { method: 'POST', route: '/api/auth/login', duration: '38ms' } },
  { id: 'evt-001', time: '2026-11-20T05:00:00Z', action: 'http.request', status: 'success', userId: '', requestId: 'req-session-001', detail: { method: 'GET', route: '/api/auth/session', duration: '4ms' } },
]

export function createMockActivitySource(preview = 'ready', delay = 250) {
  return { async list(signal: AbortSignal): Promise<ActivityEvent[]> {
    await waitForMock(signal, delay)
    if (preview === 'error') throw new Error('Activity logs belum dapat dimuat. Silakan coba lagi.')
    return preview === 'empty' ? [] : events.map(event => ({ ...event, detail: { ...event.detail } }))
  } }
}
const jakartaDay = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta', year: 'numeric', month: '2-digit', day: '2-digit' })
export function filterActivity(events: ActivityEvent[], filters: ActivityFilters) {
  const needle = filters.search.trim().toLowerCase()
  return events.filter(event => {
    const day = jakartaDay.format(new Date(event.time))
    return (!filters.from || day >= filters.from) && (!filters.to || day <= filters.to)
      && (filters.status === 'all' || event.status === filters.status)
      && (filters.category === 'all' || event.action.startsWith(`${filters.category}.`))
      && (!needle || [event.action, event.userId, event.requestId, event.traceId, event.database, event.migrationId, ...Object.values(event.detail)].some(value => value?.toLowerCase().includes(needle)))
  }).sort((a, b) => b.time.localeCompare(a.time) || b.id.localeCompare(a.id))
}

export function observabilityLinks(value: string) {
  try {
    const url = new URL(value)
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) return null
    const base = url.href.replace(/\/+$/, '')
    return { dashboard: `${base}/d/pathgres-observability`, explore: `${base}/explore` }
  } catch { return null }
}
