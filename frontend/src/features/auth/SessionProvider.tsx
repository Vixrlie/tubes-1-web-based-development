import { useEffect, useState, useSyncExternalStore } from 'react'
import type { ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { SessionController } from './session'
import { SessionContext } from './context'
import { clearBrowserMockDrafts } from '../migrations/draft-source'

export default function SessionProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [session] = useState(() => {
    const controller = new SessionController(() => { queryClient.clear(); if (controller.getSnapshot().user) clearBrowserMockDrafts() })
    return controller
  })
  const state = useSyncExternalStore(session.subscribe, session.getSnapshot)
  useEffect(() => {
    if (state.status === 'authenticated') clearBrowserMockDrafts(state.user?.id)
    else if (state.status === 'anonymous' || state.status === 'error') clearBrowserMockDrafts()
  }, [state.status, state.user?.id])
  useEffect(() => {
    void session.refresh()
    const refresh = () => { if (document.visibilityState === 'visible') void session.refresh() }
    window.addEventListener('focus', refresh)
    document.addEventListener('visibilitychange', refresh)
    return () => { window.removeEventListener('focus', refresh); document.removeEventListener('visibilitychange', refresh) }
  }, [session])
  return <SessionContext.Provider value={{ state, session }}>{children}</SessionContext.Provider>
}
