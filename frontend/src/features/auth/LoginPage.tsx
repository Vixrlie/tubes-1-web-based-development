import { useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { Alert, Button, Input } from '../../design-system'
import { useSession } from './context'
import './login.css'

export default function LoginPage() {
  const { state, session } = useSession()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [errors, setErrors] = useState({ username: '', password: '' })
  const form = useRef<HTMLFormElement>(null)
  async function submit(event: FormEvent) {
    event.preventDefault()
    const next = { username: username.trim() ? '' : 'Isi username Anda.', password: password ? '' : 'Isi password Anda.' }
    setErrors(next)
    if (next.username || next.password) { form.current?.querySelector<HTMLInputElement>(next.username ? '[name=username]' : '[name=password]')?.focus(); return }
    try { await session.login(username.trim(), password) } catch { setPassword(''); form.current?.querySelector<HTMLInputElement>('[name=password]')?.focus() }
  }
  return <div className="login-page">
    <aside className="login-brand"><div className="login-brand-panel"><img src="/design-system/c0d63.png" alt="Pathgres" width="191" height="44" /><div><h2>Data Anda, Kendali Anda,<br />Akses Anda</h2><p>Satu tempat untuk melihat schema PostgreSQL, menjalankan migration dalam transaction, dan menjaga riwayat perubahan tetap rapi.</p></div></div></aside>
    <section className="login-form-panel" aria-labelledby="login-title"><form ref={form} onSubmit={submit} noValidate aria-busy={state.busy}>
      <img className="login-mobile-logo" src="/design-system/c0d63.png" alt="Pathgres" width="191" height="44" />
      <header><h1 id="login-title">Masuk</h1><p>Masuk untuk mengakses akun Pathgres Anda.</p></header>
      {state.message && <Alert tone="error" title={state.message} />}
      <div className="login-fields"><Input label="Username" name="username" placeholder="Username Anda" autoComplete="username" autoCapitalize="none" spellCheck={false} value={username} onChange={event => setUsername(event.target.value)} disabled={state.busy} error={errors.username} /><Input label="Password" name="password" type="password" placeholder="Password Anda" autoComplete="current-password" value={password} onChange={event => setPassword(event.target.value)} disabled={state.busy} error={errors.password} /></div>
      <Button type="submit" size="lg" loading={state.busy}>Masuk</Button>
    </form></section>
  </div>
}
