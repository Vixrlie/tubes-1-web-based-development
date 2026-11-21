import { QueryClient } from '@tanstack/react-query'
import { ApiError } from '../shared/api/api-error'

export function createQueryClient() {
  return new QueryClient({ defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      retry: (failureCount, error) => {
        if (error.name === 'AbortError') return false
        if (error instanceof ApiError && (error.kind === 'csrf' || error.kind === 'unexpected-response' || (error.kind === 'http' && error.status < 500))) return false
        return failureCount < 1
      },
    },
    mutations: { retry: false },
  } })
}
