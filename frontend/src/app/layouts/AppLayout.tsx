import { useState } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { Alert, Modal, ProductBottomNavigation, ProductHeader, ProfilePopup } from '../../design-system'
import { useSession } from '../../features/auth/context'
import { paths } from '../routes'

export default function AppLayout() {
  const { state, session } = useSession()
  const { pathname, key } = useLocation()
  const [drawerLocation, setDrawerLocation] = useState<string | null>(null)
  const isDatabaseList = pathname === paths.databases
  const navigate = useNavigate()
  const active = pathname.startsWith('/admin') ? 'admin' : pathname.startsWith('/databases') ? 'database' : 'home'
  const destinations = { home: paths.home, database: paths.databases, admin: paths.admin }
  const showAdmin = state.user?.isPlatformAdmin ?? false
  return <div className={`app-shell${isDatabaseList ? ' app-shell--database-list' : ''}`}><a className="app-skip-link" href="#main">Lewati ke konten</a><ProductHeader active={active} showAdmin={showAdmin} onMenu={isDatabaseList ? () => setDrawerLocation(key) : undefined} onNavigate={destination => navigate(destinations[destination])} profile={<ProfilePopup name={state.user?.username ?? ''} email={showAdmin ? 'Administrator platform' : 'Pengguna'} loading={state.busy} onLogout={() => { void session.logout() }} />} />
    <Modal open={isDatabaseList && drawerLocation === key} onClose={() => setDrawerLocation(null)} title="Navigasi" drawer><nav className="app-mobile-drawer-links" aria-label="Navigasi mobile">{(['home', 'database', ...(showAdmin ? ['admin'] : [])] as const).map(destination => { const item = destination as keyof typeof destinations; return <NavLink key={item} to={destinations[item]} onClick={() => setDrawerLocation(null)}>{item === 'home' ? 'Beranda' : item === 'database' ? 'Database' : 'Admin'}</NavLink> })}</nav></Modal>
    <main id="main" className="app-content" tabIndex={-1}>{state.message && <Alert tone="error" title={state.message} />}<Outlet /></main>
    <ProductBottomNavigation active={active} showAdmin={showAdmin} onNavigate={destination => navigate(destinations[destination])} />
  </div>
}
