import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  Alert,
  Badge,
  Button,
  Card,
  Choice,
  ConfirmationModal,
  EmptyState,
  Input,
  Modal,
  Spinner,
  Toast,
} from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { createAdminApi } from './admin-api'
import './admin-crud.css'

export default function AdminUserDetailPage() {
  const { userId = '' } = useParams()
  const { state, session } = useSession()
  const api = useMemo(() => createAdminApi(session.api), [session])
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const query = useQuery({
    queryKey: ['admin', 'user', state.user?.id, userId],
    queryFn: ({ signal }) => api.getUser(userId, signal),
    enabled: !!userId,
    retry: false,
  })

  const [draft, setDraft] = useState<{ fullName: string; email: string; isPlatformAdmin: boolean } | null>(null)
  const [actionError, setActionError] = useState('')
  const [toast, setToast] = useState('')

  const [confirmSaveOpen, setConfirmSaveOpen] = useState(false)
  const [confirmDiscardOpen, setConfirmDiscardOpen] = useState(false)
  const [confirmToggleActiveOpen, setConfirmToggleActiveOpen] = useState(false)
  const [resetPasswordOpen, setResetPasswordOpen] = useState(false)
  const [newPassword, setNewPassword] = useState('')
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)

  const fullName = draft?.fullName ?? query.data?.fullName ?? ''
  const email = draft?.email ?? query.data?.email ?? ''
  const isPlatformAdmin = draft?.isPlatformAdmin ?? query.data?.isPlatformAdmin ?? false

  function updateDraft(patch: Partial<{ fullName: string; email: string; isPlatformAdmin: boolean }>) {
    setDraft(prev => ({
      fullName: prev?.fullName ?? query.data?.fullName ?? '',
      email: prev?.email ?? query.data?.email ?? '',
      isPlatformAdmin: prev?.isPlatformAdmin ?? query.data?.isPlatformAdmin ?? false,
      ...patch,
    }))
  }

  const isDirty =
    !!query.data &&
    (fullName.trim() !== query.data.fullName ||
      email.trim() !== query.data.email ||
      isPlatformAdmin !== query.data.isPlatformAdmin)

  function invalidateUser() {
    void queryClient.invalidateQueries({ queryKey: ['admin', 'user', state.user?.id, userId] })
    void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] })
    void queryClient.invalidateQueries({ queryKey: ['admin', 'dashboard'] })
  }

  const updateMutation = useMutation({
    mutationFn: () =>
      api.updateUser(userId, {
        fullName: fullName.trim(),
        email: email.trim(),
        isPlatformAdmin,
      }),
    onSuccess: () => {
      setActionError('')
      setDraft(null)
      setConfirmSaveOpen(false)
      setToast('Perubahan pengguna berhasil disimpan.')
      invalidateUser()
    },
    onError: err => {
      setConfirmSaveOpen(false)
      setActionError(err instanceof Error ? err.message : 'Gagal memperbarui pengguna.')
    },
  })

  const toggleActiveMutation = useMutation({
    mutationFn: () => (query.data?.isActive ? api.deactivateUser(userId) : api.activateUser(userId)),
    onSuccess: updated => {
      setActionError('')
      setConfirmToggleActiveOpen(false)
      setToast(updated.isActive ? 'Akun pengguna diaktifkan kembali.' : 'Akun pengguna dinonaktifkan.')
      invalidateUser()
    },
    onError: err => {
      setConfirmToggleActiveOpen(false)
      setActionError(err instanceof Error ? err.message : 'Gagal mengubah status akun.')
    },
  })

  const resetPasswordMutation = useMutation({
    mutationFn: () => api.resetUserPassword(userId, newPassword),
    onSuccess: () => {
      setActionError('')
      setResetPasswordOpen(false)
      setNewPassword('')
      setToast('Password pengguna berhasil direset dan sesi lama dicabut.')
    },
    onError: err => {
      setActionError(err instanceof Error ? err.message : 'Gagal mereset password.')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteUser(userId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] })
      void queryClient.invalidateQueries({ queryKey: ['admin', 'dashboard'] })
      navigate(paths.users, { replace: true })
    },
    onError: err => {
      setConfirmDeleteOpen(false)
      setActionError(err instanceof Error ? err.message : 'Gagal menghapus pengguna.')
    },
  })

  function handleCancelClick() {
    if (isDirty) {
      setConfirmDiscardOpen(true)
    } else {
      navigate(paths.users)
    }
  }

  function handleDiscardConfirm() {
    setDraft(null)
    setConfirmDiscardOpen(false)
    navigate(paths.users)
  }

  if (query.isPending) {
    return (
      <section className="admin-crud-page">
        <div className="admin-status-center">
          <Spinner label="Memuat detail pengguna" />
        </div>
      </section>
    )
  }

  if (query.isError || !query.data) {
    return (
      <section className="admin-crud-page">
        <Alert tone="error" title="Pengguna tidak dapat dimuat">
          {query.error instanceof Error ? query.error.message : 'Pengguna tidak ditemukan.'}
        </Alert>
        <div>
          <Link className="ds-button ds-button--secondary ds-button--md" to={paths.users}>
            Kembali ke Daftar Pengguna
          </Link>
        </div>
      </section>
    )
  }

  const user = query.data

  return (
    <section className="admin-crud-page" aria-labelledby="admin-user-detail-title">
      <header className="admin-crud-header">
        <div>
          <p className="ds-eyebrow">ADMIN / PENGGUNA / DETAIL</p>
          <h1 id="admin-user-detail-title">{user.username}</h1>
          <div className="admin-badge-row">
            <Badge tone={user.isActive ? 'success' : 'error'}>{user.isActive ? 'Aktif' : 'Nonaktif'}</Badge>
            <Badge tone={user.isPlatformAdmin ? 'info' : 'neutral'}>
              {user.isPlatformAdmin ? 'Platform Admin' : 'Non-admin'}
            </Badge>
            {user.isSelf && <Badge tone="neutral">Akun Anda</Badge>}
          </div>
        </div>
        <div className="admin-crud-actions">
          <Button variant="secondary" onClick={handleCancelClick}>
            Kembali
          </Button>
        </div>
      </header>

      {actionError && (
        <Alert tone="error" title="Operasi gagal">
          {actionError}
        </Alert>
      )}

      <div className="admin-detail-grid">
        <Card>
          <div className="admin-section-header">
            <h2>Informasi & Peran Akun</h2>
          </div>
          <form
            className="admin-form-grid"
            onSubmit={e => {
              e.preventDefault()
              setConfirmSaveOpen(true)
            }}
          >
            <Input label="Username" value={user.username} disabled />
            <Input label="Nama lengkap" value={fullName} onChange={e => updateDraft({ fullName: e.target.value })} />
            <Input label="Email" type="email" value={email} onChange={e => updateDraft({ email: e.target.value })} />
            <Choice
              label="Platform Admin"
              checked={isPlatformAdmin}
              disabled={user.isSelf}
              onChange={e => updateDraft({ isPlatformAdmin: e.target.checked })}
            />
            {user.isSelf && (
              <small className="ds-muted">Peran admin pada akun sendiri tidak dapat dicabut dari halaman ini.</small>
            )}
            <div className="admin-form-footer">
              <Button type="submit" disabled={!isDirty} loading={updateMutation.isPending}>
                Simpan Perubahan
              </Button>
              <Button variant="secondary" onClick={handleCancelClick}>
                Batal
              </Button>
            </div>
          </form>
        </Card>

        <div className="admin-crud-page" style={{ paddingBottom: 0 }}>
          <Card>
            <div className="admin-section-header">
              <h2>Keamanan & Status Akun</h2>
            </div>
            <div className="admin-danger-actions">
              <Button variant="secondary" onClick={() => setResetPasswordOpen(true)}>
                Reset Password
              </Button>
              <Button
                variant={user.isActive ? 'destructive' : 'primary'}
                disabled={user.isSelf}
                onClick={() => setConfirmToggleActiveOpen(true)}
              >
                {user.isActive ? 'Nonaktifkan Akun' : 'Aktifkan Akun'}
              </Button>
              <Button variant="destructive" disabled={user.isSelf} onClick={() => setConfirmDeleteOpen(true)}>
                Hapus Pengguna
              </Button>
            </div>
          </Card>

          <Card>
            <div className="admin-section-header">
              <h2>Grup & Akses Efektif</h2>
              <Badge tone="neutral">{user.groups.length} grup</Badge>
            </div>
            {user.groups.length === 0 ? (
              <EmptyState title="Belum tergabung di grup">
                Tambahkan user ini ke grup pada menu Kelola Grup untuk memberi akses database.
              </EmptyState>
            ) : (
              <div className="admin-table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Database</th>
                      <th>Grup</th>
                      <th>Level</th>
                    </tr>
                  </thead>
                  <tbody>
                    {user.effectiveAccess.length === 0 ? (
                      <tr>
                        <td colSpan={3}>Grup: {user.groups.join(', ')} (belum ada grant database)</td>
                      </tr>
                    ) : (
                      user.effectiveAccess.map(row => (
                        <tr key={`${row.connectionId}-${row.groupId}`}>
                          <td>{row.connectionName}</td>
                          <td>{row.groupName}</td>
                          <td>
                            <Badge tone={row.level === 'edit' ? 'info' : row.level === 'view' ? 'success' : 'neutral'}>
                              {row.level === 'edit' ? 'View + Edit' : row.level === 'view' ? 'View Only' : 'Tanpa akses'}
                            </Badge>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        </div>
      </div>

      <ConfirmationModal
        open={confirmSaveOpen}
        onClose={() => setConfirmSaveOpen(false)}
        onConfirm={() => updateMutation.mutate()}
        title="Simpan perubahan pengguna?"
        confirmLabel="Simpan"
      >
        Perubahan profil dan peran untuk <strong>{user.username}</strong> akan segera diterapkan.
      </ConfirmationModal>

      <ConfirmationModal
        open={confirmDiscardOpen}
        onClose={() => setConfirmDiscardOpen(false)}
        onConfirm={handleDiscardConfirm}
        title="Batalkan perubahan?"
        confirmLabel="Buang Perubahan"
        destructive
      >
        Anda memiliki perubahan yang belum disimpan. Yakin ingin keluar dan membuang perubahan tersebut?
      </ConfirmationModal>

      <ConfirmationModal
        open={confirmToggleActiveOpen}
        onClose={() => setConfirmToggleActiveOpen(false)}
        onConfirm={() => toggleActiveMutation.mutate()}
        title={user.isActive ? 'Nonaktifkan pengguna?' : 'Aktifkan pengguna?'}
        confirmLabel={user.isActive ? 'Nonaktifkan' : 'Aktifkan'}
        destructive={user.isActive}
      >
        {user.isActive
          ? `Akun ${user.username} akan dinonaktifkan dan seluruh sesinya dicabut.`
          : `Akun ${user.username} akan diaktifkan kembali dan dapat masuk ke platform.`}
      </ConfirmationModal>

      <Modal
        open={resetPasswordOpen}
        onClose={() => {
          setResetPasswordOpen(false)
          setNewPassword('')
        }}
        title="Reset Password Pengguna"
        footer={
          <>
            <Button
              variant="secondary"
              onClick={() => {
                setResetPasswordOpen(false)
                setNewPassword('')
              }}
            >
              Batal
            </Button>
            <Button
              disabled={newPassword.length < 8}
              loading={resetPasswordMutation.isPending}
              onClick={() => resetPasswordMutation.mutate()}
            >
              Simpan Password Baru
            </Button>
          </>
        }
      >
        <Input
          label="Password baru"
          type="password"
          required
          helper="Minimal 8 karakter, maksimal 72 byte. Sesi aktif user akan dicabut."
          value={newPassword}
          onChange={e => setNewPassword(e.target.value)}
          autoComplete="new-password"
        />
      </Modal>

      <ConfirmationModal
        open={confirmDeleteOpen}
        onClose={() => setConfirmDeleteOpen(false)}
        onConfirm={() => deleteMutation.mutate()}
        title="Hapus pengguna?"
        confirmLabel="Hapus"
        destructive
      >
        Akun <strong>{user.username}</strong> akan dihapus permanen. Penghapusan ditolak jika user memiliki riwayat
        migration atau merupakan admin aktif terakhir.
      </ConfirmationModal>

      {toast && <Toast message={toast} onDismiss={() => setToast('')} />}
    </section>
  )
}
