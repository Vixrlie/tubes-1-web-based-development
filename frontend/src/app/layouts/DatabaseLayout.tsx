import { useEffect, useRef } from 'react'
import { Link, Outlet, useLocation, useParams } from 'react-router-dom'
import { Alert, Button, Card, Spinner } from '../../design-system'
import { paths } from '../routes'
import { useDatabaseWorkspaceQuery } from '../../features/databases/useDatabaseWorkspace'
import { SchemaSourceError, withWorkspacePreview } from '../../features/databases/schema-source'
import { WorkspaceSidebar } from '../../features/databases/WorkspaceNavigation'
import { withMigrationSearch } from '../../features/migrations/migration-source'
import '../../features/databases/schema.css'

export default function DatabaseLayout() {
  const { connectionId = '', schemaName } = useParams()
  const { pathname, search } = useLocation()
  const contentRef = useRef<HTMLDivElement>(null)
  useEffect(() => { if (contentRef.current) contentRef.current.scrollTop = 0 }, [pathname, search])
  const query = useDatabaseWorkspaceQuery(connectionId)
  if (query.isPending) return <Card className="schema-state"><Spinner label="Memuat schema database" /><p>Memuat schema…</p></Card>
  if (query.isError || !query.data) {
    const error = query.error
    const retryable = !(error instanceof SchemaSourceError) || error.code === 'unavailable'
    return <Card className="schema-state"><Alert tone="error" title={error instanceof SchemaSourceError && error.code === 'not-found' ? 'Database tidak ditemukan' : error instanceof SchemaSourceError && error.code === 'denied' ? 'Akses ditolak' : 'Schema belum dapat dimuat'}>{error?.message}</Alert>{retryable && <Button variant="secondary" loading={query.isFetching} onClick={() => { void query.refetch() }}>Coba lagi</Button>}<Link to={paths.databases}>Kembali ke daftar database</Link></Card>
  }
  const explorer = pathname === paths.schema(connectionId) || pathname === paths.database(connectionId)
  const schemaBack = `${paths.schema(connectionId)}${schemaName ? `?schema=${encodeURIComponent(schemaName)}` : ''}`
  const backPath = pathname.startsWith(`${paths.migrations(connectionId)}/`) ? withMigrationSearch(paths.migrations(connectionId), new URLSearchParams(search)) : withWorkspacePreview(schemaBack, query.preview)
  return <div className="schema-workspace"><aside className="schema-sidebar"><WorkspaceSidebar workspace={query.data} preview={query.preview} /></aside><div ref={contentRef} className="schema-content" role="region" aria-label="Konten database" tabIndex={0}><Link className="schema-back" to={explorer ? paths.databases : backPath}><span aria-hidden="true">&larr;</span>Kembali</Link><Outlet context={{ workspace: query.data, preview: query.preview }} /></div></div>
}
