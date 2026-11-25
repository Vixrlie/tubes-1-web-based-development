import { useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { Alert, Button, Card, Choice, Input } from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { createAdminApi } from './admin-api'
import './admin-crud.css'

export default function AdminUserCreatePage() {
  const { session } = useSession()
  const api = useMemo(() => createAdminApi(session.api), [session])
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const [username, setUsername] = useState('')
  const [fullName, setFullName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isPlatformAdmin, setIsPlatformAdmin] = useState(false)
  const [formError, setFormError] = useState('')

  const createMutation = useMutation({
    mutationFn: () =>
      api.createUser({
        username: username.trim(),
        fullName: fullName.trim(),
        email: email.trim(),
        password,
        isPlatformAdmin,
      }),
    onSuccess: created => {
      void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] })
      void queryClient.invalidateQueries({ queryKey: ['admin', 'dashboard'] })
      navigate(paths.userDetail(created.id), { replace: true })
    },
    onError: err => {
      setFormError(err instanceof Error ? err.message : 'Gagal membuat pengguna.')
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setFormError('')
    if (username.trim().length < 3) {
      setFormError('Username minimal 3 karakter (huruf kecil, angka, titik, atau garis bawah).')
      return
    }
    if (password.length < 8) {
      setFormError('Password minimal 8 karakter.')
      return
    }
    createMutation.mutate()
  }

  return (
    <section className="admin-crud-page" aria-labelledby="admin-user-create-title">
      <header className="admin-crud-header">
        <div>
          <p className="ds-eyebrow">ADMIN / PENGGUNA / BARU</p>
          <h1 id="admin-user-create-title">Tambah Pengguna</h1>
          <p>Buat akun baru untuk mengakses Pathgres. Akses database diatur melalui keanggotaan grup.</p>
        </div>
        <div className="admin-crud-actions">
          <Link className="ds-button ds-button--secondary ds-button--md" to={paths.users}>
            Kembali
          </Link>
        </div>
      </header>

      {formError && (
        <Alert tone="error" title="Tidak dapat menyimpan pengguna">
          {formError}
        </Alert>
      )}

      <Card>
        <form className="admin-form-grid" onSubmit={handleSubmit}>
          <Input
            label="Username"
            required
            placeholder="misal: budi.santoso"
            helper="3–32 karakter: huruf kecil, angka, titik (.), atau garis bawah (_)."
            value={username}
            onChange={e => setUsername(e.target.value)}
            autoComplete="off"
          />

          <Input
            label="Nama lengkap"
            placeholder="misal: Budi Santoso"
            value={fullName}
            onChange={e => setFullName(e.target.value)}
          />

          <Input
            label="Email"
            type="email"
            placeholder="budi@contoh.id"
            value={email}
            onChange={e => setEmail(e.target.value)}
          />

          <Input
            label="Password"
            type="password"
            required
            helper="Minimal 8 karakter dan maksimal 72 byte."
            value={password}
            onChange={e => setPassword(e.target.value)}
            autoComplete="new-password"
          />

          <Choice
            label="Jadikan Platform Admin (mengelola user, grup, koneksi, dan monitoring)"
            checked={isPlatformAdmin}
            onChange={e => setIsPlatformAdmin(e.target.checked)}
          />

          <div className="admin-form-footer">
            <Button type="submit" loading={createMutation.isPending}>
              Simpan Pengguna
            </Button>
            <Link className="ds-button ds-button--secondary ds-button--md" to={paths.users}>
              Batal
            </Link>
          </div>
        </form>
      </Card>
    </section>
  )
}
