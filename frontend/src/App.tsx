import { AppProviders } from './app/providers'
import { ErrorBoundary } from './app/ErrorBoundary'
import { AppRouter } from './app/router'
import './app/app.css'

export default function App() {
  return <ErrorBoundary><AppProviders><AppRouter /></AppProviders></ErrorBoundary>
}
