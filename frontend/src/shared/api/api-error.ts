export type ApiErrorKind = 'http' | 'network' | 'unexpected-response' | 'csrf'

export class ApiError extends Error {
  readonly status: number
  readonly kind: ApiErrorKind
  readonly code?: string
  readonly fields?: Record<string, string>

  constructor(message: string, options: { status?: number; kind: ApiErrorKind; code?: string; fields?: Record<string, string> }) {
    super(message)
    this.name = 'ApiError'
    this.status = options.status ?? 0
    this.kind = options.kind
    this.code = options.code
    this.fields = options.fields
  }
}
