import { useEffect, useId, useRef } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Button, IconButton } from './primitives'

export function Modal({ open, onClose, title, children, footer, drawer = false, context = false }: { open: boolean; onClose: () => void; title: ReactNode; children: ReactNode; footer?: ReactNode; drawer?: boolean; context?: boolean }) {
  const ref = useRef<HTMLDialogElement>(null); const headingId = useId()
  useEffect(() => {
    const dialog = ref.current
    if (!open || !dialog) return
    const previous = document.activeElement as HTMLElement | null
    dialog.showModal()
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { dialog.close(); document.body.style.overflow = overflow; previous?.focus() }
  }, [open])
  if (!open) return null
  return createPortal(<dialog ref={ref} className={`ds-modal ${drawer ? 'ds-modal--drawer' : ''} ${context ? 'ds-modal--context' : ''}`} aria-labelledby={headingId} onCancel={(event) => { event.preventDefault(); onClose() }} onClick={event => { if (event.target === event.currentTarget) { const rect = event.currentTarget.getBoundingClientRect(); if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) onClose() } }}><div className="ds-modal-heading"><h2 id={headingId}>{title}</h2><IconButton label="Tutup" onClick={onClose}>×</IconButton></div><div className="ds-modal-body">{children}</div>{footer && <div className="ds-modal-footer">{footer}</div>}</dialog>, document.body)
}

/** Figma mobile context navigation below the app bar. */
export function MobileContextMenu({ title, description, ...props }: { open: boolean; onClose: () => void; title: string; description: string; children: ReactNode }) {
  return <Modal {...props} context title={<><span>{title}</span><small className="ds-context-description">{description}</small></>} />
}

export function ConfirmationModal({ open, onClose, onConfirm, title, children, destructive = false, confirmLabel = 'Konfirmasi' }: { open: boolean; onClose: () => void; onConfirm: () => void; title: string; children: ReactNode; destructive?: boolean; confirmLabel?: string }) {
  return <Modal open={open} onClose={onClose} title={title} footer={<><Button variant="secondary" onClick={onClose}>Batal</Button><Button variant={destructive ? 'destructive' : 'primary'} onClick={onConfirm}>{confirmLabel}</Button></>}>{children}</Modal>
}

export function ProfilePopup({ name, email, role = 'User', onLogout, loading = false }: { name: string; email?: string; role?: string; onLogout: () => void; loading?: boolean }) {
  const ref = useRef<HTMLDetailsElement>(null)
  useEffect(() => {
    function close(event: PointerEvent) { if (!ref.current?.contains(event.target as Node)) ref.current?.removeAttribute('open') }
    document.addEventListener('pointerdown', close)
    return () => document.removeEventListener('pointerdown', close)
  }, [])
  const initial = name.trim().slice(0, 1).toUpperCase()
  return <details className="ds-profile" ref={ref} onKeyDown={event => { if (event.key === 'Escape') { ref.current?.removeAttribute('open'); ref.current?.querySelector('summary')?.focus() } }}><summary aria-label={`Profil ${name}`}><span className="ds-avatar">{initial}</span></summary><div className="ds-profile-popup"><div className="ds-profile-identity"><span className="ds-profile-avatar" aria-hidden="true">{initial}</span><div className="ds-profile-info"><strong>{name}</strong>{email?.trim() && <small>{email}</small>}<span className="ds-profile-role">{role}</span></div></div><hr /><div className="ds-profile-logout-wrap"><Button variant="tertiary" className="ds-profile-logout" loading={loading} onClick={() => { ref.current?.removeAttribute('open'); onLogout() }}>{!loading && <img src="/design-system/profile-logout.svg" width="20" height="20" alt="" />}Logout</Button></div></div></details>
}

export function Toast({ message, onDismiss }: { message: string; onDismiss: () => void }) {
  useEffect(() => { const timer = window.setTimeout(onDismiss, 5000); return () => window.clearTimeout(timer) }, [message, onDismiss])
  return <div className="ds-toast" role="status"><span>{message}</span><IconButton label="Tutup notifikasi" onClick={onDismiss}>×</IconButton></div>
}
