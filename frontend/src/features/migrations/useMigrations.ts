import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { databasePreview } from '../databases/mock-source'
import { useDatabaseWorkspace } from '../databases/useDatabaseWorkspace'
import { createMockMigrationSource } from './migration-source'

export function useMigrations(executionId?: string) {
  const { workspace, preview } = useDatabaseWorkspace()
  const [params] = useSearchParams()
  const migrationPreview = import.meta.env.DEV ? databasePreview(params.get('migrationPreview')) : 'ready'
  const source = useMemo(() => createMockMigrationSource(migrationPreview, preview), [migrationPreview, preview])
  const connectionId = workspace.database.id
  const query = useQuery({ queryKey: ['migration-history', 'mock', connectionId, preview, migrationPreview], queryFn: ({ signal }) => source.list(connectionId, signal), enabled: migrationPreview !== 'loading', retry: false })
  return { ...query, workspace, execution: query.data?.find(row => row.id === executionId) }
}
