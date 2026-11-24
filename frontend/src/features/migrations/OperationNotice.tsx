import { Alert } from '../../design-system'
import type { OperationIntent } from './operation-model'
import { operationLabels } from './operation-model'

export default function OperationNotice({ intent }: { intent?: OperationIntent }) {
  if (!intent) return null
  return <Alert title={`${operationLabels[intent.kind]} · Persetujuan ulang`}>{intent.kind === 'resume' ? `Lanjutkan dari batch #${intent.checkpoint + 1} sampai #${intent.totalBatches}.` : intent.kind === 'compensation' ? `Kembalikan batch #${intent.checkpoint} sampai #001 dengan SQL Down asli.` : intent.kind === 'rollback' ? 'Batalkan eksekusi reversible aktif terakhir dengan SQL Down asli.' : 'Terapkan kembali SQL Up asli setelah rollback selesai.'} SQL tindakan ini dikunci sesuai snapshot eksekusi.</Alert>
}
