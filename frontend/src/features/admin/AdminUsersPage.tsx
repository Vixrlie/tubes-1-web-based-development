import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Alert, Badge, Button, Card, EmptyState, Input, Spinner } from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { createAdminApi } from './admin-api'
import './admin-crud.css'

export default function AdminUsersPage() {
  const { state, session } = useSession()
  const api = useMemo(() => createAdminApi(session.api), [session])
  const [search, setSearch] = useState('')

  const query = useQuery({
    queryKey: ['admin', 'users', state.user?.id],
    queryFn: ({ signal }) => api.listUsers(signal),
    retry: false,
  })

  const filtered = useMemo(() => {
    const items = query.data?.items ?? []
    const q = search.trim().toLowerCase()
    if (!q) return items
    return items.filter(
      u =>
        u.username.toLowerCase().includes(q) ||
        u.fullName.toLowerCase().includes(q) ||
        u.email.toLowerCase().includes(q) ||
        u.groups.some(g => g.toLowerCase().includes(q)),
    )
  }, [query.data, search])

  return (
    <section className="admin-crud-page" aria-labelledby="admin-users-title">
      <header className="admin-crud-header">
        <div>
          <p className="ds-eyebrow">ADMIN / PENGGUNA</p>
          <h1 id="admin-users-title">Kelola Pengguna</h1>
          <p>Atur akun pengguna platform, peran administrator, status aktif, dan kata sandi.</p>
        </div>
        <div className="admin-crud-actions">
          <Link className="ds-button ds-button--primary ds-button--md" to={paths.newUser}>
            Tambah Pengguna
          </Link>
        </div>
      </header>

      {query.isError && (
        <Alert tone="error" title="Gagal memuat daftar pengguna">
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
              label="Cari pengguna"
              placeholder="Username, nama, email, atau grup..."
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
          </div>
          {query.data && <Badge tone="neutral">{filtered.length} pengguna</Badge>}
        </div>

        {query.isPending ? (
          <div className="admin-status-center">
            <Spinner label="Memuat pengguna" />
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState title="Tidak ada pengguna">
            {search ? 'Tidak ada pengguna yang cocok dengan pencarian.' : 'Belum ada akun pengguna terdaftar.'}
          </EmptyState>
        ) : (
          <div className="admin-table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Username</th>
                  <th>Nama</th>
                  <th>Status</th>
                  <th>Peran</th>
                  <th>Grup</th>
                  <th className="admin-cell-actions">Aksi</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map(u => (
                  <tr key={u.id}>
                    <td>
                      <strong>{u.username}</strong>
                    </td>
                    <td>{u.fullName || '-'}</td>
                    <td>
                      <Badge tone={u.isActive ? 'success' : 'error'}>
                        {u.isActive ? 'Aktif' : 'Nonaktif'}
                      </Badge>
                    </td>
                    <td>
                      <Badge tone={u.isPlatformAdmin ? 'info' : 'neutral'}>
                        {u.isPlatformAdmin ? 'Admin' : 'Non-admin'}
                      </Badge>
                    </td>
                    <td>{u.groups.length > 0 ? u.groups.join(', ') : '-'}</td>
                    <td className="admin-cell-actions">
                      <Link className="ds-button ds-button--secondary ds-button--sm" to={paths.userDetail(u.id)}>
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
