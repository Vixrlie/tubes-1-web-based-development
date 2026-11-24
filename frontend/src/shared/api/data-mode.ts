// Sumber data SPA: JSON API backend secara default. Fixture mock tetap tersedia
// untuk pratinjau state UI saat development (?preview=error dan sejenisnya)
// atau bila VITE_DATA_SOURCE=mock.
const previewKeys = ['preview', 'migrationPreview', 'tablePreview', 'toolsPreview', 'jobOutcome']

export function usesMockData(params: URLSearchParams, env: { DEV?: boolean; VITE_DATA_SOURCE?: string } = import.meta.env) {
  if (env.VITE_DATA_SOURCE === 'mock') return true
  return !!env.DEV && previewKeys.some(key => params.has(key))
}
