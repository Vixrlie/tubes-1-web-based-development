import { Badge, Card } from '../../design-system'
import { workflowStates } from './workflow-model'
import type { WorkflowStatus } from './workflow-model'

export default function DraftSummary({ database, version, author, status = 'draft' }: { database: string; version: string; author: string; status?: WorkflowStatus }) {
  const state = workflowStates[status]
  return <Card className="draft-summary"><div className="draft-summary-mobile"><div><small>DATABASE</small><Badge tone={state.tone}>{state.label}</Badge></div><h2>Migration {version}</h2><p>{database} · {state.label}</p></div><div className="draft-summary-desktop"><h2>Ringkasan</h2><dl>{[{ label: 'Database', value: database }, { label: 'Versi', value: version }, { label: 'Pelaksana', value: author }, { label: 'Status', value: <Badge tone={state.tone}>{state.label}</Badge> }].map(item => <div key={item.label}><dt>{item.label}</dt><dd>{item.value}</dd></div>)}</dl></div></Card>
}
