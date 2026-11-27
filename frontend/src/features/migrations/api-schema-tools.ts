import type { createApiClient } from '../../shared/api/client.ts'
import { schemaError, toTableDescription } from '../databases/api-source.ts'
import type { ApiTable } from '../databases/api-source.ts'
import { compareSnapshots } from './schema-tools-source.ts'
import type { SchemaSnapshot } from './schema-tools-source.ts'

type Api = ReturnType<typeof createApiClient>
type ApiExplainItem = {
  n: number; statement: string; kind: string; note?: string; plan?: string; analyze: boolean
  root: { node: string; relation: string; startup_cost: number; total_cost: number; rows: number; width: number } | null
  planning_ms: number | null; execution_ms: number | null
}

export type ExplainPlan = {
  sql: string; analyze: boolean; planningMs: number; executionMs?: number; text: string
  scan: { node: string; relation: string; startupCost: number; totalCost: number; rows: number; width: number }
}

/** Respons EXPLAIN backend → hasil yang ditampilkan halaman SQL preview. */
export function explainPlan(sql: string, analyze: boolean, items: ApiExplainItem[]): ExplainPlan {
  const planned = items.find(item => item.root)
  if (!planned?.root) throw new Error(items.find(item => item.note)?.note ?? 'Statement ini tidak memiliki execution plan.')
  const root = planned.root
  return {
    sql, analyze, planningMs: planned.planning_ms ?? 0, ...(planned.execution_ms !== null ? { executionMs: planned.execution_ms } : {}),
    text: items.map(item => item.plan || `#${item.n} ${item.statement}\n${item.note ?? ''}`).join('\n').trim(),
    scan: { node: root.node, relation: root.relation || '-', startupCost: root.startup_cost, totalCost: root.total_cost, rows: root.rows, width: root.width },
  }
}

export function createApiSchemaTools(api: Api) {
  async function snapshot(connectionId: string, signal: AbortSignal): Promise<SchemaSnapshot> {
    try {
      const body = await api.request<{ id: number; name: string; tables: ApiTable[] }>(`/databases/${encodeURIComponent(connectionId)}/snapshot`, { signal })
      return { id: String(body.id), name: body.name, tables: body.tables.map(table => ({ schema: table.schema, name: table.name, description: toTableDescription(table) })) }
    } catch (error) { throw schemaError(error) }
  }
  return {
    async compare(targetId: string, referenceId: string, signal: AbortSignal) {
      if (targetId === referenceId) throw new Error('Pilih dua database yang berbeda.')
      const [target, reference] = await Promise.all([snapshot(targetId, signal), snapshot(referenceId, signal)])
      return compareSnapshots(target, reference)
    },
    async explain(connectionId: string, query: string, analyze: boolean, signal: AbortSignal) {
      if (!query.trim() || query.length > 50000) throw new Error('Isi SQL untuk EXPLAIN, maksimal 50.000 karakter.')
      const body = await api.request<{ items: ApiExplainItem[] }>(`/databases/${encodeURIComponent(connectionId)}/explain`, { method: 'POST', json: { sql: query, analyze }, signal })
      return explainPlan(query, analyze, body.items)
    },
  }
}
