import { lazy, Suspense, useId } from 'react'
import type { ReactNode } from 'react'
import { Alert, Badge, Button, Card, EmptyState, Spinner } from './primitives'
import type { Tone } from './primitives'
import { Icon } from './navigation'
import { paginationItems } from './pagination'

const SqlCodeMirror = lazy(() => import('./sql-editor'))

export function DatabaseCard({ name, description, connected = true, permission, onOpen, loading = false, disabled = false, onReconnect, variant = 'default', readOnly = false, connectionError }: {
  name: string; description: string; connected?: boolean; permission: string; onOpen: () => void
  loading?: boolean; disabled?: boolean; onReconnect?: () => void
  variant?: 'default' | 'workspace'; readOnly?: boolean; connectionError?: string
}) {
  if (variant === 'workspace') return <Card className="ds-database-tile" role="group" aria-label={name} aria-busy={loading || undefined}>
    <div className="ds-database-tile-heading"><span className="ds-database-tile-icon"><picture><source media="(min-width: 768px)" srcSet={`/design-system/database-${connected ? 'connected' : 'disconnected'}.svg`} /><img src="/design-system/database-mobile.svg" alt="" /></picture></span><Badge tone={loading ? 'neutral' : connected ? 'success' : 'error'}>{loading ? 'Memeriksa…' : connected ? 'Terhubung' : 'Gagal terhubung'}</Badge></div>
    <div className="ds-database-tile-description"><h3>{name}</h3><p>{description}</p></div>
    <div className="ds-database-tile-permission"><Badge tone={readOnly ? 'neutral' : 'info'}>{permission}</Badge></div>
    {connectionError && <Alert tone="error" title={connectionError} />}
    <div className="ds-database-tile-action">{!connected && onReconnect ? <Button variant="secondary" aria-label={`Coba lagi koneksi ${name}`} disabled={disabled} loading={loading} onClick={onReconnect}>Coba lagi</Button> : <Button aria-label={`Buka ${name}`} disabled={disabled || !connected} loading={loading} onClick={onOpen}>Buka</Button>}</div>
  </Card>
  return <Card className="ds-database-card" aria-busy={loading || undefined}>
    <div className="ds-row ds-between"><Icon name="database" /><Badge tone={loading ? 'neutral' : connected ? 'success' : 'error'}>{loading ? 'Memeriksa koneksi' : connected ? 'Terhubung' : 'Terputus'}</Badge></div>
    <div><h3>{name}</h3><p className="ds-muted">{description}</p></div><div><Badge>{permission}</Badge></div><hr />
    {!connected && onReconnect ? <Button variant="secondary" disabled={disabled} loading={loading} onClick={onReconnect}>Hubungkan kembali</Button>
      : <Button disabled={disabled || !connected} loading={loading} onClick={onOpen}>Buka</Button>}
  </Card>
}

export function KpiCard({ label, value, detail }: { label: string; value: string | number; detail?: string }) {
  return <Card className="ds-stack"><small>{label}</small><strong className="ds-kpi-value">{value}</strong>{detail && <small>{detail}</small>}</Card>
}

export type Column<T> = { key: string; label: string; header?: ReactNode; sortDirection?: 'ascending' | 'descending'; render: (row: T) => ReactNode }
export function DataTable<T>({ columns, rows, rowKey, caption }: { columns: Column<T>[]; rows: T[]; rowKey: (row: T) => string; caption: string }) {
  if (!rows.length) return <EmptyState title="Belum ada data">Data yang sesuai akan muncul di sini.</EmptyState>
  return <div className="ds-table-scroll" tabIndex={0} role="region" aria-label={caption}><table className="ds-table"><caption className="sr-only">{caption}</caption><thead><tr>{columns.map(col => <th key={col.key} scope="col" aria-sort={col.sortDirection}>{col.header ?? col.label}</th>)}</tr></thead><tbody>{rows.map(row => <tr key={rowKey(row)}>{columns.map(col => <td key={col.key} data-label={col.label}>{col.render(row)}</td>)}</tr>)}</tbody></table></div>
}

