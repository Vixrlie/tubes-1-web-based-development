import { useEffect, useId, useRef } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Button, IconButton } from './primitives'

export function Modal({ open, onClose, title, children, footer, drawer = false }: { open: boolean; onClose: () => void; title: string; children: ReactNode; footer?: ReactNode; drawer?: boolean }) {
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
  return createPortal(<dialog ref={ref} className={`ds-modal ${drawer ? 'ds-modal--drawer' : ''}`} aria-labelledby={headingId} onCancel={(event) => { event.preventDefault(); onClose() }} onClick={event => { if (event.target === event.currentTarget) { const rect = event.currentTarget.getBoundingClientRect(); if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) onClose() } }}><div className="ds-modal-heading"><h2 id={headingId}>{title}</h2><IconButton label="Tutup" onClick={onClose}>×</IconButton></div><div className="ds-modal-body">{children}</div>{footer && <div className="ds-modal-footer">{footer}</div>}</dialog>, document.body)
}

export function ConfirmationModal({ open, onClose, onConfirm, title, children, destructive = false, confirmLabel = 'Konfirmasi' }: { open: boolean; onClose: () => void; onConfirm: () => void; title: string; children: ReactNode; destructive?: boolean; confirmLabel?: string }) {
  return <Modal open={open} onClose={onClose} title={title} footer={<><Button variant="secondary" onClick={onClose}>Batal</Button><Button variant={destructive ? 'destructive' : 'primary'} onClick={onConfirm}>{confirmLabel}</Button></>}>{children}</Modal>
}

export function ProfilePopup({ name, email, onLogout, loading = false }: { name: string; email: string; onLogout: () => void; loading?: boolean }) {
  const ref = useRef<HTMLDetailsElement>(null)
  useEffect(() => {
    function close(event: PointerEvent) { if (!ref.current?.contains(event.target as Node)) ref.current?.removeAttribute('open') }
    document.addEventListener('pointerdown', close)
    return () => document.removeEventListener('pointerdown', close)
  }, [])
  return <details className="ds-profile" ref={ref} onKeyDown={event => { if (event.key === 'Escape') { ref.current?.removeAttribute('open'); ref.current?.querySelector('summary')?.focus() } }}><summary aria-label={`Profil ${name}`}><span className="ds-avatar">{name.split(' ').map(part => part[0]).slice(0, 2).join('')}</span></summary><div className="ds-profile-popup"><strong>{name}</strong><small>{email}</small><hr /><Button variant="tertiary" loading={loading} onClick={() => { ref.current?.removeAttribute('open'); onLogout() }}>Keluar</Button></div></details>
}

export function Toast({ message, onDismiss }: { message: string; onDismiss: () => void }) {
  useEffect(() => { const timer = window.setTimeout(onDismiss, 5000); return () => window.clearTimeout(timer) }, [message, onDismiss])
  return <div className="ds-toast" role="status"><span>{message}</span><IconButton label="Tutup notifikasi" onClick={onDismiss}>×</IconButton></div>
}
