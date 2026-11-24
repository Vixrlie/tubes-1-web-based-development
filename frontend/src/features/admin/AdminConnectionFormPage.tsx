import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Alert, Button, Card, Input, Select, Spinner } from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { createAdminApi } from './admin-api'
import type { ConnectionFormPayload, ConnectionTestResult } from './admin-types'
import './admin-crud.css'

type FormDraft = {
  dbms: 'postgres' | 'mysql'
  name: string
  description: string
  host: string
  port: string
  database: string
  username: string
  password: string
}

const emptyDraft: FormDraft = {
  dbms: 'postgres',
  name: '',
  description: '',
  host: '127.0.0.1',
  port: '5432',
  database: '',
  username: '',
  password: '',
}

export default function AdminConnectionFormPage() {
  const { connectionId } = useParams()
  const isEdit = !!connectionId
  const { state, session } = useSession()
  const api = useMemo(() => createAdminApi(session.api), [session])
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const detailQuery = useQuery({
    queryKey: ['admin', 'connection', state.user?.id, connectionId],
    queryFn: ({ signal }) => api.getConnection(connectionId!, signal),
    enabled: isEdit,
    retry: false,
  })

  const [draft, setDraft] = useState<FormDraft | null>(null)
  const [formError, setFormError] = useState('')
  const [testResult, setTestResult] = useState<ConnectionTestResult | null>(null)

  const baseValues: FormDraft = detailQuery.data
    ? {
        dbms: detailQuery.data.dbms === 'mysql' ? 'mysql' : 'postgres',
        name: detailQuery.data.name,
        description: detailQuery.data.description,
        host: detailQuery.data.host,
        port: detailQuery.data.port,
        database: detailQuery.data.database,
        username: detailQuery.data.username,
        password: '',
      }
    : emptyDraft

  const form = draft ?? baseValues

  function updateForm(patch: Partial<FormDraft>) {
    setDraft(prev => {
      const current = prev ?? baseValues
      const next = { ...current, ...patch }
      if (patch.dbms && patch.dbms !== current.dbms) {
        if (patch.dbms === 'mysql' && current.port === '5432') next.port = '3306'
        if (patch.dbms === 'postgres' && current.port === '3306') next.port = '5432'
      }
      return next
    })
  }

  function buildPayload(): ConnectionFormPayload {
    return {
      id: isEdit ? connectionId : undefined,
      dbms: form.dbms,
      name: form.name.trim(),
      description: form.description.trim(),
      host: form.host.trim(),
      port: form.port.trim(),
      database: form.database.trim(),
      username: form.username.trim(),
      password: form.password,
    }
  }

  const saveMutation = useMutation({
    mutationFn: () => (isEdit ? api.updateConnection(connectionId!, buildPayload()) : api.createConnection(buildPayload())),
    onSuccess: saved => {
      void queryClient.invalidateQueries({ queryKey: ['admin', 'connections'] })
      void queryClient.invalidateQueries({ queryKey: ['admin', 'connection', state.user?.id, saved.id] })
      void queryClient.invalidateQueries({ queryKey: ['admin', 'dashboard'] })
      navigate(paths.connectionDetail(saved.id), { replace: true })
    },
    onError: err => {
      setFormError(err instanceof Error ? err.message : 'Gagal menyimpan koneksi.')
    },
  })

  const testMutation = useMutation({
    mutationFn: () => api.testConnectionInput(buildPayload()),
    onSuccess: res => {
      setFormError('')
      setTestResult(res)
    },
    onError: err => {
      setTestResult(null)
      setFormError(err instanceof Error ? err.message : 'Gagal menguji koneksi.')
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setFormError('')
    if (!form.name.trim() || !form.host.trim() || !form.database.trim() || !form.username.trim()) {
      setFormError('Nama tampilan, host, database, dan username wajib diisi.')
      return
    }
    if (!isEdit && !form.password) {
      setFormError('Password database wajib diisi untuk koneksi baru.')
      return
    }
    saveMutation.mutate()
  }

  if (isEdit && detailQuery.isPending) {
    return (
      <section className="admin-crud-page">
        <div className="admin-status-center">
          <Spinner label="Memuat form koneksi" />
        </div>
      </section>
    )
  }

  if (isEdit && detailQuery.isError) {
    return (
      <section className="admin-crud-page">
        <Alert tone="error" title="Koneksi tidak ditemukan">
          {detailQuery.error instanceof Error ? detailQuery.error.message : 'Koneksi tidak dapat dimuat.'}
        </Alert>
        <div>
          <Link className="ds-button ds-button--secondary ds-button--md" to={paths.connections}>
            Kembali ke Daftar Koneksi
          </Link>
        </div>
      </section>
    )
  }

  const cancelPath = isEdit ? paths.connectionDetail(connectionId!) : paths.connections

  return (
    <section className="admin-crud-page" aria-labelledby="admin-conn-form-title">
      <header className="admin-crud-header">
        <div>
          <p className="ds-eyebrow">ADMIN / KONEKSI DATABASE / {isEdit ? 'UBAH' : 'BARU'}</p>
          <h1 id="admin-conn-form-title">{isEdit ? `Ubah Koneksi: ${baseValues.name}` : 'Tambah Koneksi Database'}</h1>
          <p>
            {isEdit
              ? 'Perbarui parameter koneksi. Kosongkan password untuk mempertahankan kredensial tersimpan.'
              : 'Daftarkan koneksi database target baru dan uji konektivitas sebelum menyimpan.'}
          </p>
        </div>
        <div className="admin-crud-actions">
          <Link className="ds-button ds-button--secondary ds-button--md" to={cancelPath}>
            Kembali
          </Link>
        </div>
      </header>

      {formError && (
        <Alert tone="error" title="Validasi gagal">
          {formError}
        </Alert>
      )}

      {testResult && (
        <Alert
          tone={testResult.ok ? 'success' : 'error'}
          title={testResult.ok ? `Tes koneksi berhasil (${testResult.latencyMs} ms)` : 'Tes koneksi gagal'}
        >
          {testResult.message}
        </Alert>
      )}

      <Card>
        <form className="admin-form-grid" onSubmit={handleSubmit}>
          <Select
            label="Jenis DBMS"
            value={form.dbms}
            onChange={e => updateForm({ dbms: e.target.value === 'mysql' ? 'mysql' : 'postgres' })}
          >
            <option value="postgres">PostgreSQL</option>
            <option value="mysql">MySQL / MariaDB</option>
          </Select>

          <Input
            label="Nama tampilan"
            required
            placeholder="misal: target-demo"
            value={form.name}
            onChange={e => updateForm({ name: e.target.value })}
          />

          <Input
            label="Deskripsi (opsional)"
            placeholder="Keterangan singkat tujuan database"
            value={form.description}
            onChange={e => updateForm({ description: e.target.value })}
          />

          <Input
            label="Host"
            required
            placeholder="127.0.0.1"
            value={form.host}
            onChange={e => updateForm({ host: e.target.value })}
          />

          <Input
            label="Port"
            type="number"
            required
            value={form.port}
            onChange={e => updateForm({ port: e.target.value })}
          />

          <Input
            label="Nama database"
            required
            placeholder="sample"
            value={form.database}
            onChange={e => updateForm({ database: e.target.value })}
          />

          <Input
            label="Username"
            required
            placeholder="target"
            value={form.username}
            onChange={e => updateForm({ username: e.target.value })}
          />

          <Input
            label="Password"
            type="password"
            required={!isEdit}
            helper={
              isEdit
                ? 'Kosongkan jika tetap memakai password tersimpan.'
                : 'Wajib diisi; tidak akan ditampilkan ulang setelah disimpan.'
            }
            value={form.password}
            onChange={e => updateForm({ password: e.target.value })}
            autoComplete="new-password"
          />

          <div className="admin-form-footer">
            <Button type="submit" loading={saveMutation.isPending}>
              {isEdit ? 'Simpan Perubahan' : 'Simpan Koneksi'}
            </Button>
            <Button variant="secondary" loading={testMutation.isPending} onClick={() => testMutation.mutate()}>
              Tes Koneksi
            </Button>
            <Link className="ds-button ds-button--tertiary ds-button--md" to={cancelPath}>
              Batal
            </Link>
          </div>
        </form>
      </Card>
    </section>
  )
}
