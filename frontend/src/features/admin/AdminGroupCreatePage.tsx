import { useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { Alert, Button, Card, Input } from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { createAdminApi } from './admin-api'
import './admin-crud.css'

export default function AdminGroupCreatePage() {
  const { session } = useSession()
  const api = useMemo(() => createAdminApi(session.api), [session])
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const [name, setName] = useState('')
  const [formError, setFormError] = useState('')

  const createMutation = useMutation({
    mutationFn: () => api.createGroup(name.trim()),
    onSuccess: created => {
      void queryClient.invalidateQueries({ queryKey: ['admin', 'groups'] })
      void queryClient.invalidateQueries({ queryKey: ['admin', 'dashboard'] })
      navigate(paths.groupDetail(created.id), { replace: true })
    },
    onError: err => {
      setFormError(err instanceof Error ? err.message : 'Gagal membuat grup.')
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setFormError('')
    if (!name.trim()) {
      setFormError('Nama grup wajib diisi.')
      return
    }
    createMutation.mutate()
  }

  return (
    <section className="admin-crud-page" aria-labelledby="admin-group-create-title">
      <header className="admin-crud-header">
        <div>
          <p className="ds-eyebrow">ADMIN / GRUP / BARU</p>
          <h1 id="admin-group-create-title">Grup Baru</h1>
          <p>Buat grup baru untuk mengelompokkan pengguna dan mengatur akses database bersama.</p>
        </div>
        <div className="admin-crud-actions">
          <Link className="ds-button ds-button--secondary ds-button--md" to={paths.groups}>
            Kembali
          </Link>
        </div>
      </header>

      {formError && (
        <Alert tone="error" title="Tidak dapat menyimpan grup">
          {formError}
        </Alert>
      )}

      <Card>
        <form className="admin-form-grid" onSubmit={handleSubmit}>
          <Input
            label="Nama grup"
            required
            placeholder="misal: tim-backend"
            value={name}
            onChange={e => setName(e.target.value)}
            autoComplete="off"
          />

          <div className="admin-form-footer">
            <Button type="submit" loading={createMutation.isPending}>
              Simpan Grup
            </Button>
            <Link className="ds-button ds-button--secondary ds-button--md" to={paths.groups}>
              Batal
            </Link>
          </div>
        </form>
      </Card>
    </section>
  )
}