export function Pagination({ page, total, pageSize, onChange, variant = 'default' }: { page: number; total: number; pageSize: number; onChange: (page: number) => void; variant?: 'default' | 'table' }) {
  const pages = Math.max(1, Math.ceil(total / pageSize)); const current = Math.min(pages, Math.max(1, page))
  if (variant === 'table') return <nav className="ds-pagination ds-pagination--table" aria-label="Paginasi data tabel"><small>Menampilkan {total ? (current - 1) * pageSize + 1 : 0}–{Math.min(current * pageSize, total)} dari {total.toLocaleString('id-ID')} baris</small><div className="ds-row"><Button variant="secondary" size="sm" aria-label="Halaman sebelumnya" disabled={current === 1} onClick={() => onChange(current - 1)}>‹ Prev</Button><span className="ds-pagination-current" aria-live="polite">Halaman {current} / {pages}</span><span className="ds-pagination-numbers">{paginationItems(current, pages).map(item => typeof item === 'number' ? <Button key={item} variant="secondary" size="sm" aria-current={item === current ? 'page' : undefined} aria-label={`Halaman ${item}`} onClick={() => onChange(item)}>{item}</Button> : <span key={item} aria-hidden="true">…</span>)}</span><Button variant="secondary" size="sm" aria-label="Halaman berikutnya" disabled={current === pages} onClick={() => onChange(current + 1)}>Next ›</Button></div></nav>
  return <nav className="ds-pagination" aria-label="Paginasi"><small>{total === 0 ? 0 : (current - 1) * pageSize + 1}–{Math.min(current * pageSize, total)} dari {total}</small><div className="ds-row"><Button variant="secondary" size="sm" disabled={current === 1} onClick={() => onChange(current - 1)}>Sebelumnya</Button><span aria-live="polite">{current} / {pages}</span><Button variant="secondary" size="sm" disabled={current === pages} onClick={() => onChange(current + 1)}>Berikutnya</Button></div></nav>
}

export function MetadataRow({ label, children }: { label: string; children: ReactNode }) {
  return <div className="ds-metadata"><span className="ds-muted">{label}</span><span>{children}</span></div>
}

export function ListRow({ title, detail, trailing }: { title: string; detail?: string; trailing?: ReactNode }) {
  return <div className="ds-list-row"><div><strong>{title}</strong>{detail && <p className="ds-muted">{detail}</p>}</div>{trailing}</div>
}

export function BatchProgress({ value, total, detail }: { value: number; total: number; detail?: string }) {
  const id = useId(); const safeTotal = Math.max(0, total); const safeValue = Math.min(safeTotal, Math.max(0, value)); const percent = safeTotal ? Math.round(safeValue / safeTotal * 100) : 0
  return <Card className="ds-stack"><strong id={id}>Batch Processing</strong><span className="ds-accent">{safeValue}/{safeTotal} batches · {percent}%</span><progress aria-labelledby={id} value={safeValue} max={safeTotal || 1} />{detail && <small>{detail}</small>}</Card>
}

export function SqlEditor({ value, onChange, readOnly = false, label = 'SQL UP', error }: { value: string; onChange?: (value: string) => void; readOnly?: boolean; label?: string; error?: string }) {
  const id = useId()
  return <div className={`ds-sql ${error ? 'ds-sql--error' : ''}`}><div id={id} className="ds-sql-label">{label}</div><Suspense fallback={<div className="ds-sql-loading"><Spinner /></div>}><SqlCodeMirror value={value} onChange={onChange} readOnly={readOnly} labelId={id} errorId={error ? `${id}-error` : undefined} /></Suspense>{error && <p className="ds-sql-error" id={`${id}-error`}>{error}</p>}</div>
}
export function SqlPreview({ value, label }: { value: string; label?: string }) { return <SqlEditor value={value} label={label} readOnly /> }
export function MigrationJobRow({ job, migration, detail, tone = 'info' }: { job: string; migration: string; detail: string; tone?: Tone }) { return <ListRow title={job} detail={`${migration} · ${detail}`} trailing={<Badge tone={tone}>{tone === 'success' ? 'Selesai' : tone === 'error' ? 'Gagal' : 'Berjalan'}</Badge>} /> }
export function ApprovalGroupRow({ name, detail, status, tone = 'warning' }: { name: string; detail: string; status: string; tone?: Tone }) { return <ListRow title={name} detail={detail} trailing={<Badge tone={tone}>{status}</Badge>} /> }
export function SchemaDiffRow({ name, before, after }: { name: string; before: string; after: string }) { return <div className="ds-diff"><strong>{name}</strong><del>{before}</del><ins>{after}</ins></div> }
