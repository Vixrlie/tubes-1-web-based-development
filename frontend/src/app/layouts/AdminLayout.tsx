import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { AdminSidebar } from '../../design-system'
import { paths } from '../routes'

export default function AdminLayout() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const active = pathname.startsWith(paths.users) ? 'users' : pathname.startsWith(paths.groups) ? 'groups' : pathname.startsWith(paths.connections) ? 'connections' : undefined
  return <div className="app-workspace"><aside><NavLink className="app-admin-overview" to={paths.admin} end>Dashboard</NavLink>
    <AdminSidebar active={active} onNavigate={destination => navigate(paths[destination])} /></aside><div><Outlet /></div></div>
}
