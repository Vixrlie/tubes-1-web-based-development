import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Alert, Button, Card, Choice, ConfirmationModal, EmptyState, Input, Spinner, SqlEditor } from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { DraftValidationError, validateDraft } from './draft-source'
import type { DraftInput, MigrationDraft } from './draft-source'
import { withMigrationSearch } from './migration-source'
import { useDrafts } from './useDrafts'
import DraftSummary from './DraftSummary'
import { isEditable } from './workflow-model'
import './drafts.css'

export default function MigrationFormPage() {
  const query = useDrafts()
  const { draftId } = useParams()
  const [params] = useSearchParams()
  const { state } = useSession()
  const { database } = query.workspace
  const version = useQuery({ queryKey: [...query.queryKey, 'next-version', query.data?.length], queryFn: ({ signal }) => query.source.nextVersion(database.id, signal), enabled: !draftId && database.permission === 'edit', retry: false })
  const back = withMigrationSearch(paths.migrations(database.id), params)
  if (database.permission !== 'edit') return <Card><EmptyState title="Akses hanya untuk melihat" action={<Link to={back}>Kembali ke Migrasi</Link>}>Anda tidak memiliki izin untuk membuat atau mengubah migrasi pada database ini.</EmptyState></Card>
  if (query.isPending || (!draftId && version.isPending)) return <Card className="schema-state"><Spinner label="Memuat form migrasi" /></Card>
  if (query.isError || (!draftId && version.isError)) return <Card className="schema-state"><Alert tone="error" title="Form migrasi belum dapat dimuat">{query.error?.message ?? version.error?.message}</Alert><Button variant="secondary" onClick={() => { void query.refetch(); if (!draftId) void version.refetch() }}>Coba lagi</Button></Card>
  const draft = query.data?.find(row => row.id === draftId)
  if (draftId && !draft) return <Card><EmptyState title="Draft tidak ditemukan" action={<Link to={back}>Kembali ke Migrasi</Link>}>Pilih draft yang tersedia pada database ini.</EmptyState></Card>
  if (draft && !isEditable(draft)) return <Card><EmptyState title="SQL terkunci untuk review" action={<Link to={withMigrationSearch(paths.review(database.id, draft.id), params)}>Buka Review</Link>}>SQL migrasi yang sudah diajukan tidak dapat diubah.</EmptyState></Card>
  return <MigrationForm key={`${state.user?.id}/${database.id}/${draftId ?? 'new'}`} draft={draft} version={draft?.version ?? version.data ?? ''} author={state.user?.username ?? ''} />
}

