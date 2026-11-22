import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { Alert, Badge, Button, Card, EmptyState, Spinner, SqlPreview } from '../../design-system'
import { paths } from '../../app/routes'
import { executionAction, executionStatus, withMigrationSearch } from './migration-source'
import { useMigrations } from './useMigrations'
import './migrations.css'

export default function MigrationDetailPage() {
  const { migrationId = '' } = useParams()
  // Remount copy feedback when another execution is opened in the same outlet.
  return <ExecutionDetail key={migrationId} executionId={migrationId} />
}
function ExecutionDetail({ executionId }: { executionId: string }) {
  const query = useMigrations(executionId)
  const [params] = useSearchParams()
  const [copyStatus, setCopyStatus] = useState<'idle' | 'success' | 'error'>('idle')
  const { database } = query.workspace
  const row = query.execution
  const historyPath = withMigrationSearch(paths.migrations(database.id), params)
  const historyLink = <Link className="ds-button ds-button--secondary ds-button--sm" to={historyPath}>Kembali ke Riwayat Migrasi</Link>
  if (query.isPending) return <Card className="schema-state"><Spinner label="Memuat detail eksekusi" /><p>Memuat detail eksekusi.</p></Card>
  if (query.isError) return <Card className="schema-state"><Alert tone="error" title="Detail eksekusi belum dapat dimuat">{query.error.message}</Alert><Button variant="secondary" loading={query.isFetching} onClick={() => { void query.refetch() }}>Coba lagi</Button>{historyLink}</Card>
  if (!row) return <Card><EmptyState title="Eksekusi tidak ditemukan" action={historyLink}>Pilih eksekusi yang tersedia dalam riwayat database ini.</EmptyState></Card>
  async function copySql() {
    try { await navigator.clipboard.writeText(row!.sql); setCopyStatus('success') } catch { setCopyStatus('error') }
  }
  return <div className="migration-detail-page"><header className="migration-detail-heading"><div className="schema-heading"><h1>Execution {row.id} · {row.status === 'success' ? 'Success' : 'Failed'}</h1><p>{database.name} › <Link to={historyPath}>Migrasi</Link> › {row.version} › {row.id}</p></div>{historyLink}</header>
    <Alert tone={row.status === 'success' ? 'success' : 'error'} title={row.status === 'success' ? 'Migration berhasil di-commit' : 'Migration gagal dieksekusi'}>{row.status === 'success' ? `Migration berhasil diterapkan pada ${database.name}.` : row.error ?? 'Periksa SQL dan detail eksekusi sebelum mencoba kembali.'}</Alert>
    {row.irreversible && <Alert tone="warning" title="Migrasi irreversible">Eksekusi ini tidak menyediakan SQL Down untuk rollback.</Alert>}
    {row.rollbackTarget && <Alert title={`Rollback ${row.rollbackTarget}`}>Eksekusi ini membatalkan perubahan pada <Link to={withMigrationSearch(paths.migration(database.id, row.rollbackTarget), params)}>{row.rollbackTarget}</Link>.</Alert>}
    <dl className="migration-execution-metadata">{[
      { label: 'Versi', value: row.version },
      { label: 'Execution Status', value: <Badge tone={row.status === 'success' ? 'success' : 'error'}>{executionStatus(row.status)}</Badge> },
      { label: 'Pelaksana', value: row.actor },
      { label: 'Waktu eksekusi', value: row.executedAt },
      { label: 'Durasi', value: row.duration },
    ].map(item => <div key={item.label}><dt>{item.label}</dt><dd>{item.value}</dd></div>)}</dl>
    <Card className="migration-sql"><div><h2>SQL yang Dijalankan</h2><Button variant="secondary" size="sm" onClick={() => { void copySql() }}>{copyStatus === 'success' ? 'Tersalin' : 'Salin SQL'}</Button></div><SqlPreview value={row.sql} label={`${row.version} · ${row.name} · ${executionAction(row)}`} />{copyStatus === 'success' && <p role="status">SQL berhasil disalin.</p>}{copyStatus === 'error' && <Alert tone="error" title="SQL belum dapat disalin">Pilih teks SQL lalu salin secara manual.</Alert>}</Card>
    <div className="migration-detail-mobile-back">{historyLink}</div>
  </div>
}
