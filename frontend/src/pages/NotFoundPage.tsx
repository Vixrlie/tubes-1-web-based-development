import { Link } from 'react-router-dom'
import { paths } from '../app/routes'

export default function NotFoundPage() {
  return <main className="app-status-page"><h1>Halaman tidak ditemukan</h1><p>Periksa kembali alamat halaman yang dituju.</p><Link to={paths.catalog}>Kembali ke katalog</Link></main>
}
