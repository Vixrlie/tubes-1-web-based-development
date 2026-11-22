import { ApiError } from '../../shared/api/api-error.ts'
import { createApiClient } from '../../shared/api/client.ts'

export type SessionUser = { id: string; username: string; isPlatformAdmin: boolean }
export type Session = { user: SessionUser; csrfToken: string }
export type SessionState = { status: 'checking' | 'authenticated' | 'anonymous' | 'error'; user: SessionUser | null; busy: boolean; message: string }

export function parseSession(value: unknown): Session {
  if (typeof value === 'object' && value !== null && 'user' in value && 'csrfToken' in value) {
    const { user, csrfToken } = value
    if (typeof user === 'object' && user !== null && 'id' in user && 'username' in user && 'isPlatformAdmin' in user && typeof user.id === 'string' && /^[1-9]\d*$/.test(user.id) && typeof user.username === 'string' && user.username.trim() && typeof user.isPlatformAdmin === 'boolean' && typeof csrfToken === 'string' && csrfToken.length > 0) {
      return { user: { id: user.id, username: user.username, isPlatformAdmin: user.isPlatformAdmin }, csrfToken }
    }
  }
  throw new ApiError('Respons sesi server tidak valid.', { kind: 'unexpected-response' })
}

// Never accept external destinations or navigate back into the login page.
export function loginDestination(value: unknown): string {
  if (typeof value !== 'string' || !/^\/(?:home|databases|admin)(?:[/?#]|$)/.test(value) || /[\\\s]/.test(value) || [...value].some(char => char.charCodeAt(0) < 32)) return '/home'
  return value
}

/** Cookie-only sessions. The request generation prevents stale responses restoring a logged-out user. */
export class SessionController {
  private state: SessionState = { status: 'checking', user: null, busy: false, message: '' }
  private csrfToken: string | null = null
  private generation = 0
  private refreshRequest: Promise<void> | null = null
  private listeners = new Set<() => void>()
  private clearCache: () => void
  private authApi: ReturnType<typeof createApiClient>
  readonly api: ReturnType<typeof createApiClient>

  constructor(clearCache: () => void, fetchImpl?: typeof fetch) {
    this.clearCache = clearCache
    this.authApi = createApiClient({ getCsrfToken: () => this.csrfToken, fetchImpl })
    this.api = createApiClient({ getCsrfToken: () => this.csrfToken, fetchImpl: async (...args) => {
      const generation = this.generation
      const response = await (fetchImpl ?? fetch)(...args)
      if (response.status === 401 && generation === this.generation) this.expire()
      return response
    } })
  }
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener) } }
  getSnapshot = () => this.state
  private publish(state: SessionState) { this.state = state; this.listeners.forEach(listener => listener()) }
  private anonymous(message = '') {
    this.csrfToken = null
    this.clearCache()
    this.publish({ status: 'anonymous', user: null, busy: false, message })
  }
  expire = () => { ++this.generation; this.anonymous('Sesi sudah berakhir. Silakan masuk kembali.') }
  private accept(session: Session) {
    if (this.state.user?.id !== session.user.id || this.state.user?.isPlatformAdmin !== session.user.isPlatformAdmin) this.clearCache()
    this.csrfToken = session.csrfToken
    this.publish({ status: 'authenticated', user: session.user, busy: false, message: '' })
  }
  refresh = (): Promise<void> => {
    if (this.state.busy) return Promise.resolve()
    if (this.refreshRequest) return this.refreshRequest
    const generation = this.generation
    this.refreshRequest = (async () => {
      try {
        const session = parseSession(await this.authApi.request('/auth/session'))
        if (generation === this.generation) this.accept(session)
      } catch (error) {
        if (generation !== this.generation) return
        if (error instanceof ApiError && error.status === 401) this.anonymous(this.state.user ? 'Sesi sudah berakhir. Silakan masuk kembali.' : '')
        else {
          this.csrfToken = null
          this.clearCache()
          this.publish({ status: 'error', user: null, busy: false, message: error instanceof Error ? error.message : 'Sesi belum dapat diperiksa.' })
        }
      }
    })().finally(() => { this.refreshRequest = null })
    return this.refreshRequest
  }
  login = async (username: string, password: string) => {
    if (this.state.busy) return
    const generation = ++this.generation
    this.publish({ ...this.state, busy: true, message: '' })
    try {
      const session = parseSession(await this.authApi.request('/auth/login', { method: 'POST', csrf: 'omit', json: { username, password } }))
      if (generation === this.generation) this.accept(session)
    } catch (error) {
      if (generation === this.generation) this.anonymous(error instanceof Error ? error.message : 'Login gagal diproses.')
      throw error
    }
  }
  logout = async () => {
    if (this.state.busy) return
    const generation = ++this.generation
    this.publish({ ...this.state, busy: true, message: '' })
    try {
      await this.authApi.request('/auth/logout', { method: 'POST' })
      if (generation === this.generation) this.anonymous()
    } catch (error) {
      if (generation !== this.generation) return
      const message = error instanceof Error ? error.message : 'Keluar gagal diproses. Coba lagi.'
      // These responses explicitly mean the browser cookie has been removed.
      if (error instanceof ApiError && (error.status === 401 || error.code === 'LOGOUT_FAILED')) this.anonymous(message)
      else this.publish({ ...this.state, busy: false, message })
    }
  }
}
