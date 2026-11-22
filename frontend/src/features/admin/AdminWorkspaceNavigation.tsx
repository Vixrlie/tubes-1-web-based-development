import { useLocation, useNavigate } from 'react-router-dom'
import { AdminSidebar } from '../../design-system'
import { paths } from '../../app/routes'

/** Shared between the desktop sidebar and the mobile drawer. */
export function AdminWorkspaceNavigation({ onNavigate }: { onNavigate?: () => void }) {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const active = pathname.startsWith(paths.users) ? 'users' : pathname.startsWith(paths.groups) ? 'groups' : pathname.startsWith(paths.connections) ? 'connections' : pathname.startsWith(paths.activity) ? 'activity' : pathname.startsWith(paths.observability) ? 'observability' : undefined
  return <AdminSidebar active={active} onNavigate={destination => { onNavigate?.(); navigate(paths[destination]) }} />
}
