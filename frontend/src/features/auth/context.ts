import { createContext, useContext } from 'react'
import type { SessionController, SessionState } from './session'

export const SessionContext = createContext<{ state: SessionState; session: SessionController } | null>(null)
export function useSession() {
  const value = useContext(SessionContext)
  if (!value) throw new Error('SessionProvider diperlukan.')
  return value
}
