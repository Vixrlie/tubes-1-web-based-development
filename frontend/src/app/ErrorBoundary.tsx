import { Component } from 'react'
import type { ReactNode } from 'react'
import { Button } from '../design-system'

export class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() { return { failed: true } }
  render() {
    if (this.state.failed) return <main className="app-status-page"><h1>Halaman tidak dapat ditampilkan</h1><p>Muat ulang halaman untuk mencoba kembali.</p><Button onClick={() => window.location.reload()}>Muat ulang</Button></main>
    return this.props.children
  }
}
