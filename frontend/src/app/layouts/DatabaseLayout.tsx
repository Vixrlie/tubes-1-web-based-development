import { NavLink, Outlet, useParams } from 'react-router-dom'
import { paths } from '../routes'

export default function DatabaseLayout() {
  const { connectionId = '' } = useParams()
  return <div><nav className="app-section-navigation" aria-label="Workspace database">
    <NavLink to={paths.database(connectionId)} end>Ringkasan</NavLink>
    <NavLink to={paths.schema(connectionId)}>Schema</NavLink>
    <NavLink to={paths.migrations(connectionId)}>Migrasi</NavLink>
  </nav><Outlet /></div>
}
