import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Alert, Badge, Button, Card, EmptyState, Input, Spinner } from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { createAdminApi } from './admin-api'
import './admin-crud.css'

export default function AdminGroupsPage() {
  const { state, session } = useSession()
  const api = useMemo(() => createAdminApi(session.api), [session])
  const [search, setSearch] = useState('')

  const query = useQuery({
    queryKey: ['admin', 'groups', state.user?.id],
    queryFn: ({ signal }) => api.listGroups(signal),
    retry: false,
  })

  const filtered = useMemo(() => {
    const items = query.data?.items ?? []
    const q = search.trim().toLowerCase()
    if (!q) return items
    return items.filter(
      g =>
        g.name.toLowerCase().includes(q) ||
        g.accesses.some(a => a.connectionName.toLowerCase().includes(q)),
    )
  }, [query.data, search])

  return (
    <section className="admin-crud-page" aria-labelledby="admin-groups-title">
      <header className="admin-crud-header">
        <div>
          <p className="ds-eyebrow">ADMIN / GRUP</p>
          <h1 id="admin-groups-title">Kelola Grup</h1>
          <p>Atur keanggotaan pengguna dan pemberian izin akses database per grup.</p>
        </div>
        <div className="admin-crud-actions">
          <Link className="ds-button ds-button--primary ds-button--md" to={paths.newGroup}>
            Grup Baru
          </Link>
        </div>
      </header>

      {query.isError && (
        <Alert tone="error" title="Gagal memuat daftar grup">
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
              label="Cari grup"
              placeholder="Nama grup atau nama database..."
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
          </div>
          {query.data && <Badge tone="neutral">{filtered.length} grup</Badge>}
        </div>

        {query.isPending ? (
          <div className="admin-status-center">
            <Spinner label="Memuat daftar grup" />
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState title="Tidak ada grup">
            {search ? 'Tidak ada grup yang cocok dengan pencarian.' : 'Belum ada grup terdaftar.'}
          </EmptyState>
        ) : (
          <div className="admin-table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Nama Grup</th>
                  <th>Anggota</th>
                  <th>Akses Database</th>
                  <th className="admin-cell-actions">Aksi</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map(g => (
                  <tr key={g.id}>
                    <td>
                      <strong>{g.name}</strong>
                    </td>
                    <td>{g.memberCount} anggota</td>
                    <td>
                      {g.accesses.length === 0 ? (
                        <span className="ds-muted">Belum ada akses</span>
                      ) : (
                        <div className="admin-badge-row">
                          {g.accesses.map(a => (
                            <Badge key={`${g.id}-${a.connectionName}`} tone={a.level === 'edit' ? 'info' : 'success'}>
                              {a.connectionName}: {a.level === 'edit' ? 'View + Edit' : 'View Only'}
                            </Badge>
                          ))}
                        </div>
                      )}
                    </td>
                    <td className="admin-cell-actions">
                      <Link className="ds-button ds-button--secondary ds-button--sm" to={paths.groupDetail(g.id)}>
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
