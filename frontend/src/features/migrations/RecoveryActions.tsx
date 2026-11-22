import { useEffect, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Alert, Button, Modal, SqlPreview } from '../../design-system'
import { paths } from '../../app/routes'
import { useSession } from '../auth/context'
import { withMigrationSearch } from './migration-source'
import { findExecution, operationEligibility, operationLabels } from './operation-model'
import type { OperationKind } from './operation-model'
import { useDrafts } from './useDrafts'

export default function RecoveryActions({ executionId }: { executionId: string }) {
  const query = useDrafts()
  const { session } = useSession()
  const navigate = useNavigate()
  const client = useQueryClient()
  const [params] = useSearchParams()
  const [kind, setKind] = useState<OperationKind | null>(null)
  const abort = useRef<AbortController | null>(null)
  useEffect(() => () => abort.current?.abort(), [])
  const target = findExecution(query.data ?? [], executionId)
  const mutation = useMutation({ mutationFn: async (action: OperationKind) => { abort.current = new AbortController(); return query.source.requestOperation(query.workspace.database.id, executionId, action, abort.current.signal) }, onSuccess: row => { if (session.getSnapshot().status !== 'authenticated' || session.getSnapshot().user?.id !== row.ownerId) return; void client.invalidateQueries({ queryKey: ['migration-drafts'] }); navigate(withMigrationSearch(paths.review(row.connectionId, row.id), params)) } })
  if (!target || query.workspace.database.permission !== 'edit') return null
  const possible: OperationKind[] = target.job.status === 'failed' && target.job.completedBatches > 0 ? ['resume', 'compensation'] : target.row.intent?.kind === 'rollback' || target.row.intent?.kind === 'compensation' ? ['forward'] : ['rollback']
  const actions = possible.map(action => ({ kind: action, ...operationEligibility(query.data ?? [], executionId, action) }))
  return <div className="workflow-panel"><div className="workflow-actions">{actions.map(action => <Button key={action.kind} variant="secondary" disabled={!action.allowed} title={action.reason || undefined} onClick={() => { mutation.reset(); setKind(action.kind) }}>Ajukan {operationLabels[action.kind]}</Button>)}</div>
    <Modal open={!!kind} title={`Ajukan ${kind ? operationLabels[kind] : 'pemulihan'}?`} onClose={() => { if (!mutation.isPending) setKind(null) }} footer={<><Button variant="secondary" disabled={mutation.isPending} onClick={() => setKind(null)}>Batal</Button><Button loading={mutation.isPending} onClick={() => { if (kind && !mutation.isPending) mutation.mutate(kind) }}>Ajukan Persetujuan</Button></>}><div className="schema-state"><p>Semua grup wajib menyetujui ulang sebelum tindakan ini dapat dijadwalkan.</p>{kind === 'resume' && <p>Lanjut dari batch #{target.job.completedBatches + 1}; {target.job.completedBatches} batch yang selesai tidak diulang.</p>}{kind === 'compensation' && <p>Kembalikan {target.job.completedBatches} batch dalam urutan terbalik memakai SQL Down asli.</p>}<SqlPreview label={kind === 'rollback' || kind === 'compensation' ? 'SQL DOWN ASLI' : 'SQL UP ASLI'} value={kind === 'resume' ? target.row.sqlUp : target.row.sqlDown} />{mutation.isError && <Alert tone="error" title="Permintaan belum dapat diajukan">{mutation.error.message}</Alert>}</div></Modal>
  </div>
}