function MigrationForm({ draft, version, author }: { draft?: MigrationDraft; version: string; author: string }) {
  const query = useDrafts()
  const { session } = useSession()
  const { database } = query.workspace
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const client = useQueryClient()
  const initial: DraftInput = draft ? { name: draft.name, sqlUp: draft.sqlUp, sqlDown: draft.sqlDown, irreversible: draft.irreversible, ...(draft.batch ? { batch: { ...draft.batch } } : {}) } : { name: '', sqlUp: '', sqlDown: '', irreversible: false }
  const [input, setInput] = useState<DraftInput>(initial)
  const [submitted, setSubmitted] = useState(false)
  const [cancelOpen, setCancelOpen] = useState(false)
  const [saved, setSaved] = useState(false)
  const controller = useRef<AbortController | null>(null)
  const form = useRef<HTMLFormElement>(null)
  const dirty = !saved && JSON.stringify(input) !== JSON.stringify(initial)
  const errors = submitted ? validateDraft(input) : {}
  const back = withMigrationSearch(draft ? paths.migrationDraft(database.id, draft.id) : paths.migrations(database.id), params)
  useEffect(() => () => controller.current?.abort(), [])
  useEffect(() => {
    if (!dirty) return
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = '' }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])
  const mutation = useMutation({ mutationFn: async (value: DraftInput) => {
    controller.current = new AbortController()
    return query.source.save(database.id, value, controller.current.signal, draft?.id)
  }, onSuccess: row => {
    if (session.getSnapshot().status !== 'authenticated' || session.getSnapshot().user?.id !== row.ownerId) return
    setSaved(true)
    client.setQueryData<MigrationDraft[]>(query.queryKey, previous => [row, ...(previous ?? []).filter(item => item.id !== row.id)])
    void client.invalidateQueries({ queryKey: ['migration-drafts'] })
    navigate(withMigrationSearch(paths.migrationDraft(database.id, row.id), params), { replace: true, state: { draftSaved: row.id } })
  } })
  function change<K extends keyof DraftInput>(field: K, value: DraftInput[K]) { setInput(previous => ({ ...previous, [field]: value })); mutation.reset() }
  function save() {
    if (mutation.isPending) return
    setSubmitted(true)
    const fields = validateDraft(input)
    if (Object.keys(fields).length) {
      requestAnimationFrame(() => { form.current?.querySelector<HTMLElement>('[aria-invalid="true"] .cm-content, [aria-invalid="true"]')?.focus() })
      return
    }
    mutation.mutate(input)
  }
  return <div className="draft-page"><header className="schema-heading"><h1>{draft ? 'Edit Draft Migrasi' : 'Buat Migrasi'}</h1><p>{database.name} › <Link to={withMigrationSearch(paths.migrations(database.id), params)}>Migrasi</Link> › {draft ? 'Draft' : 'Buat'} {version}</p></header>
    <div className="draft-workspace"><DraftSummary database={database.name} version={version} author={draft?.author ?? author} />
      <form ref={form} className="draft-editor" noValidate onSubmit={event => { event.preventDefault(); save() }} aria-busy={mutation.isPending}>
        <fieldset disabled={mutation.isPending}><legend className="sr-only">Form migrasi</legend>
          <Input label="Nama migrasi" placeholder="Deskripsi perubahan" required maxLength={120} value={input.name} error={errors.name} onChange={event => change('name', event.target.value)} />
          <h2>SQL Up · SQL Down · Reversibility</h2>
          {submitted && Object.keys(errors).length > 0 && <Alert tone="error" title="Periksa field yang belum lengkap">Lengkapi nama migrasi, SQL Up, dan SQL Down jika migrasi reversible.</Alert>}
          <SqlEditor label="SQL UP" value={input.sqlUp} onChange={value => change('sqlUp', value)} readOnly={mutation.isPending} error={errors.sqlUp} />
          <div className="draft-reversibility"><Choice label="Irreversible (tanpa SQL Down)" checked={input.irreversible} onChange={event => change('irreversible', event.target.checked)} />{input.irreversible && <Alert tone="warning" title="Migrasi irreversible">SQL Down tidak akan disimpan. Perubahan ini tidak menyediakan rollback otomatis.</Alert>}</div>
          <SqlEditor label="SQL DOWN · ROLLBACK" value={input.irreversible ? '' : input.sqlDown} onChange={value => change('sqlDown', value)} readOnly={input.irreversible || mutation.isPending} error={errors.sqlDown} />
          <div className="draft-reversibility"><Choice label="Eksekusi per batch (dataset besar)" checked={!!input.batch} onChange={event => change('batch', event.target.checked ? { table: '', column: 'id', size: 10000 } : null)} />
            {input.batch && <><Alert tone="info" title="Mode batch">Setiap batch berjalan dalam transaksi sendiri dengan checkpoint. Pakai $1 dan $2 sebagai batas rentang kursor, misalnya <code>WHERE id BETWEEN $1 AND $2</code>. Bila gagal di tengah, batch yang sudah tersimpan dapat dilanjutkan (Resume) atau dibalik (Rollback).</Alert>
              <Input label="Tabel" placeholder="public.orders" value={input.batch.table} error={errors.batch} onChange={event => change('batch', { ...input.batch!, table: event.target.value })} />
              <Input label="Kolom kursor" placeholder="id" value={input.batch.column} onChange={event => change('batch', { ...input.batch!, column: event.target.value })} />
              <Input label="Ukuran batch (baris)" type="number" min={1} max={1000000} value={String(input.batch.size)} onChange={event => change('batch', { ...input.batch!, size: Number(event.target.value) })} /></>}</div>
          {mutation.isError && <Alert tone="error" title="Draft belum tersimpan">{mutation.error instanceof DraftValidationError ? 'Periksa field migrasi.' : mutation.error.message}</Alert>}
          <div className="draft-actions"><Button variant="tertiary" onClick={() => dirty ? setCancelOpen(true) : navigate(back)}>Batal</Button><Button type="submit" variant="secondary" loading={mutation.isPending}>Simpan Draft</Button></div>
        </fieldset>
      </form>
    </div>
    <ConfirmationModal open={cancelOpen} onClose={() => setCancelOpen(false)} onConfirm={() => { setSaved(true); navigate(back) }} title="Tinggalkan perubahan?" confirmLabel="Tinggalkan">Perubahan yang belum disimpan akan hilang.</ConfirmationModal>
  </div>
}
