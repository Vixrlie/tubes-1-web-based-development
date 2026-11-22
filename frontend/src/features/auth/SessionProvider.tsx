import { useEffect, useState, useSyncExternalStore } from 'react'
import type { ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { SessionController } from './session'
import { SessionContext } from './context'

export default function SessionProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [session] = useState(() => new SessionController(() => queryClient.clear()))
  const state = useSyncExternalStore(session.subscribe, session.getSnapshot)
  useEffect(() => {
    void session.refresh()
    const refresh = () => { if (document.visibilityState === 'visible') void session.refresh() }
    window.addEventListener('focus', refresh)
    document.addEventListener('visibilitychange', refresh)
    return () => { window.removeEventListener('focus', refresh); document.removeEventListener('visibilitychange', refresh) }
  }, [session])
  return <SessionContext.Provider value={{ state, session }}>{children}</SessionContext.Provider>
}
