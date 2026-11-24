import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Alert, Badge, Button, Card, ConfirmationModal, EmptyState, Spinner } from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { createAdminApi } from './admin-api'
import type { ConnectionTestResult } from './admin-types'
import './admin-crud.css'

export default function AdminConnectionDetailPage() {
  const { connectionId = '' } = useParams()
  const { state, session } = useSession()
  const api = useMemo(() => createAdminApi(session.api), [session])
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const query = useQuery({
    queryKey: ['admin', 'connection', state.user?.id, connectionId],
    queryFn: ({ signal }) => api.getConnection(connectionId, signal),
    enabled: !!connectionId,
    retry: false,
  })

  const [testResult, setTestResult] = useState<ConnectionTestResult | null>(null)
  const [actionError, setActionError] = useState('')
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)

  const testMutation = useMutation({
    mutationFn: () => api.testSavedConnection(connectionId),
    onSuccess: res => {
      setActionError('')
      setTestResult(res)
    },
    onError: err => {
      setTestResult(null)
      setActionError(err instanceof Error ? err.message : 'Gagal menguji koneksi.')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteConnection(connectionId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['admin', 'connections'] })
      void queryClient.invalidateQueries({ queryKey: ['admin', 'groups'] })
      void queryClient.invalidateQueries({ queryKey: ['admin', 'dashboard'] })
      navigate(paths.connections, { replace: true })
    },
    onError: err => {
      setConfirmDeleteOpen(false)
      setActionError(err instanceof Error ? err.message : 'Gagal menghapus koneksi.')
    },
  })

  if (query.isPending) {
    return (
      <section className="admin-crud-page">
        <div className="admin-status-center">
          <Spinner label="Memuat detail koneksi" />
        </div>
      </section>
    )
  }

  if (query.isError || !query.data) {
    return (
      <section className="admin-crud-page">
        <Alert tone="error" title="Koneksi tidak dapat dimuat">
          {query.error instanceof Error ? query.error.message : 'Koneksi tidak ditemukan.'}
        </Alert>
        <div>
          <Link className="ds-button ds-button--secondary ds-button--md" to={paths.connections}>
            Kembali ke Daftar Koneksi
          </Link>
        </div>
      </section>
    )
  }

  const conn = query.data

  return (
    <section className="admin-crud-page" aria-labelledby="admin-conn-detail-title">
      <header className="admin-crud-header">
        <div>
          <p className="ds-eyebrow">ADMIN / KONEKSI DATABASE / DETAIL</p>
          <h1 id="admin-conn-detail-title">{conn.name}</h1>
          <div className="admin-badge-row">
            <Badge tone={conn.dbms === 'mysql' ? 'warning' : 'info'}>
              {conn.dbms === 'mysql' ? 'MySQL / MariaDB' : 'PostgreSQL'}
            </Badge>
            <Badge tone="neutral">{conn.groups.length} grup memiliki akses</Badge>
          </div>
        </div>
        <div className="admin-crud-actions">
          <Button variant="secondary" loading={testMutation.isPending} onClick={() => testMutation.mutate()}>
            Tes Koneksi
          </Button>
          <Link className="ds-button ds-button--primary ds-button--md" to={paths.editConnection(conn.id)}>
            Ubah Koneksi
          </Link>
          <Button variant="destructive" onClick={() => setConfirmDeleteOpen(true)}>
            Hapus
          </Button>
          <Link className="ds-button ds-button--secondary ds-button--md" to={paths.connections}>
            Kembali
          </Link>
        </div>
      </header>

      {actionError && (
        <Alert tone="error" title="Operasi gagal">
          {actionError}
        </Alert>
      )}

      {testResult && (
        <Alert
          tone={testResult.ok ? 'success' : 'error'}
          title={testResult.ok ? `Koneksi berhasil (${testResult.latencyMs} ms)` : 'Tes koneksi gagal'}
        >
          {testResult.message}
        </Alert>
      )}

      <div className="admin-detail-grid">
        <Card>
          <div className="admin-section-header">
            <h2>Parameter Koneksi</h2>
          </div>
          <div className="admin-table-wrap">
            <table>
              <tbody>
                <tr>
                  <th>Nama Tampilan</th>
                  <td>
                    <strong>{conn.name}</strong>
                  </td>
                </tr>
                <tr>
                  <th>Jenis DBMS</th>
                  <td>{conn.dbms === 'mysql' ? 'MySQL / MariaDB' : 'PostgreSQL'}</td>
                </tr>
                <tr>
                  <th>Host</th>
                  <td>
                    <code>{conn.host}</code>
                  </td>
                </tr>
                <tr>
                  <th>Port</th>
                  <td>
                    <code>{conn.port}</code>
                  </td>
                </tr>
                <tr>
                  <th>Database</th>
                  <td>
                    <code>{conn.database}</code>
                  </td>
                </tr>
                <tr>
                  <th>Username</th>
                  <td>
                    <code>{conn.username}</code>
                  </td>
                </tr>
                <tr>
                  <th>Password</th>
                  <td>
                    <span className="ds-muted">Disimpan tersembunyi (write-only)</span>
                  </td>
                </tr>
                {conn.description && (
                  <tr>
                    <th>Deskripsi</th>
                    <td>{conn.description}</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </Card>

        <Card>
          <div className="admin-section-header">
            <h2>Grup dengan Akses</h2>
            <Link className="ds-button ds-button--tertiary ds-button--sm" to={paths.groups}>
              Kelola Grup
            </Link>
          </div>
          {conn.groups.length === 0 ? (
            <EmptyState title="Belum ada grup dengan akses">
              Berikan izin akses ke database ini melalui halaman Kelola Grup.
            </EmptyState>
          ) : (
            <div className="admin-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Grup</th>
                    <th>Anggota</th>
                    <th>Level</th>
                  </tr>
                </thead>
                <tbody>
                  {conn.groups.map(g => (
                    <tr key={g.name}>
                      <td>
                        <strong>{g.name}</strong>
                      </td>
                      <td>{g.memberCount} anggota</td>
                      <td>
                        <Badge tone={g.level === 'edit' ? 'info' : 'success'}>
                          {g.level === 'edit' ? 'View + Edit' : 'View Only'}
                        </Badge>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      </div>

      <ConfirmationModal
        open={confirmDeleteOpen}
        onClose={() => setConfirmDeleteOpen(false)}
        onConfirm={() => deleteMutation.mutate()}
        title="Hapus koneksi database?"
        confirmLabel="Hapus Koneksi"
        destructive
      >
        Koneksi <strong>{conn.name}</strong> beserta seluruh grant aksesnya akan dihapus permanen. Sesi pengguna yang
        memiliki akses ke database ini akan dicabut.
      </ConfirmationModal>
    </section>
  )
}
