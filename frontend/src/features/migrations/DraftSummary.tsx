import { Badge, Card } from '../../design-system'

export default function DraftSummary({ database, version, author }: { database: string; version: string; author: string }) {
  return <Card className="draft-summary"><div className="draft-summary-mobile"><div><small>DATABASE</small><Badge tone="neutral">Draft</Badge></div><h2>Migration {version}</h2><p>{database} · Draft</p></div><div className="draft-summary-desktop"><h2>Ringkasan</h2><dl>{[{ label: 'Database', value: database }, { label: 'Versi', value: version }, { label: 'Pelaksana', value: author }, { label: 'Status', value: <Badge tone="neutral">Draft</Badge> }].map(item => <div key={item.label}><dt>{item.label}</dt><dd>{item.value}</dd></div>)}</dl></div></Card>
}
