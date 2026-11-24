import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Alert, Badge, Button, Card, EmptyState, Input, Spinner } from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { createAdminApi } from './admin-api'
import './admin-crud.css'

export default function AdminConnectionsPage() {
  const { state, session } = useSession()
  const api = useMemo(() => createAdminApi(session.api), [session])
  const [search, setSearch] = useState('')

  const query = useQuery({
    queryKey: ['admin', 'connections', state.user?.id],
    queryFn: ({ signal }) => api.listConnections(signal),
    retry: false,
  })

  const filtered = useMemo(() => {
    const items = query.data?.items ?? []
    const q = search.trim().toLowerCase()
    if (!q) return items
    return items.filter(
      c =>
        c.name.toLowerCase().includes(q) ||
        c.host.toLowerCase().includes(q) ||
        c.database.toLowerCase().includes(q) ||
        c.dbms.toLowerCase().includes(q),
    )
  }, [query.data, search])

  return (
    <section className="admin-crud-page" aria-labelledby="admin-connections-title">
      <header className="admin-crud-header">
        <div>
          <p className="ds-eyebrow">ADMIN / KONEKSI DATABASE</p>
          <h1 id="admin-connections-title">Kelola Koneksi Database</h1>
          <p>Daftarkan, uji, ubah, dan hapus koneksi ke database target PostgreSQL atau MySQL.</p>
        </div>
        <div className="admin-crud-actions">
          <Link className="ds-button ds-button--primary ds-button--md" to={paths.newConnection}>
            Tambah Koneksi
          </Link>
        </div>
      </header>

      {query.isError && (
        <Alert tone="error" title="Gagal memuat daftar koneksi">
          {query.error instanceof Error ? query.error.message : 'Terjadi kesalahan.'}{' '}
          <Button variant="tertiary" size="sm" onClick={() => query.refetch()}>
            Coba lagi
          </Button>
        </Alert>
      )}

      <Card>
        <div className="admin-toolbar">
          <div className="admin-toolbar-search">
            <Input
              label="Cari koneksi"
              placeholder="Nama, host, database, atau DBMS..."
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
          </div>
          {query.data && <Badge tone="neutral">{filtered.length} koneksi</Badge>}
        </div>

        {query.isPending ? (
          <div className="admin-status-center">
            <Spinner label="Memuat daftar koneksi" />
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState title="Tidak ada koneksi database">
            {search ? 'Tidak ada koneksi yang cocok dengan pencarian.' : 'Belum ada koneksi database terdaftar.'}
          </EmptyState>
        ) : (
          <div className="admin-table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Nama</th>
                  <th>DBMS</th>
                  <th>Host : Port</th>
                  <th>Database</th>
                  <th>Grup dengan Akses</th>
                  <th className="admin-cell-actions">Aksi</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map(c => (
                  <tr key={c.id}>
                    <td>
                      <strong>{c.name}</strong>
                    </td>
                    <td>
                      <Badge tone={c.dbms === 'mysql' ? 'warning' : 'info'}>
                        {c.dbms === 'mysql' ? 'MySQL' : 'PostgreSQL'}
                      </Badge>
                    </td>
                    <td>
                      <code>
                        {c.host}:{c.port}
                      </code>
                    </td>
                    <td>{c.database}</td>
                    <td>
                      {c.groups.length === 0 ? (
                        <span className="ds-muted">Belum ada grup</span>
                      ) : (
                        <div className="admin-badge-row">
                          {c.groups.map(g => (
                            <Badge key={`${c.id}-${g.name}`} tone={g.level === 'edit' ? 'info' : 'success'}>
                              {g.name} ({g.level === 'edit' ? 'View + Edit' : 'View Only'})
                            </Badge>
                          ))}
                        </div>
                      )}
                    </td>
                    <td className="admin-cell-actions">
                      <Link className="ds-button ds-button--secondary ds-button--sm" to={paths.connectionDetail(c.id)}>
                        Kelola
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </section>
  )
}
