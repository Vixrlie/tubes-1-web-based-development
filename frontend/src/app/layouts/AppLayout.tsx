import { useEffect, useState } from 'react'
import { NavLink, Outlet, useLocation, useMatch, useNavigate } from 'react-router-dom'
import { Alert, Modal, ProductBottomNavigation, ProductHeader, ProfilePopup } from '../../design-system'
import { useSession } from '../../features/auth/context'
import { paths } from '../routes'
import { DatabaseDrawerNavigation } from '../../features/databases/WorkspaceNavigation'
import { AdminWorkspaceNavigation } from '../../features/admin/AdminWorkspaceNavigation'

export default function AppLayout() {
  const { state, session } = useSession()
  const { pathname, key } = useLocation()
  const [drawerLocation, setDrawerLocation] = useState<string | null>(null)
  const isDatabaseList = pathname === paths.databases
  const workspaceMatch = useMatch('/databases/:connectionId/*')
  const isDatabaseWorkspace = !!workspaceMatch
  const navigate = useNavigate()
  const active = pathname.startsWith('/admin') ? 'admin' : pathname.startsWith('/databases') ? 'database' : 'home'
  useEffect(() => { if (active === 'database') window.scrollTo(0, 0) }, [key, active])
  const destinations = { home: paths.home, database: paths.databases, admin: paths.admin }
  const showAdmin = state.user?.isPlatformAdmin ?? false
  return <div className={`app-shell${isDatabaseList ? ' app-shell--database-list' : isDatabaseWorkspace ? ' app-shell--database-workspace' : active === 'admin' ? ' app-shell--admin' : ''}`}><a className="app-skip-link" href="#main">Lewati ke konten</a><ProductHeader active={active} showAdmin={showAdmin} onMenu={() => setDrawerLocation(key)} onNavigate={destination => navigate(destinations[destination])} profile={<ProfilePopup name={state.user?.username ?? ''} email={showAdmin ? 'Administrator platform' : 'Pengguna'} loading={state.busy} onLogout={() => { void session.logout() }} />} />
    <Modal open={drawerLocation === key} onClose={() => setDrawerLocation(null)} title={<NavLink className="app-drawer-brand" to={paths.home} aria-label="Pathgres beranda" onClick={() => setDrawerLocation(null)}><img src="/design-system/c0d63.png" width="191" height="44" alt="Pathgres" /></NavLink>} drawer><p className="app-drawer-label">WORKSPACE</p><nav className="app-mobile-drawer-links" aria-label="Navigasi mobile">{(['home', 'database', ...(showAdmin ? ['admin'] : [])] as const).map(destination => { const item = destination as keyof typeof destinations; return <NavLink key={item} to={destinations[item]} onClick={() => setDrawerLocation(null)}>{item === 'home' ? 'Beranda' : item === 'database' ? 'Database' : 'Admin'}</NavLink> })}</nav>{active === 'admin' && showAdmin && <div className="app-drawer-admin"><AdminWorkspaceNavigation onNavigate={() => setDrawerLocation(null)} /></div>}{workspaceMatch?.params.connectionId && <div className="schema-drawer-tree"><DatabaseDrawerNavigation connectionId={workspaceMatch.params.connectionId} onNavigate={() => setDrawerLocation(null)} /></div>}</Modal>
    <main id="main" className="app-content" tabIndex={-1}>{state.message && <Alert tone="error" title={state.message} />}<Outlet /></main>
    <ProductBottomNavigation active={active} showAdmin={showAdmin} onNavigate={destination => navigate(destinations[destination])} />
  </div>
}
