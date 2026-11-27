import { Link } from 'react-router-dom'
import { Alert, Badge, Card } from '../../design-system'
import { paths } from '../../app/routes'
import { observabilityLinks } from './monitoring-source'
import './monitoring.css'

export default function ObservabilityPage() {
  const links = observabilityLinks(import.meta.env.VITE_GRAFANA_URL || (import.meta.env.DEV ? 'http://localhost:3000' : ''))
  return <section className="monitoring-page" aria-labelledby="observability-title">
    <header className="monitoring-heading"><div><p className="ds-eyebrow">ADMIN / MONITORING</p><h1 id="observability-title">Observability</h1><p>Pantau metrik, log, dan trace Pathgres melalui Grafana.</p></div><Badge tone="neutral">Grafana</Badge></header>
    {!links && <Alert tone="warning" title="Alamat Grafana belum tersedia">Hubungi administrator untuk mengatur akses monitoring.</Alert>}
    <div className="monitoring-cards">
      <Card className="monitoring-tool"><span className="monitoring-number">01</span><h2>Metrics</h2><p>Migration berjalan, selesai dan gagal; durasi eksekusi dan lock; serta penggunaan koneksi database.</p><small>Dashboard: Pathgres Observability · Prometheus</small>{links && <a className="ds-button ds-button--primary ds-button--md" href={links.dashboard} target="_blank" rel="noopener noreferrer">Buka dashboard<span className="sr-only"> di tab baru</span></a>}</Card>
      <Card className="monitoring-tool"><span className="monitoring-number">02</span><h2>Logs</h2><p>Telusuri log HTTP dan SQL. Gunakan request ID atau migration ID untuk menghubungkan operasi yang sama.</p><small>Grafana Explore · Loki</small>{links && <a className="ds-button ds-button--secondary ds-button--md" href={links.explore} target="_blank" rel="noopener noreferrer">Buka log di Grafana<span className="sr-only"> di tab baru</span></a>}</Card>
      <Card className="monitoring-tool"><span className="monitoring-number">03</span><h2>Traces</h2><p>Ikuti durasi request, eksekusi migration, lock, dan SQL menggunakan trace ID dari log.</p><small>Grafana Explore · Tempo</small>{links && <a className="ds-button ds-button--secondary ds-button--md" href={links.explore} target="_blank" rel="noopener noreferrer">Buka trace di Grafana<span className="sr-only"> di tab baru</span></a>}</Card>
    </div>
    <Card className="monitoring-panel monitoring-stack"><h2>Telusuri operasi</h2><ol className="monitoring-steps"><li>Buka Grafana Explore, pilih datasource <strong>Loki</strong>.</li><li>Gunakan query <code>{'{service_name="pathgres"} | json | action="sql.result"'}</code>, lalu cari request ID atau migration ID.</li><li>Salin <code>trace_id</code> dari log jika tersedia. Pilih datasource <strong>Tempo</strong>, lalu cari trace ID tersebut.</li></ol><p className="ds-muted">Grafana memakai sesi login tersendiri. Metrik dihitung sejak backend mulai; status layanan diperiksa di Grafana.</p><Link className="ds-button ds-button--tertiary ds-button--md" to={paths.activity}>Buka Activity Logs</Link></Card>
  </section>
}
