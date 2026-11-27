import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Badge, Button, Card, DataTable, EmptyState, Pagination, Spinner } from '../../design-system'
import { formatTableCell } from './table-source'
import type { TableDescription, TableIdentity, TableRecord, TableSource } from './table-source'

export default function TableDataPanel({ source, identity, description, page, direction, cacheKey, loading, onPage, onSort }: {
  source: TableSource; identity: TableIdentity; description: TableDescription; page: number; direction: 'asc' | 'desc'; cacheKey: string[]; loading: boolean; onPage: (page: number, replace?: boolean) => void; onSort: () => void
}) {
  const query = useQuery({ queryKey: ['table-data', ...cacheKey, page, direction], queryFn: ({ signal }) => source.rows({ ...identity, page, direction }, signal), enabled: !loading, retry: false })
  const actualPage = query.data?.page
  useEffect(() => { if (actualPage !== undefined && actualPage !== page) onPage(actualPage, true) }, [actualPage, page, onPage])
  if (query.isPending) return <Card className="schema-state"><Spinner label="Memuat isi tabel" /><p>Memuat isi tabel…</p></Card>
  if (query.isError || !query.data) return <Card className="schema-state"><Alert tone="error" title="Data belum dapat dimuat">{query.error?.message}</Alert><Button variant="secondary" loading={query.isFetching} onClick={() => { void query.refetch() }}>Coba lagi</Button></Card>
  const data = query.data
  const mainColumn = data.columns.find(name => ['email', 'name', 'username'].includes(name)) ?? data.columns.find(name => name !== 'id')
  return <div className="table-data-panel"><div className="table-data-heading"><h2><span className="table-data-desktop-label">Isi table</span><span className="table-data-mobile-label">Isi tabel · View-only · {data.total.toLocaleString('id-ID')} baris</span></h2><Badge tone="neutral">View-only</Badge></div>
    {!data.rows.length ? <EmptyState title="Belum ada data">Tabel ini belum memiliki baris data.</EmptyState> : <>
      <div className="table-detail-mobile table-record-list">{data.rows.map(row => <Card className="table-record-card" key={String(row.id)}><small>id: {formatTableCell(row.id)}</small>{mainColumn && <strong>{formatTableCell(row[mainColumn])}</strong>}<dl>{description.columns.filter(column => column.name !== 'id' && column.name !== mainColumn).map(column => <div key={column.name}><dt>{column.name}</dt><dd className={row[column.name] === null ? 'table-cell-null' : undefined}>{formatTableCell(row[column.name])}</dd></div>)}</dl></Card>)}</div>
      <div className="table-detail-desktop"><DataTable<TableRecord> caption={`Isi tabel ${identity.tableName}`} rows={data.rows} rowKey={row => String(row.id)} columns={data.columns.map(name => ({ key: name, label: name, ...(name === 'id' ? { header: <button className="table-sort-button" type="button" onClick={onSort} aria-label={`Urutkan id ${direction === 'asc' ? 'menurun' : 'menaik'}`}>id <span aria-hidden="true">{direction === 'asc' ? '▲' : '▼'}</span></button>, sortDirection: direction === 'asc' ? 'ascending' as const : 'descending' as const } : {}), render: (row: TableRecord) => <span className={row[name] === null ? 'table-cell-null' : undefined}>{formatTableCell(row[name])}</span> }))} /></div>
    </>}
    <Pagination variant="table" page={data.page} total={data.total} pageSize={data.pageSize} onChange={onPage} />
  </div>
}
