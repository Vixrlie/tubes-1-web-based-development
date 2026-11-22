import { NavLink, useLocation, useNavigate } from 'react-router-dom'
import { AdminSidebar } from '../../design-system'
import { paths } from '../../app/routes'

/** Shared between the desktop sidebar and the mobile drawer. */
export function AdminWorkspaceNavigation({ onNavigate }: { onNavigate?: () => void }) {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const active = pathname.startsWith(paths.users) ? 'users' : pathname.startsWith(paths.groups) ? 'groups' : pathname.startsWith(paths.connections) ? 'connections' : undefined
  return <><NavLink className="app-admin-overview" to={paths.admin} end onClick={onNavigate}>Dashboard</NavLink><AdminSidebar active={active} onNavigate={destination => { onNavigate?.(); navigate(paths[destination]) }} /></>
}
