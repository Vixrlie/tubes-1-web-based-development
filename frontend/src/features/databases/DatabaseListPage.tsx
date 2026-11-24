import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { Alert, Button, Card, DatabaseCard, EmptyState, Spinner } from '../../design-system'
import { paths } from '../../app/routes'
import { connectionErrorMessage, createMockDatabaseSource, databasePreview } from './mock-source'
import { databaseDescription } from './types'
import type { DatabaseConnection, DatabaseSource } from './types'
import './databases.css'

function ConnectionCard({ database, source }: { database: DatabaseConnection; source: DatabaseSource }) {
  const navigate = useNavigate()
  const [checking, setChecking] = useState(false)
  const [feedback, setFeedback] = useState('')
  const request = useRef<AbortController | null>(null)
  useEffect(() => () => request.current?.abort(), [])
  async function reconnect() {
    if (request.current) return
    const controller = new AbortController()
    request.current = controller
    setChecking(true)
    setFeedback('')
    try { await source.reconnect(database.id, controller.signal) }
    catch (error) { if (!controller.signal.aborted) setFeedback(error instanceof Error ? error.message : connectionErrorMessage) }
    finally { if (!controller.signal.aborted) { setChecking(false); request.current = null } }
  }
  return <div className="database-list-item"><DatabaseCard variant="workspace" name={database.name} description={databaseDescription(database)} connected={database.status === 'connected'} permission={database.permission === 'edit' ? 'View + Edit' : 'View Only'} readOnly={database.permission === 'view'} connectionError={database.status === 'disconnected' ? connectionErrorMessage : undefined} loading={checking} onOpen={() => navigate(paths.schema(database.id))} onReconnect={() => { void reconnect() }} />
    <span className="sr-only" role="status">{feedback && `${database.name}: ${feedback}`}</span></div>
}

export default function DatabaseListPage() {
  const [searchParams] = useSearchParams()
  // Development preview states do not add controls to the product UI.
  const preview = import.meta.env.DEV ? databasePreview(searchParams.get('preview')) : 'ready'
  const source = useMemo(() => createMockDatabaseSource(preview), [preview])
  const query = useQuery({ queryKey: ['databases', 'mock', preview], queryFn: ({ signal }) => source.list(signal), enabled: preview !== 'loading', retry: false })
  return <section className="database-list-page" aria-labelledby="database-list-title">
    <Link to={paths.home} className="database-back"><span aria-hidden="true">←</span>Kembali</Link>
    <header className="database-list-heading"><h1 id="database-list-title">Daftar Database</h1><p>Pilih database sesuai hak akses.</p></header>
    {query.isPending ? <Card className="database-list-state"><Spinner label="Memuat daftar database" /><p>Memuat daftar database…</p></Card>
      : query.isError ? <Card className="database-list-state"><Alert tone="error" title="Daftar database belum dapat dimuat">{query.error.message}</Alert><Button variant="secondary" loading={query.isFetching} onClick={() => { void query.refetch() }}>Coba lagi</Button></Card>
      : !query.data?.length ? <Card><EmptyState title="Belum ada database">Database yang dapat Anda akses akan muncul di sini.</EmptyState></Card>
      : <div className="database-list-grid">{query.data.map(database => <ConnectionCard key={database.id} database={database} source={source} />)}</div>}
  </section>
}
