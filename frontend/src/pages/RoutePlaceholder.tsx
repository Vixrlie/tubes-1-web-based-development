import { Link } from 'react-router-dom'
import { paths } from '../app/routes'

export default function RoutePlaceholder({ title }: { title: string }) {
  return <section className="app-status-page"><h1>{title}</h1><p>Halaman ini sedang disiapkan.</p><Link to={paths.catalog}>Buka katalog komponen</Link></section>
}
