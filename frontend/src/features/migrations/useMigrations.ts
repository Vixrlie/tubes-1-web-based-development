import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { databasePreview } from '../databases/mock-source'
import { useDatabaseWorkspace } from '../databases/useDatabaseWorkspace'
import { createMockMigrationSource } from './migration-source'
import { useDrafts } from './useDrafts'
import { workflowExecutions } from './workflow-model'
import { usesMockData } from '../../shared/api/data-mode'
import type { ApiDraftSource } from './api-workflow-source'

export function useMigrations(executionId?: string) {
  const drafts = useDrafts()
  const { workspace, preview } = useDatabaseWorkspace()
  const [params] = useSearchParams()
  const migrationPreview = import.meta.env.DEV ? databasePreview(params.get('migrationPreview')) : 'ready'
  const mock = usesMockData(params)
  const source = useMemo(() => createMockMigrationSource(migrationPreview, preview), [migrationPreview, preview])
  const connectionId = workspace.database.id
  const api = !mock ? drafts.source as ApiDraftSource : undefined
  // API: riwayat diambil dari execution backend; mock: gabungan job simulasi dan fixture.
  const query = useQuery({ queryKey: ['migration-history', mock ? 'mock' : 'api', connectionId, preview, migrationPreview], queryFn: ({ signal }) => api ? api.history(connectionId, signal) : source.list(connectionId, signal), enabled: migrationPreview !== 'loading', retry: false, refetchInterval: !mock && drafts.data?.some(row => row.status === 'queued' || row.status === 'running') ? 1000 : false })
  const data = query.data ? (api ? query.data : [...workflowExecutions(drafts.data ?? []), ...query.data]) : undefined
  return { ...query, data, workspace, isPending: query.isPending || drafts.isPending, isError: query.isError || drafts.isError, error: query.error ?? drafts.error, refetch: async () => { await Promise.all([query.refetch(), drafts.refetch()]) }, execution: data?.find(row => row.id === executionId) }
}
