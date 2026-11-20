import { useEffect, useId, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { ChevronDownIcon } from './control-icons'

export type DropdownOption = { value: string; label: string; disabled?: boolean }
export type DropdownProps = {
  label: string
  options: DropdownOption[]
  value: string
  onChange: (value: string) => void
  placeholder?: string
  helper?: string
  error?: string
  disabled?: boolean
  id?: string
  name?: string
}

/** Controlled, single-select dropdown. Keyboard focus stays on the combobox. */
export function Dropdown({ label, options, value, onChange, placeholder = 'Pilih opsi', helper, error, disabled = false, id, name }: DropdownProps) {
  const generatedId = useId()
  const fieldId = id ?? generatedId
  const listId = `${fieldId}-options`
  const root = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const search = useRef({ text: '', time: 0 })
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(-1)
  const expanded = open && !disabled
  const selected = options.find(option => option.value === value)

  useEffect(() => {
    if (!expanded) return
    function onPointerDown(event: PointerEvent) {
      if (!root.current?.contains(event.target as Node)) setOpen(false)
    }
    document.addEventListener('pointerdown', onPointerDown)
    return () => document.removeEventListener('pointerdown', onPointerDown)
  }, [expanded])

  useEffect(() => {
    if (expanded && active >= 0) document.getElementById(`${listId}-${active}`)?.scrollIntoView({ block: 'nearest' })
  }, [active, expanded, listId])

  function show(last = false) {
    const current = options.findIndex(option => option.value === value && !option.disabled)
    const available = options.map((option, index) => option.disabled ? -1 : index).filter(index => index >= 0)
    setActive(current >= 0 ? current : available[last ? available.length - 1 : 0] ?? -1)
    setOpen(true)
  }

  function choose(index: number) {
    const option = options[index]
    if (!option || option.disabled) return
    onChange(option.value)
    setOpen(false)
    trigger.current?.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const available = options.map((option, index) => option.disabled ? -1 : index).filter(index => index >= 0)
    if (event.key === 'Tab' || event.key === 'Escape') {
      if (expanded && event.key === 'Escape') event.preventDefault()
      setOpen(false)
    } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (!expanded) { show(event.key === 'ArrowUp'); return }
      const index = available.indexOf(active)
      const next = event.key === 'ArrowDown' ? (index + 1) % available.length : (index <= 0 ? available.length - 1 : index - 1)
      setActive(available[next] ?? -1)
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault(); setOpen(true)
      setActive(available[event.key === 'Home' ? 0 : available.length - 1] ?? -1)
    } else if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      if (expanded) choose(active)
      else show()
    } else if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault()
      const now = Date.now()
      const text = (now - search.current.time > 700 ? '' : search.current.text) + event.key.toLowerCase()
      search.current = { text, time: now }
      const match = options.findIndex(option => !option.disabled && option.label.toLowerCase().startsWith(text))
      if (match >= 0) { setActive(match); setOpen(true) }
    }
  }

  return <div ref={root} className={`ds-field ds-dropdown ${error ? 'ds-field--error' : ''}`} onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false) }}>
    <label id={`${fieldId}-label`} htmlFor={fieldId}>{label}</label>
    {name && <input type="hidden" name={name} value={value} disabled={disabled} />}
    <div className="ds-dropdown-control"><button ref={trigger} id={fieldId} type="button" role="combobox" className="ds-dropdown-trigger" disabled={disabled}
      aria-haspopup="listbox" aria-expanded={expanded} aria-controls={listId} aria-labelledby={`${fieldId}-label`}
      aria-activedescendant={expanded && active >= 0 && options[active] ? `${listId}-${active}` : undefined}
      aria-invalid={error ? true : undefined} aria-describedby={error || helper ? `${fieldId}-help` : undefined}
      onClick={() => expanded ? setOpen(false) : show()} onKeyDown={onKeyDown}>
      <span className={selected ? '' : 'ds-dropdown-placeholder'}>{selected?.label ?? placeholder}</span><ChevronDownIcon />
    </button>
    {expanded && <div id={listId} role="listbox" aria-labelledby={`${fieldId}-label`} className="ds-dropdown-options">
      {options.length === 0 ? <p className="ds-muted ds-dropdown-empty">Tidak ada opsi</p> : options.map((option, index) => <div key={option.value} id={`${listId}-${index}`} role="option"
        aria-selected={value === option.value} aria-disabled={option.disabled || undefined} data-active={active === index || undefined}
        className="ds-dropdown-option" onPointerMove={() => { if (!option.disabled) setActive(index) }} onPointerDown={event => event.preventDefault()} onClick={() => choose(index)}>
        <span>{option.label}</span>{value === option.value && <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m5 12 4 4 10-10" /></svg>}
      </div>)}
    </div>}</div>
    {(error || helper) && <small id={`${fieldId}-help`}>{error ?? helper}</small>}
  </div>
}
