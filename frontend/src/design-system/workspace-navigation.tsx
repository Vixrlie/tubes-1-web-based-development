import { useId, useState } from 'react'
import type { ReactNode } from 'react'
import { ChevronDownIcon, HomeIcon } from './control-icons'
import { Header, Icon, Sidebar, SidebarItem } from './navigation'
import { EmptyState } from './primitives'

export type Workspace = 'database' | 'admin'
export type ProductDestination = 'home' | Workspace

export function ProductHeader({ active, onNavigate, profile, onMenu, showAdmin = true }: {
  active: ProductDestination; onNavigate: (destination: Workspace) => void
  profile?: ReactNode; onMenu?: () => void; showAdmin?: boolean
}) {
  return <Header homeTo="/home" onMenu={onMenu} profile={profile} navigation={<>
    <button type="button" className="ds-header-link" aria-current={active === 'database' ? 'page' : undefined} onClick={() => onNavigate('database')}>Database</button>
    {showAdmin && <button type="button" className="ds-header-link" aria-current={active === 'admin' ? 'page' : undefined} onClick={() => onNavigate('admin')}>Admin</button>}
  </>} />
}

export type SchemaNavigation = { id: string; name: string; tables: { id: string; name: string; disabled?: boolean }[] }
export type DatabaseSelection = { kind: 'table'; schemaId: string; tableId: string } | { kind: 'migration' }

function SchemaBranch({ schema, selection, onSelect, activeSchema, onSelectSchema, variant }: { schema: SchemaNavigation; selection?: DatabaseSelection; onSelect: (selection: DatabaseSelection) => void; activeSchema?: string; onSelectSchema?: (id: string) => void; variant: 'default' | 'schema' }) {
  const [expanded, setExpanded] = useState(true)
  const id = useId()
  return <div className="ds-schema-branch">
    <SidebarItem active={activeSchema === schema.id} icon={<Icon name="database" />} aria-expanded={expanded} aria-controls={id} onClick={() => { setExpanded(!expanded); onSelectSchema?.(schema.id) }}>
      <span className="ds-nav-label">{schema.name}</span><span className={`ds-schema-chevron ${expanded ? 'is-expanded' : ''}`}><ChevronDownIcon /></span>
    </SidebarItem>
    <div id={id} className="ds-schema-tables" hidden={!expanded}>
      {schema.tables.length ? schema.tables.map(table => <SidebarItem key={table.id} nested disabled={table.disabled} icon={variant === 'schema' ? <img className={`ds-icon${selection?.kind === 'table' && selection.schemaId === schema.id && selection.tableId === table.id ? ' ds-icon--selected-asset' : ''}`} src={selection?.kind === 'table' && selection.schemaId === schema.id && selection.tableId === table.id ? '/design-system/schema-table-active.svg' : '/design-system/schema-table.svg'} alt="" /> : <Icon name="table" />}
        active={selection?.kind === 'table' && selection.schemaId === schema.id && selection.tableId === table.id}
        onClick={() => onSelect({ kind: 'table', schemaId: schema.id, tableId: table.id })}>{table.name}</SidebarItem>) : <p className="ds-sidebar-empty">Belum ada tabel</p>}
    </div>
  </div>
}

export function DatabaseSidebar({ database, schemas, selection, onSelect, loading = false, activeSchema, onSelectSchema, variant = 'default' }: {
  database: string; schemas: SchemaNavigation[]; selection?: DatabaseSelection
  onSelect: (selection: DatabaseSelection) => void; loading?: boolean
  activeSchema?: string; onSelectSchema?: (id: string) => void; variant?: 'default' | 'schema'
}) {
  return <Sidebar label={database}>
    {loading ? <p className="ds-sidebar-empty" role="status">Memuat schema...</p> : schemas.length
      ? schemas.map(schema => <SchemaBranch key={schema.id} schema={schema} selection={selection} onSelect={onSelect} activeSchema={activeSchema} onSelectSchema={onSelectSchema} variant={variant} />)
      : <EmptyState title="Belum ada schema" />}
    <SidebarItem icon={variant === 'schema' ? <img className="ds-icon" src="/design-system/schema-migration.svg" alt="" /> : <Icon name="migration" />} active={selection?.kind === 'migration'} onClick={() => onSelect({ kind: 'migration' })}>Migrasi</SidebarItem>
  </Sidebar>
}

export type AdminDestination = 'users' | 'groups' | 'connections'
const adminItems = [
  { value: 'users', label: 'Pengguna', icon: 'user' },
  { value: 'groups', label: 'Grup', icon: 'group' },
  { value: 'connections', label: 'Koneksi Database', icon: 'link' },
] as const

export function AdminSidebar({ active, onNavigate }: { active?: AdminDestination; onNavigate: (destination: AdminDestination) => void }) {
  return <Sidebar label="Admin workspace">{adminItems.map(item => <SidebarItem key={item.value} icon={<Icon name={item.icon} />} active={active === item.value} onClick={() => onNavigate(item.value)}>{item.label}</SidebarItem>)}</Sidebar>
}

/** Inline previews the mobile navigation without fixing it to the viewport. */
export function ProductBottomNavigation({ active, onNavigate, inline = false, showAdmin = true }: {
  active: ProductDestination; onNavigate: (destination: ProductDestination) => void; inline?: boolean; showAdmin?: boolean
}) {
  const items = [{ value: 'home', label: 'Beranda', icon: <HomeIcon /> }, { value: 'database', label: 'Database', icon: <Icon name="database" /> },
    ...(showAdmin ? [{ value: 'admin', label: 'Admin', icon: <Icon name="user" /> }] : [])] as { value: ProductDestination; label: string; icon: ReactNode }[]
  return <nav aria-label="Navigasi utama mobile" className={`ds-product-bottom-nav ${inline ? 'ds-product-bottom-nav--inline' : ''}`}>
    {items.map(item => <button key={item.value} type="button" aria-current={active === item.value ? 'page' : undefined} onClick={() => onNavigate(item.value)}>{item.icon}<span>{item.label}</span></button>)}
  </nav>
}
