import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { ProductBottomNavigation, ProductHeader } from '../../design-system'
import { paths } from '../routes'

export default function AppLayout() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const active = pathname.startsWith('/admin') ? 'admin' : pathname.startsWith('/databases') ? 'database' : 'home'
  const destinations = { home: paths.home, database: paths.databases, admin: paths.admin }
  return <div className="app-shell"><ProductHeader active={active} onNavigate={destination => navigate(destinations[destination])} />
    <main id="main" className="app-content"><Outlet /></main>
    <ProductBottomNavigation active={active} onNavigate={destination => navigate(destinations[destination])} />
  </div>
}
