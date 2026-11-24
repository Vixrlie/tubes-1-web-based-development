import { ApiError } from './api-error.ts'

type QueryValue = string | number | boolean | null | undefined
type RequestOptions = {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  query?: Record<string, QueryValue>
  json?: unknown
  signal?: AbortSignal
  /** Only for explicitly CSRF-exempt endpoints agreed with BE, e.g. login. */
  csrf?: 'required' | 'omit'
}

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** JSON transport only. No endpoint is called automatically or assumed ready. */
export function createApiClient({ getCsrfToken = () => null, fetchImpl = (...args) => fetch(...args) }: {
  getCsrfToken?: () => string | null
  fetchImpl?: typeof fetch
} = {}) {
  return {
    async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
      if (!path.startsWith('/') || path.startsWith('//') || /[\\?#]/.test(path)) throw new Error('Gunakan path API relatif, misalnya /databases, dan query terpisah.')
      const segments = path.split('/').map(segment => decodeURIComponent(segment))
      if (segments.some(segment => segment === '.' || segment === '..' || segment.includes('/') || segment.includes('\\'))) throw new Error('Path API tidak valid.')
      const method = options.method ?? 'GET'
      if (method === 'GET' && options.json !== undefined) throw new Error('GET tidak menerima JSON body.')
      const headers = new Headers({ Accept: 'application/json' })
      if (method !== 'GET' && options.csrf !== 'omit') {
        const token = getCsrfToken()
        if (!token) throw new ApiError('Token CSRF belum tersedia.', { kind: 'csrf' })
        headers.set('X-CSRF-Token', token)
      }
      if (options.json !== undefined) headers.set('Content-Type', 'application/json')
      const query = new URLSearchParams()
      for (const [key, value] of Object.entries(options.query ?? {})) if (value !== undefined && value !== null) query.set(key, String(value))
      const suffix = query.size ? `?${query}` : ''
      let response: Response
      try {
        response = await fetchImpl(`/api${path}${suffix}`, {
          method, headers, body: options.json === undefined ? undefined : JSON.stringify(options.json),
          credentials: 'include', redirect: 'error', cache: 'no-store', signal: options.signal,
        })
      } catch (error) {
        if (options.signal?.aborted || (error instanceof Error && error.name === 'AbortError')) throw error
        throw new ApiError('Tidak dapat menghubungi server.', { kind: 'network' })
      }
      if (response.ok && (response.status === 204 || response.status === 205)) return undefined as T
      const contentType = response.headers.get('content-type')?.split(';')[0].trim() ?? ''
      const isJson = contentType === 'application/json' || /^application\/[\w.+-]+\+json$/.test(contentType)
      if (!isJson) throw new ApiError('Server belum memberikan respons JSON yang sesuai.', { status: response.status, kind: 'unexpected-response' })
      let body: unknown
      try { body = await response.json() } catch {
        throw new ApiError('Respons JSON server tidak valid.', { status: response.status, kind: 'unexpected-response' })
      }
      if (!response.ok) {
        const detail = record(body) && record(body.error) ? body.error : body
        // Workflow API (dbm) memakai { error: "pesan", code }; admin API memakai { error: { code, message } }.
        const message = record(detail) && typeof detail.message === 'string' ? detail.message : record(body) && typeof body.error === 'string' ? body.error : 'Permintaan gagal diproses.'
        const code = record(detail) && typeof detail.code === 'string' ? detail.code : undefined
        const fields = record(detail) && record(detail.fields) && Object.values(detail.fields).every(value => typeof value === 'string') ? detail.fields as Record<string, string> : undefined
        throw new ApiError(message, { status: response.status, kind: 'http', code, fields })
      }
      return body as T
    },
  }
}
