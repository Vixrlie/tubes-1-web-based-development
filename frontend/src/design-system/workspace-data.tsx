import type { ReactNode } from 'react'
import { ChevronRightIcon } from './control-icons'
import { Badge, Button, Card, Input, Spinner } from './primitives'
import { Dropdown } from './dropdown'
import type { DropdownOption } from './dropdown'
import { BatchProgress } from './data'

export function TableCard({ name, columnCount, primaryKey, permission, onOpen, disabled = false }: {
  name: string; columnCount: number; primaryKey?: string; permission: string; onOpen: () => void; disabled?: boolean
}) {
  return <button type="button" className="ds-table-card" onClick={onOpen} disabled={disabled}>
    <span className="ds-stack"><strong>{name}</strong><span className="ds-muted">{columnCount} kolom &middot; PK {primaryKey || '-'} &middot; {permission}</span></span>
    <ChevronRightIcon />
  </button>
}

export function ListToolbar({ search, onSearch, searchLabel = 'Cari', searchPlaceholder = 'Cari berdasarkan nama', resultCount, filter, action }: {
  search: string; onSearch: (value: string) => void; searchLabel?: string; searchPlaceholder?: string; resultCount: number
  filter?: { label: string; value: string; options: DropdownOption[]; onChange: (value: string) => void }; action?: ReactNode
}) {
  return <div className="ds-list-toolbar"><div className="ds-toolbar-controls">
    <Input type="search" label={searchLabel} placeholder={searchPlaceholder} value={search} onChange={event => onSearch(event.target.value)} />
    {filter && <Dropdown label={filter.label} options={filter.options} value={filter.value} onChange={filter.onChange} />}
    {action && <div className="ds-toolbar-action">{action}</div>}
  </div><small role="status">{resultCount} hasil ditemukan</small></div>
}

export function MetadataPanel({ title, items, action }: { title: string; items: { label: string; value: ReactNode }[]; action?: ReactNode }) {
  return <Card className="ds-metadata-panel"><div className="ds-row ds-between"><h3>{title}</h3>{action}</div><dl>{items.map(item => <div className="ds-metadata" key={item.label}><dt className="ds-muted">{item.label}</dt><dd>{item.value}</dd></div>)}</dl></Card>
}

export type MigrationStatus = 'queued' | 'running' | 'success' | 'failed' | 'canceled'
const migrationStates = {
  queued: { label: 'Antrean', tone: 'warning' }, running: { label: 'Berjalan', tone: 'info' },
  success: { label: 'Selesai', tone: 'success' }, failed: { label: 'Gagal', tone: 'error' }, canceled: { label: 'Dibatalkan', tone: 'neutral' },
} as const

export function MigrationJobCard({ jobId, name, database, status, completedBatches, totalBatches, detail, onOpen }: {
  jobId: string; name: string; database: string; status: MigrationStatus; completedBatches?: number; totalBatches?: number; detail?: string; onOpen: () => void
}) {
  const state = migrationStates[status]
  return <Card className="ds-stack ds-job-card"><div className="ds-row ds-between"><span className="ds-eyebrow">{jobId}</span><Badge tone={state.tone}>{status === 'running' && <Spinner label="Migrasi berjalan" />}{state.label}</Badge></div>
    <div><h3>{name}</h3><p className="ds-muted">{database}</p></div>
    {completedBatches !== undefined && totalBatches !== undefined && <BatchProgress value={completedBatches} total={totalBatches} detail={detail} />}
    {(completedBatches === undefined || totalBatches === undefined) && detail && <p className="ds-muted">{detail}</p>}
    <Button variant="secondary" onClick={onOpen}>Lihat detail</Button>
  </Card>
}

export type Approver = { id: string; name: string; status: 'pending' | 'approved' | 'rejected' }
export function ApprovalGroupCard({ name, required, approvers, onOpen }: { name: string; required: number; approvers: Approver[]; onOpen?: () => void }) {
  const threshold = Math.max(1, required)
  const approved = approvers.filter(person => person.status === 'approved').length
  const rejected = approvers.some(person => person.status === 'rejected')
  const complete = approved >= threshold
  return <Card className="ds-stack"><div className="ds-row ds-between"><h3>{name}</h3><Badge tone={complete ? 'success' : rejected ? 'error' : 'warning'}>{complete ? 'Disetujui' : rejected ? 'Ada penolakan' : 'Menunggu'}</Badge></div>
    <p className="ds-muted">{approved} dari {threshold} persetujuan diterima</p>
    <ul className="ds-approver-list">{approvers.map(person => <li key={person.id}><span>{person.name}</span><Badge tone={person.status === 'approved' ? 'success' : person.status === 'rejected' ? 'error' : 'neutral'}>{person.status === 'approved' ? 'Disetujui' : person.status === 'rejected' ? 'Ditolak' : 'Menunggu'}</Badge></li>)}</ul>
    {approvers.length === 0 && <p className="ds-muted">Belum ada approver</p>}
    {onOpen && <Button variant="secondary" onClick={onOpen}>Lihat anggota</Button>}
  </Card>
}
