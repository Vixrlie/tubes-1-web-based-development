import { useEffect, useState } from 'react'
import { Outlet, useLocation, useMatch, useNavigate } from 'react-router-dom'
import { Alert, ProductBottomNavigation, ProductHeader, ProfilePopup } from '../../design-system'
import { useSession } from '../../features/auth/context'
import { paths } from '../routes'
import MobileWorkspaceMenu from './MobileWorkspaceMenu'

export default function AppLayout() {
  const { state, session } = useSession()
  const { pathname, key } = useLocation()
  const [drawerLocation, setDrawerLocation] = useState<string | null>(null)
  const isDatabaseList = pathname === paths.databases
  const workspaceMatch = useMatch('/databases/:connectionId/*')
  const isDatabaseWorkspace = !!workspaceMatch
  const navigate = useNavigate()
  const active = pathname.startsWith('/admin') ? 'admin' : pathname.startsWith('/databases') ? 'database' : 'home'
  useEffect(() => { window.scrollTo(0, 0) }, [key])
  useEffect(() => {
    const desktop = window.matchMedia('(min-width: 1024px)')
    const closeOnDesktop = () => { if (desktop.matches) setDrawerLocation(null) }
    desktop.addEventListener('change', closeOnDesktop)
    return () => desktop.removeEventListener('change', closeOnDesktop)
  }, [])
  const destinations = { home: paths.home, database: paths.databases, admin: paths.admin }
  const showAdmin = state.user?.isPlatformAdmin ?? false
  return <div className={`app-shell${isDatabaseList ? ' app-shell--database-list' : isDatabaseWorkspace ? ' app-shell--database-workspace' : active === 'admin' ? ' app-shell--admin' : ''}`}><a className="app-skip-link" href="#main">Lewati ke konten</a><ProductHeader active={active} showAdmin={showAdmin} menuOpen={drawerLocation === key} onMenu={() => setDrawerLocation(drawerLocation === key ? null : key)} onNavigate={destination => navigate(destinations[destination])} profile={<ProfilePopup name={state.user?.fullName || state.user?.username || ''} email={state.user?.email} role={showAdmin ? 'Admin' : 'User'} loading={state.busy} onLogout={() => { void session.logout() }} />} />
    {drawerLocation === key && <MobileWorkspaceMenu active={active} connectionId={workspaceMatch?.params.connectionId} onClose={() => setDrawerLocation(null)} />}
    <main id="main" className="app-content" tabIndex={-1}>{state.message && <Alert tone="error" title={state.message} />}<Outlet /></main>
    <ProductBottomNavigation active={active} showAdmin={showAdmin} onNavigate={destination => navigate(destinations[destination])} />
  </div>
}
