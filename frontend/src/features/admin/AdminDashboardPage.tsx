import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Alert, Button, Card, Spinner } from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { createAdminApi } from './admin-api'
import './admin-crud.css'

export default function AdminDashboardPage() {
  const { state, session } = useSession()
  const api = useMemo(() => createAdminApi(session.api), [session])

  const query = useQuery({
    queryKey: ['admin', 'dashboard', state.user?.id],
    queryFn: ({ signal }) => api.dashboard(signal),
    retry: false,
  })

  return (
    <section className="admin-crud-page" aria-labelledby="admin-dashboard-title">
      <header className="admin-crud-header">
        <div>
          <p className="ds-eyebrow">ADMIN / DASHBOARD</p>
          <h1 id="admin-dashboard-title">Dashboard Admin</h1>
          <p>Ringkasan akun platform, grup akses, koneksi database, dan jalan pintas administrasi.</p>
        </div>
        <div className="admin-crud-actions">
          <Link className="ds-button ds-button--secondary ds-button--md" to={paths.activity}>
            Activity Logs
          </Link>
          <Link className="ds-button ds-button--tertiary ds-button--md" to={paths.observability}>
            Observability
          </Link>
        </div>
      </header>

      {query.isPending && (
        <div className="admin-status-center">
          <Spinner label="Memuat ringkasan admin" />
        </div>
      )}

      {query.isError && (
        <Alert tone="error" title="Gagal memuat dashboard">
          {query.error instanceof Error ? query.error.message : 'Terjadi kesalahan saat memuat data.'}{' '}
          <Button variant="tertiary" size="sm" onClick={() => query.refetch()}>
            Coba lagi
          </Button>
        </Alert>
      )}

      {query.data && (
        <>
          <div className="admin-stats-grid">
            <Card className="admin-stat-card">
              <span className="admin-stat-label">Pengguna Platform</span>
              <strong className="admin-stat-value">{query.data.totalUsers}</strong>
              <span className="admin-stat-label">{query.data.activeUsers} akun aktif</span>
              <Link className="ds-button ds-button--primary ds-button--md" to={paths.users}>
                Kelola Pengguna
              </Link>
            </Card>

            <Card className="admin-stat-card">
              <span className="admin-stat-label">Grup Akses</span>
              <strong className="admin-stat-value">{query.data.totalGroups}</strong>
              <span className="admin-stat-label">Pengaturan anggota dan grant database</span>
              <Link className="ds-button ds-button--primary ds-button--md" to={paths.groups}>
                Kelola Grup
              </Link>
            </Card>

            <Card className="admin-stat-card">
              <span className="admin-stat-label">Koneksi Database</span>
              <strong className="admin-stat-value">{query.data.totalConnections}</strong>
              <span className="admin-stat-label">Database target yang terdaftar</span>
              <Link className="ds-button ds-button--primary ds-button--md" to={paths.connections}>
                Kelola Koneksi
              </Link>
            </Card>
          </div>

          <Card>
            <div className="admin-section-header">
              <h2>Aksi Cepat</h2>
            </div>
            <div className="admin-crud-actions">
              <Link className="ds-button ds-button--secondary ds-button--md" to={paths.newUser}>
                Tambah Pengguna
              </Link>
              <Link className="ds-button ds-button--secondary ds-button--md" to={paths.newGroup}>
                Buat Grup Baru
              </Link>
              <Link className="ds-button ds-button--secondary ds-button--md" to={paths.newConnection}>
                Tambah Koneksi
              </Link>
            </div>
          </Card>
        </>
      )}
    </section>
  )
}
