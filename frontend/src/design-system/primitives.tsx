import { useId, useState } from 'react'
import type { ButtonHTMLAttributes, HTMLAttributes, InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from 'react'

export type Tone = 'info' | 'success' | 'warning' | 'error' | 'neutral'
export type ButtonVariant = 'primary' | 'secondary' | 'tertiary' | 'destructive'

export function Button({ variant = 'primary', size = 'md', loading = false, children, className = '', disabled, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant; size?: 'sm' | 'md' | 'lg'; loading?: boolean }) {
  return <button type="button" {...props} className={`ds-button ds-button--${variant} ds-button--${size} ${className}`} disabled={disabled || loading} aria-busy={loading || undefined}>{loading && <Spinner />}{children}</button>
}

export function IconButton({ label, children, ...props }: Omit<Parameters<typeof Button>[0], 'children'> & { label: string; children: ReactNode }) {
  return <Button variant="tertiary" {...props} className={`ds-icon-button ${props.className ?? ''}`} aria-label={label} title={label}>{children}</Button>
}

export function Spinner({ label = 'Memuat' }: { label?: string }) {
  return <span className="ds-spinner" role="status"><span className="sr-only">{label}</span></span>
}

export function Badge({ tone = 'info', children, ...props }: HTMLAttributes<HTMLSpanElement> & { tone?: Tone }) {
  return <span {...props} className={`ds-badge ds-tone--${tone} ${props.className ?? ''}`}>{children}</span>
}

type FieldProps = { label: string; helper?: string; error?: string }
export function Input({ label, helper, error, id, type = 'text', ...props }: InputHTMLAttributes<HTMLInputElement> & FieldProps) {
  const generatedId = useId()
  const fieldId = id ?? generatedId
  const [visible, setVisible] = useState(false)
  return <div className={`ds-field ${error ? 'ds-field--error' : ''}`}>
    <label htmlFor={fieldId}>{label}{props.required && <span aria-hidden="true"> *</span>}</label>
    <div className="ds-input-wrap"><input {...props} id={fieldId} type={type === 'password' && visible ? 'text' : type} aria-invalid={error ? true : undefined} aria-describedby={error || helper ? `${fieldId}-help` : undefined} />
      {type === 'password' && <IconButton label={visible ? 'Sembunyikan password' : 'Tampilkan password'} disabled={props.disabled} aria-pressed={visible} onClick={() => setVisible(!visible)}>{visible ? 'Sembunyi' : 'Lihat'}</IconButton>}
    </div>
    {(error || helper) && <small id={`${fieldId}-help`}>{error ?? helper}</small>}
  </div>
}

export function Select({ label, helper, error, id, children, ...props }: SelectHTMLAttributes<HTMLSelectElement> & FieldProps) {
  const generatedId = useId(); const fieldId = id ?? generatedId
  return <div className={`ds-field ${error ? 'ds-field--error' : ''}`}><label htmlFor={fieldId}>{label}</label><select {...props} id={fieldId} aria-invalid={error ? true : undefined} aria-describedby={error || helper ? `${fieldId}-help` : undefined}>{children}</select>{(error || helper) && <small id={`${fieldId}-help`}>{error ?? helper}</small>}</div>
}

export function Textarea({ label, helper, error, id, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement> & FieldProps) {
  const generatedId = useId(); const fieldId = id ?? generatedId
  return <div className={`ds-field ${error ? 'ds-field--error' : ''}`}><label htmlFor={fieldId}>{label}</label><textarea rows={3} {...props} id={fieldId} aria-invalid={error ? true : undefined} aria-describedby={error || helper ? `${fieldId}-help` : undefined} />{(error || helper) && <small id={`${fieldId}-help`}>{error ?? helper}</small>}</div>
}

export function Choice({ label, type = 'checkbox', ...props }: Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> & { label: string; type?: 'checkbox' | 'radio' }) {
  return <label className="ds-choice"><input {...props} type={type} /><span>{label}</span></label>
}

export function Alert({ tone = 'info', title, children }: { tone?: Tone; title: string; children?: ReactNode }) {
  return <div className={`ds-alert ds-tone--${tone}`} role={tone === 'error' ? 'alert' : 'status'}><strong>{title}</strong>{children && <p>{children}</p>}</div>
}

export function Card({ children, className = '', ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div {...props} className={`ds-card ${className}`}>{children}</div>
}

export function Tabs({ items, value, onChange, label }: { items: { value: string; label: string; disabled?: boolean }[]; value: string; onChange: (value: string) => void; label: string }) {
  return <div className="ds-tabs" role="tablist" aria-label={label}>{items.map((item) => <button type="button" key={item.value} role="tab" aria-selected={item.value === value} tabIndex={item.value === value ? 0 : -1} disabled={item.disabled} onClick={() => onChange(item.value)} onKeyDown={(event) => {
    const enabled = items.filter(i => !i.disabled); const index = enabled.findIndex(i => i.value === value)
    let next: number
    if (event.key === 'ArrowRight') next = (index + 1) % enabled.length
    else if (event.key === 'ArrowLeft') next = (index - 1 + enabled.length) % enabled.length
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = enabled.length - 1
    else return
    event.preventDefault(); onChange(enabled[next].value)
    const buttons = event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)'); buttons?.[next]?.focus()
  }}>{item.label}</button>)}</div>
}

export function EmptyState({ title, children, action }: { title: string; children?: ReactNode; action?: ReactNode }) {
  return <div className="ds-empty"><h3>{title}</h3><p>{children}</p>{action}</div>
}
