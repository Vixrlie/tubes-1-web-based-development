import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams, useOutletContext } from 'react-router-dom'
import { createMockSchemaSource, schemaPreview } from './schema-source'
import type { DatabaseWorkspace, SchemaPreview } from './schema-source'

export function useDatabaseWorkspaceQuery(connectionId: string) {
  const [params] = useSearchParams()
  const preview = import.meta.env.DEV ? schemaPreview(params.get('preview')) : 'ready'
  const source = useMemo(() => createMockSchemaSource(preview), [preview])
  const query = useQuery({ queryKey: ['database-workspace', 'mock', connectionId, preview], queryFn: ({ signal }) => source.load(connectionId, signal), enabled: preview !== 'loading', retry: false })
  return { ...query, preview }
}
export type DatabaseWorkspaceContext = { workspace: DatabaseWorkspace; preview: SchemaPreview }
export function useDatabaseWorkspace() { return useOutletContext<DatabaseWorkspaceContext>() }
