import type { ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Alert, Badge, Button, Card, EmptyState, Spinner } from '../../design-system'
import { paths } from '../../app/routes'
import { withMigrationSearch } from './migration-source'
import { workflowStates } from './workflow-model'
import type { WorkflowStatus } from './workflow-model'

export function WorkflowBadge({ status }: { status: WorkflowStatus | 'canceled' }) {
  const value = status === 'canceled' ? { tone: 'neutral' as const, label: 'Dibatalkan' } : workflowStates[status]
  return <Badge tone={value.tone}>{value.label}</Badge>
}
export function WorkflowHeading({ title, database, connectionId, children }: { title: string; database: string; connectionId: string; children?: ReactNode }) {
  const [params] = useSearchParams()
  return <header className="workflow-heading"><div className="schema-heading"><h1>{title}</h1><p>{database} › <Link to={withMigrationSearch(paths.migrations(connectionId), params)}>Migrasi</Link> › {title}</p></div>{children}</header>
}
export function WorkflowState({ pending, error, missing, retry, children }: { pending: boolean; error?: Error | null; missing?: boolean; retry: () => void; children: ReactNode }) {
  if (pending) return <Card className="schema-state"><Spinner label="Memuat migrasi" /></Card>
  if (error) return <Card className="schema-state"><Alert tone="error" title="Data migrasi belum dapat dimuat">{error.message}</Alert><Button variant="secondary" onClick={retry}>Coba lagi</Button></Card>
  if (missing) return <Card><EmptyState title="Migrasi tidak ditemukan">Pilih migrasi yang tersedia pada database ini.</EmptyState></Card>
  return children
}
export function WorkflowDate({ value }: { value: string }) { return <time dateTime={value}>{new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))}</time> }
