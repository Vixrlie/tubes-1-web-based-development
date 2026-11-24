import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { NavLink, useLocation, useMatch, useSearchParams } from 'react-router-dom'
import { Button, MobileContextMenu, Spinner } from '../../design-system'
import { ChevronRightIcon } from '../../design-system/control-icons'
import { createMockDatabaseSource } from '../../features/databases/mock-source'
import { useDatabaseWorkspaceQuery } from '../../features/databases/useDatabaseWorkspace'
import { withWorkspacePreview } from '../../features/databases/schema-source'
import { paths } from '../routes'

const databaseSource = createMockDatabaseSource()
type MenuProps = { onClose: () => void }

function ContextLink({ to, children, onClose, current }: { to: string; children: ReactNode; onClose: () => void; current?: boolean }) {
  return <NavLink to={to} onClick={onClose} aria-current={current === undefined ? undefined : current ? 'page' : false}>{children}<ChevronRightIcon /></NavLink>
}

function DatabaseChoices({ onClose }: MenuProps) {
  const query = useQuery({ queryKey: ['databases', 'mock', 'ready'], queryFn: ({ signal }) => databaseSource.list(signal), retry: false })
  return <MobileContextMenu open onClose={onClose} title="Pilih database" description="Buka koneksi yang tersedia">
    {query.isPending ? <Spinner label="Memuat database" /> : query.isError ? <div role="alert"><p>Daftar database belum dapat dimuat.</p><Button size="sm" variant="secondary" onClick={() => { void query.refetch() }}>Coba lagi</Button></div>
      : <nav className="ds-context-links" aria-label="Pilih database">{query.data.length ? query.data.map(database => <ContextLink key={database.id} to={paths.schema(database.id)} onClose={onClose}>{database.name}{database.status === 'disconnected' ? ' · gagal terhubung' : ''}</ContextLink>) : <p>Belum ada database.</p>}</nav>}
  </MobileContextMenu>
}

function DatabaseContext({ connectionId, onClose }: MenuProps & { connectionId: string }) {
  const query = useDatabaseWorkspaceQuery(connectionId)
  const { pathname } = useLocation()
  const [params] = useSearchParams()
  const tableMatch = useMatch('/databases/:connectionId/schemas/:schemaName/tables/:tableName')
  const workspace = query.data
  const schema = workspace?.schemas.find(item => item.name === (tableMatch?.params.schemaName || params.get('schema') || workspace.schemas[0]?.name))
  return <MobileContextMenu open onClose={onClose} title={workspace?.database.name ?? 'Database'} description={workspace ? schema ? `Schema ${schema.name} · ${schema.tables.length} tabel` : 'Belum ada schema' : 'Navigasi database'}>
    {query.isPending ? <Spinner label="Memuat navigasi database" /> : query.isError ? <div role="alert"><p>{query.error.message}</p><ContextLink to={paths.databases} onClose={onClose}>Pilih database lain</ContextLink></div> : workspace && <nav className="ds-context-links" aria-label={`Navigasi ${workspace.database.name}`}>
      {workspace.schemas.map(item => <div key={item.name} className="ds-context-group">
        <ContextLink to={withWorkspacePreview(`${paths.schema(connectionId)}?schema=${encodeURIComponent(item.name)}`, query.preview)} onClose={onClose} current={pathname.endsWith('/schema') && schema?.name === item.name}>Schema {item.name}</ContextLink>
        {item.tables.map(table => <ContextLink key={table.name} to={withWorkspacePreview(paths.table(connectionId, item.name, table.name), query.preview)} onClose={onClose}>{table.name}</ContextLink>)}
      </div>)}
      <ContextLink to={withWorkspacePreview(paths.migrations(connectionId), query.preview)} onClose={onClose}>Migrasi</ContextLink>
    </nav>}
  </MobileContextMenu>
}

export default function MobileWorkspaceMenu({ active, connectionId, onClose }: MenuProps & { active: 'home' | 'database' | 'admin'; connectionId?: string }) {
  if (active === 'admin') return <MobileContextMenu open onClose={onClose} title="Kelola platform" description="Administrasi · Aktivitas · Monitoring"><nav className="ds-context-links" aria-label="Navigasi Admin">
    <ContextLink to={paths.users} onClose={onClose}>Pengguna</ContextLink><ContextLink to={paths.groups} onClose={onClose}>Grup</ContextLink><ContextLink to={paths.connections} onClose={onClose}>Koneksi Database</ContextLink><ContextLink to={paths.activity} onClose={onClose}>Activity Logs</ContextLink><ContextLink to={paths.observability} onClose={onClose}>Observability</ContextLink>
  </nav></MobileContextMenu>
  return connectionId ? <DatabaseContext connectionId={connectionId} onClose={onClose} /> : <DatabaseChoices onClose={onClose} />
}
