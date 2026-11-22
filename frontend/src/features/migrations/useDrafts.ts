import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSession } from '../auth/context'
import { useDatabaseWorkspace } from '../databases/useDatabaseWorkspace'
import { browserDraftStorage, createMockDraftSource } from './draft-source'

export function useDrafts() {
  const { state, session } = useSession()
  const { workspace, preview } = useDatabaseWorkspace()
  const ownerId = state.user?.id ?? ''
  const author = state.user?.username ?? ''
  const source = useMemo(() => createMockDraftSource({ ownerId, author, storage: browserDraftStorage, preview, isActive: () => session.getSnapshot().status === 'authenticated' && session.getSnapshot().user?.id === ownerId }), [ownerId, author, preview, session])
  const queryKey = ['migration-drafts', 'mock', ownerId, workspace.database.id, preview]
  const query = useQuery({ queryKey, queryFn: ({ signal }) => source.list(workspace.database.id, signal), enabled: !!ownerId, retry: false })
  return { ...query, source, workspace, queryKey }
}
