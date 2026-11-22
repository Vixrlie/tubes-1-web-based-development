import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { Alert, ProductBottomNavigation, ProductHeader, ProfilePopup } from '../../design-system'
import { useSession } from '../../features/auth/context'
import { paths } from '../routes'

export default function AppLayout() {
  const { state, session } = useSession()
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const active = pathname.startsWith('/admin') ? 'admin' : pathname.startsWith('/databases') ? 'database' : 'home'
  const destinations = { home: paths.home, database: paths.databases, admin: paths.admin }
  const showAdmin = state.user?.isPlatformAdmin ?? false
  return <div className="app-shell"><a className="app-skip-link" href="#main">Lewati ke konten</a><ProductHeader active={active} showAdmin={showAdmin} onNavigate={destination => navigate(destinations[destination])} profile={<ProfilePopup name={state.user?.username ?? ''} email={showAdmin ? 'Administrator platform' : 'Pengguna'} loading={state.busy} onLogout={() => { void session.logout() }} />} />
    <main id="main" className="app-content" tabIndex={-1}>{state.message && <Alert tone="error" title={state.message} />}<Outlet /></main>
    <ProductBottomNavigation active={active} showAdmin={showAdmin} onNavigate={destination => navigate(destinations[destination])} />
  </div>
}
