import { useEffect, useRef } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useSession } from '../auth/context'
import type { MigrationDraft } from './draft-source'
import type { WorkflowAction } from './workflow-model'
import { useDrafts } from './useDrafts'

export function useWorkflowAction(onSuccess?: (row: MigrationDraft) => void) {
  const query = useDrafts()
  const { session } = useSession()
  const client = useQueryClient()
  const controller = useRef<AbortController | null>(null)
  useEffect(() => () => controller.current?.abort(), [])
  return useMutation({ mutationFn: ({ draftId, action }: { draftId: string; action: WorkflowAction }) => {
    controller.current?.abort()
    controller.current = new AbortController()
    return query.source.transition(query.workspace.database.id, draftId, action, controller.current.signal)
  }, onSuccess: row => {
    if (session.getSnapshot().status !== 'authenticated' || session.getSnapshot().user?.id !== row.ownerId) return
    client.setQueryData<MigrationDraft[]>(query.queryKey, previous => [row, ...(previous ?? []).filter(item => item.id !== row.id)])
    void client.invalidateQueries({ queryKey: ['migration-drafts'] })
    void client.invalidateQueries({ queryKey: ['migration-history'] })
    onSuccess?.(row)
  } })
}
