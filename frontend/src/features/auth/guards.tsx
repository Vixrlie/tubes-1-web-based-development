import { Link, Navigate, Outlet, useLocation } from 'react-router-dom'
import { Alert, Button, Spinner } from '../../design-system'
import { useSession } from './context'
import { loginDestination } from './session'

export function SessionGate({ guest = false }: { guest?: boolean }) {
  const { state, session } = useSession()
  const location = useLocation()
  if (state.status === 'checking') return <div className="app-status-page"><Spinner label="Sesi sedang diperiksa" /></div>
  if (state.status === 'error') return <div className="app-status-page"><Alert tone="error" title="Sesi belum dapat diperiksa">{state.message}</Alert><Button onClick={() => { void session.refresh() }}>Coba lagi</Button></div>
  if (guest) return state.status === 'authenticated' ? <Navigate to={loginDestination(location.state?.from)} replace /> : <Outlet />
  if (state.status !== 'authenticated') return <Navigate to="/login" replace state={{ from: location.pathname + location.search + location.hash }} />
  return <Outlet />
}

export function AdminGate() {
  const { state } = useSession()
  return state.user?.isPlatformAdmin ? <Outlet /> : <div className="app-status-page"><h1>Akses terbatas</h1><p>Halaman ini hanya dapat diakses oleh administrator platform.</p><Link className="ds-button ds-button--primary ds-button--md" to="/home">Kembali ke Beranda</Link></div>
}
