import { Link } from 'react-router-dom'
import { Icon } from '../../design-system'
import { useSession } from '../auth/context'
import './home.css'

export default function HomePage() {
  const { state } = useSession()
  const admin = state.user?.isPlatformAdmin
  const actions = [
    { title: 'Jelajahi database', text: 'Temukan koneksi yang bisa diakses, lalu lihat schema dan data tabel.', label: 'Buka database', to: '/databases' },
    ...(admin ? [
      { title: 'Kelola koneksi', text: 'Lihat koneksi terdaftar dan kelola akses ke server database.', label: 'Lihat koneksi', to: '/admin/connections' },
      { title: 'Administrasi akses', text: 'Atur pengguna, group, dan koneksi dalam satu dashboard.', label: 'Buka admin', to: '/admin' },
    ] : []),
  ]
  return <div className="home-page"><section className="home-hero" aria-labelledby="home-title">
    <div className="home-welcome"><span className="home-eyebrow">● &nbsp; DATABASE WORKSPACE</span><h1 id="home-title">Kelola database dengan<br />lebih percaya diri.</h1><p>Satu tempat untuk menjelajahi schema, mengelola akses, dan menjalankan migration PostgreSQL dengan alur yang jelas.</p><div className="home-cta"><Link className="ds-button ds-button--primary ds-button--md" to="/databases">Jelajahi Database</Link>{admin && <Link className="ds-button ds-button--secondary ds-button--md" to="/admin">Dashboard Admin</Link>}</div></div>
    <div className="home-illustration" aria-hidden="true"><span className="home-postgres">● &nbsp; PostgreSQL</span><div className="home-preview"><div className="home-preview-heading"><strong><i />shop_db</strong><span>Connected</span></div><small>SCHEMA EXPLORER</small><div className="home-schema"><Icon name="database" />public</div><div className="home-table home-table-selected">products</div><div className="home-table">users</div><div className="home-table">orders</div></div></div>
  </section><section aria-labelledby="quick-actions-title"><h2 id="quick-actions-title">Mulai dari sini</h2><div className="home-actions">{actions.map((action, index) => <Link key={action.to} to={action.to} className="home-action"><span className="home-action-number">0{index + 1}</span><div><h3>{action.title}</h3><p>{action.text}</p><span className="ds-button ds-button--secondary ds-button--sm">{action.label}</span></div></Link>)}</div></section></div>
}
