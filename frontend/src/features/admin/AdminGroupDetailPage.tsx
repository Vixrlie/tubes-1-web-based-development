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
  Select,
  Spinner,
  Toast,
} from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { createAdminApi } from './admin-api'
import type { AdminMember } from './admin-types'
import './admin-crud.css'

type DraftGrant = {
  connectionId: string
  connectionName: string
  level: 'view' | 'edit'
}

export default function AdminGroupDetailPage() {
  const { groupId = '' } = useParams()
  const { state, session } = useSession()
  const api = useMemo(() => createAdminApi(session.api), [session])
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const query = useQuery({
    queryKey: ['admin', 'group', state.user?.id, groupId],
    queryFn: ({ signal }) => api.getGroup(groupId, signal),
    enabled: !!groupId,
    retry: false,
  })

  const [actionError, setActionError] = useState('')
  const [toast, setToast] = useState('')

  // Rename state
  const [editingName, setEditingName] = useState(false)
  const [draftName, setDraftName] = useState('')

  // Delete group modal
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)

  // Members modals
  const [addMembersOpen, setAddMembersOpen] = useState(false)
  const [memberSearch, setMemberSearch] = useState('')
  const [selectedUserIds, setSelectedUserIds] = useState<string[]>([])
  const [memberToRemove, setMemberToRemove] = useState<AdminMember | null>(null)

  // Access editor state
  const [editingAccess, setEditingAccess] = useState(false)
  const [draftGrants, setDraftGrants] = useState<DraftGrant[]>([])
  const [addDbOpen, setAddDbOpen] = useState(false)
  const [newDbId, setNewDbId] = useState('')
  const [newDbLevel, setNewDbLevel] = useState<'view' | 'edit'>('view')
  const [grantToRevoke, setGrantToRevoke] = useState<DraftGrant | null>(null)

  function invalidateGroup() {
    void queryClient.invalidateQueries({ queryKey: ['admin', 'group', state.user?.id, groupId] })
    void queryClient.invalidateQueries({ queryKey: ['admin', 'groups'] })
    void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] })
    void queryClient.invalidateQueries({ queryKey: ['admin', 'dashboard'] })
  }

  const renameMutation = useMutation({
    mutationFn: () => api.renameGroup(groupId, draftName.trim()),
    onSuccess: () => {
      setActionError('')
      setEditingName(false)
      setToast('Nama grup berhasil diperbarui.')
      invalidateGroup()
    },
    onError: err => {
      setActionError(err instanceof Error ? err.message : 'Gagal mengubah nama grup.')
    },
  })

  const deleteGroupMutation = useMutation({
    mutationFn: () => api.deleteGroup(groupId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['admin', 'groups'] })
      void queryClient.invalidateQueries({ queryKey: ['admin', 'dashboard'] })
      navigate(paths.groups, { replace: true })
    },
    onError: err => {
      setConfirmDeleteOpen(false)
      setActionError(err instanceof Error ? err.message : 'Gagal menghapus grup.')
    },
  })

  const addMembersMutation = useMutation({
    mutationFn: () => api.addGroupMembers(groupId, selectedUserIds),
    onSuccess: () => {
      setActionError('')
      setAddMembersOpen(false)
      setSelectedUserIds([])
      setMemberSearch('')
      setToast('Anggota berhasil ditambahkan ke grup.')
      invalidateGroup()
    },
    onError: err => {
      setActionError(err instanceof Error ? err.message : 'Gagal menambahkan anggota.')
    },
  })

  const removeMemberMutation = useMutation({
    mutationFn: (uid: string) => api.removeGroupMember(groupId, uid),
    onSuccess: () => {
      setActionError('')
      setMemberToRemove(null)
      setToast('Anggota berhasil dihapus dari grup.')
      invalidateGroup()
    },
    onError: err => {
      setMemberToRemove(null)
      setActionError(err instanceof Error ? err.message : 'Gagal menghapus anggota.')
    },
  })

  const saveAccessMutation = useMutation({
    mutationFn: () =>
      api.updateGroupAccess(
        groupId,
        draftGrants.map(g => ({ connectionId: g.connectionId, level: g.level })),
      ),
    onSuccess: () => {
      setActionError('')
      setEditingAccess(false)
      setToast('Perubahan akses database grup berhasil disimpan.')
      invalidateGroup()
    },
    onError: err => {
      setActionError(err instanceof Error ? err.message : 'Gagal menyimpan akses database.')
    },
  })

  if (query.isPending) {
    return (
      <section className="admin-crud-page">
        <div className="admin-status-center">
          <Spinner label="Memuat detail grup" />
        </div>
      </section>
    )
  }

  if (query.isError || !query.data) {
    return (
      <section className="admin-crud-page">
        <Alert tone="error" title="Grup tidak dapat dimuat">
          {query.error instanceof Error ? query.error.message : 'Grup tidak ditemukan.'}
        </Alert>
        <div>
          <Link className="ds-button ds-button--secondary ds-button--md" to={paths.groups}>
            Kembali ke Daftar Grup
          </Link>
        </div>
      </section>
    )
  }

  const group = query.data

  const filteredCandidates = group.candidateUsers.filter(u => {
    const q = memberSearch.trim().toLowerCase()
    if (!q) return true
    return u.username.toLowerCase().includes(q) || u.fullName.toLowerCase().includes(q)
  })

  const allConnectionsPool = [
    ...group.availableConnections,
    ...group.grants
      .filter(g => !!g.connectionId)
      .map(g => ({ id: g.connectionId!, name: g.connectionName })),
  ]
  const draftIds = new Set(draftGrants.map(g => g.connectionId))
  const addableConnections = allConnectionsPool.filter(c => !draftIds.has(c.id))

  function startEditAccess() {
    setDraftGrants(
      group.grants
        .filter(g => !!g.connectionId)
        .map(g => ({
          connectionId: g.connectionId!,
          connectionName: g.connectionName,
          level: g.level === 'edit' ? 'edit' : 'view',
        })),
    )
    setEditingAccess(true)
  }

  function openAddDbModal() {
    setNewDbId(addableConnections[0]?.id ?? '')
    setNewDbLevel('view')
    setAddDbOpen(true)
  }

  function handleAddDbConfirm() {
    const picked = addableConnections.find(c => c.id === newDbId)
    if (!picked) return
    setDraftGrants(prev => [...prev, { connectionId: picked.id, connectionName: picked.name, level: newDbLevel }])
    setAddDbOpen(false)
  }

  function toggleCandidate(uid: string) {
    setSelectedUserIds(prev => (prev.includes(uid) ? prev.filter(id => id !== uid) : [...prev, uid]))
  }

  return (
    <section className="admin-crud-page" aria-labelledby="admin-group-detail-title">
      <header className="admin-crud-header">
        <div>
          <p className="ds-eyebrow">ADMIN / GRUP / DETAIL</p>
          {editingName ? (
            <form
              className="admin-crud-actions"
              style={{ marginTop: 8 }}
              onSubmit={e => {
                e.preventDefault()
                if (draftName.trim()) renameMutation.mutate()
              }}
            >
              <Input
                label="Nama grup"
                value={draftName}
                onChange={e => setDraftName(e.target.value)}
                required
              />
              <Button type="submit" loading={renameMutation.isPending}>
                Simpan
              </Button>
              <Button variant="secondary" onClick={() => setEditingName(false)}>
                Batal
              </Button>
            </form>
          ) : (
            <h1 id="admin-group-detail-title">{group.name}</h1>
          )}
          <div className="admin-badge-row">
            <Badge tone="neutral">{group.members.length} anggota</Badge>
            <Badge tone="info">{group.grants.length} database</Badge>
          </div>
        </div>
        <div className="admin-crud-actions">
          {!editingName && (
            <Button
              variant="secondary"
              onClick={() => {
                setDraftName(group.name)
                setEditingName(true)
              }}
            >
              Ubah Nama
            </Button>
          )}
          <Button variant="destructive" onClick={() => setConfirmDeleteOpen(true)}>
            Hapus Grup
          </Button>
          <Link className="ds-button ds-button--secondary ds-button--md" to={paths.groups}>
            Kembali
          </Link>
        </div>
      </header>

      {actionError && (
        <Alert tone="error" title="Operasi gagal">
          {actionError}
        </Alert>
      )}

      <div className="admin-detail-grid">
        {/* Kartu Anggota */}
        <Card>
          <div className="admin-section-header">
            <h2>Anggota Grup</h2>
            <Button
              size="sm"
              disabled={group.candidateUsers.length === 0}
              onClick={() => {
                setSelectedUserIds([])
                setMemberSearch('')
                setAddMembersOpen(true)
              }}
            >
              Tambah Anggota
            </Button>
          </div>

          {group.members.length === 0 ? (
            <EmptyState title="Belum ada anggota">
              Tambahkan pengguna agar mewarisi izin akses database milik grup ini.
            </EmptyState>
          ) : (
            <div className="admin-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Username</th>
                    <th>Nama</th>
                    <th>Status</th>
                    <th className="admin-cell-actions">Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {group.members.map(m => (
                    <tr key={m.id}>
                      <td>
                        <Link to={paths.userDetail(m.id)}>
                          <strong>{m.username}</strong>
                        </Link>
                      </td>
                      <td>{m.fullName || '-'}</td>
                      <td>
                        <Badge tone={m.isActive ? 'success' : 'error'}>{m.isActive ? 'Aktif' : 'Nonaktif'}</Badge>
                      </td>
                      <td className="admin-cell-actions">
                        <Button variant="destructive" size="sm" onClick={() => setMemberToRemove(m)}>
                          Keluarkan
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>

        {/* Kartu Akses Database (Group Access Editor) */}
        <Card>
          <div className="admin-section-header">
            <h2>Akses Database</h2>
            {!editingAccess ? (
              <Button variant="secondary" size="sm" onClick={startEditAccess}>
                Kelola Database
              </Button>
            ) : (
              <Button
                variant="secondary"
                size="sm"
                disabled={addableConnections.length === 0}
                onClick={openAddDbModal}
              >
                Tambah Database
              </Button>
            )}
          </div>

          {!editingAccess ? (
            group.grants.length === 0 ? (
              <EmptyState title="Belum ada akses database">
                Klik <strong>Kelola Database</strong> untuk memberikan izin <code>View Only</code> atau{' '}
                <code>View + Edit</code> ke grup ini.
              </EmptyState>
            ) : (
              <div className="admin-table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Database</th>
                      <th>Level Akses</th>
                    </tr>
                  </thead>
                  <tbody>
                    {group.grants.map(gr => (
                      <tr key={gr.connectionId ?? gr.connectionName}>
                        <td>
                          <strong>{gr.connectionName}</strong>
                        </td>
                        <td>
                          <Badge tone={gr.level === 'edit' ? 'info' : 'success'}>
                            {gr.level === 'edit' ? 'View + Edit' : 'View Only'}
                          </Badge>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )
          ) : (
            <div className="admin-form-grid">
              {draftGrants.length === 0 ? (
                <EmptyState title="Tidak ada database terpilih">
                  Klik <strong>Tambah Database</strong> atau simpan untuk mencabut seluruh akses grup ini.
                </EmptyState>
              ) : (
                <div className="admin-table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>Database</th>
                        <th>Level Akses</th>
                        <th className="admin-cell-actions">Aksi</th>
                      </tr>
                    </thead>
                    <tbody>
                      {draftGrants.map(gr => (
                        <tr key={gr.connectionId}>
                          <td>
                            <strong>{gr.connectionName}</strong>
                          </td>
                          <td>
                            <Select
                              label={`Level akses ${gr.connectionName}`}
                              value={gr.level}
                              onChange={e => {
                                const next = e.target.value === 'edit' ? 'edit' : 'view'
                                setDraftGrants(prev =>
                                  prev.map(row =>
                                    row.connectionId === gr.connectionId ? { ...row, level: next } : row,
                                  ),
                                )
                              }}
                            >
                              <option value="view">View Only</option>
                              <option value="edit">View + Edit</option>
                            </Select>
                          </td>
                          <td className="admin-cell-actions">
                            <Button variant="destructive" size="sm" onClick={() => setGrantToRevoke(gr)}>
                              Cabut
                            </Button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}

              <div className="admin-form-footer">
                <Button loading={saveAccessMutation.isPending} onClick={() => saveAccessMutation.mutate()}>
                  Simpan Perubahan
                </Button>
                <Button variant="secondary" onClick={() => setEditingAccess(false)}>
                  Batal
                </Button>
              </div>
            </div>
          )}
        </Card>
      </div>

      {/* Modal Tambah Anggota */}
      <Modal
        open={addMembersOpen}
        onClose={() => setAddMembersOpen(false)}
        title="Tambah Anggota ke Grup"
        footer={
          <>
            <Button variant="secondary" onClick={() => setAddMembersOpen(false)}>
              Batal
            </Button>
            <Button
              disabled={selectedUserIds.length === 0}
              loading={addMembersMutation.isPending}
              onClick={() => addMembersMutation.mutate()}
            >
              Tambahkan ({selectedUserIds.length})
            </Button>
          </>
        }
      >
        <div className="admin-form-grid">
          <Input
            label="Cari pengguna"
            placeholder="Ketik username atau nama..."
            value={memberSearch}
            onChange={e => setMemberSearch(e.target.value)}
          />
          {filteredCandidates.length === 0 ? (
            <p className="ds-muted">Tidak ada pengguna tersedia untuk ditambahkan.</p>
          ) : (
            <div style={{ display: 'grid', gap: 10, maxHeight: 280, overflowY: 'auto' }}>
              {filteredCandidates.map(u => (
                <Choice
                  key={u.id}
                  label={`${u.username}${u.fullName ? ` (${u.fullName})` : ''}${!u.isActive ? ' [Nonaktif]' : ''}`}
                  checked={selectedUserIds.includes(u.id)}
                  onChange={() => toggleCandidate(u.id)}
                />
              ))}
            </div>
          )}
        </div>
      </Modal>

      {/* Modal Tambah Database ke Draft Access */}
      <Modal
        open={addDbOpen}
        onClose={() => setAddDbOpen(false)}
        title="Tambah Akses Database"
        footer={
          <>
            <Button variant="secondary" onClick={() => setAddDbOpen(false)}>
              Batal
            </Button>
            <Button disabled={!newDbId} onClick={handleAddDbConfirm}>
              Tambahkan
            </Button>
          </>
        }
      >
        <div className="admin-form-grid">
          <Select label="Pilih Database" value={newDbId} onChange={e => setNewDbId(e.target.value)}>
            {addableConnections.map(c => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </Select>
          <Select
            label="Level Akses"
            value={newDbLevel}
            onChange={e => setNewDbLevel(e.target.value === 'edit' ? 'edit' : 'view')}
          >
            <option value="view">View Only</option>
            <option value="edit">View + Edit</option>
          </Select>
        </div>
      </Modal>

      {/* Konfirmasi Cabut Akses Database */}
      <ConfirmationModal
        open={!!grantToRevoke}
        onClose={() => setGrantToRevoke(null)}
        onConfirm={() => {
          if (grantToRevoke) {
            setDraftGrants(prev => prev.filter(g => g.connectionId !== grantToRevoke.connectionId))
          }
          setGrantToRevoke(null)
        }}
        title="Cabut akses database?"
        confirmLabel="Cabut Akses"
        destructive
      >
        Akses grup <strong>{group.name}</strong> ke database <strong>{grantToRevoke?.connectionName}</strong> akan
        dihapus saat Anda menyimpan perubahan.
      </ConfirmationModal>

      {/* Konfirmasi Hapus Anggota */}
      <ConfirmationModal
        open={!!memberToRemove}
        onClose={() => setMemberToRemove(null)}
        onConfirm={() => {
          if (memberToRemove) removeMemberMutation.mutate(memberToRemove.id)
        }}
        title="Keluarkan anggota dari grup?"
        confirmLabel="Keluarkan"
        destructive
      >
        Pengguna <strong>{memberToRemove?.username}</strong> akan dikeluarkan dari grup <strong>{group.name}</strong>{' '}
        dan sesinya dicabut.
      </ConfirmationModal>

      {/* Konfirmasi Hapus Grup */}
      <ConfirmationModal
        open={confirmDeleteOpen}
        onClose={() => setConfirmDeleteOpen(false)}
        onConfirm={() => deleteGroupMutation.mutate()}
        title="Hapus grup?"
        confirmLabel="Hapus Grup"
        destructive
      >
        Grup <strong>{group.name}</strong> beserta seluruh grant aksesnya akan dihapus permanen dan sesi seluruh
        anggotanya akan dicabut.
      </ConfirmationModal>

      {toast && <Toast message={toast} onDismiss={() => setToast('')} />}
    </section>
  )
}
