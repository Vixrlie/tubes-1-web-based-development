import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { useSession } from '../auth/context'
import { useDatabaseWorkspace } from '../databases/useDatabaseWorkspace'
import { browserDraftStorage, createMockDraftSource } from './draft-source'
import { createApiDraftSource } from './api-workflow-source'
import { usesMockData } from '../../shared/api/data-mode'

export function useDrafts() {
  const { state, session } = useSession()
  const { workspace, preview } = useDatabaseWorkspace()
  const ownerId = state.user?.id ?? ''
  const author = state.user?.username ?? ''
  const [params] = useSearchParams()
  const outcome = import.meta.env.DEV && ['failed', 'error'].includes(params.get('jobOutcome') ?? '') ? 'failed' : 'success'
  const failAfter = import.meta.env.DEV && params.get('jobOutcome') === 'error' ? 0 : undefined
  const mock = usesMockData(params)
  const source = useMemo(() => !mock ? createApiDraftSource({ api: session.api, ownerId }) : createMockDraftSource({ ownerId, author, storage: browserDraftStorage, preview, outcome, failAfter, isActive: () => session.getSnapshot().status === 'authenticated' && session.getSnapshot().user?.id === ownerId }), [mock, ownerId, author, preview, session, outcome, failAfter])
  const queryKey = ['migration-drafts', mock ? 'mock' : 'api', ownerId, workspace.database.id, preview]
  const query = useQuery({ queryKey, queryFn: ({ signal }) => source.list(workspace.database.id, signal), enabled: !!ownerId, retry: false, refetchOnMount: 'always', refetchInterval: query => query.state.data?.some(row => row.status === 'queued' || row.status === 'running') ? 1000 : false })
  return { ...query, source, workspace, queryKey }
}
