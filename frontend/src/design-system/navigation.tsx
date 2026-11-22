import type { ReactNode } from 'react'
import { NavLink } from 'react-router-dom'
import { IconButton } from './primitives'
import { MenuIcon } from './control-icons'

const asset = '/design-system/'
const icons = { database: '46f20.svg', table: '32036.svg', migration: '85892.svg', user: '5cc3c.svg', group: 'c52b9.svg', link: '65b4c.svg' }
export function Icon({ name }: { name: keyof typeof icons }) {
  return <img className="ds-icon" src={asset + icons[name]} alt="" />
}

export function Header({ navigation, profile, onMenu, menuOpen = false, homeTo = '/' }: { navigation?: ReactNode; profile?: ReactNode; onMenu?: () => void; menuOpen?: boolean; homeTo?: string }) {
  return <header className="ds-header"><div className="ds-brand-row"><NavLink to={homeTo} aria-label="Pathgres beranda"><img className="ds-logo" src={asset + 'c0d63.png'} width="191" height="44" alt="Pathgres" /></NavLink></div><nav className="ds-header-nav" aria-label="Navigasi utama">{navigation}</nav><div className="ds-header-actions">{profile}{onMenu && <span className="ds-menu-trigger"><IconButton label="Buka navigasi" aria-expanded={menuOpen} aria-haspopup="dialog" onClick={onMenu}><MenuIcon /></IconButton></span>}</div></header>
}

export function SidebarItem({ active = false, children, icon, nested = false, className = '', ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { active?: boolean; nested?: boolean; icon?: ReactNode }) {
  return <button type="button" {...props} aria-current={active ? 'page' : undefined} className={`ds-sidebar-item ${nested ? 'ds-sidebar-item--nested' : ''} ${className}`} >{icon && <span className="ds-sidebar-item-icon" aria-hidden="true">{icon}</span>}<span className="ds-sidebar-item-content">{typeof children === 'string' || typeof children === 'number' ? <span className="ds-nav-label">{children}</span> : children}</span></button>
}

export function Sidebar({ label, children }: { label: string; children: ReactNode }) {
  return <nav className="ds-sidebar" aria-label={label}><div className="ds-sidebar-heading"><p className="ds-eyebrow">{label}</p></div>{children}</nav>
}

export function Breadcrumb({ items }: { items: { label: string; href?: string }[] }) {
  return <nav aria-label="Breadcrumb"><ol className="ds-breadcrumb">{items.map((item, index) => <li key={index}>{item.href ? <NavLink to={item.href}>{item.label}</NavLink> : <span aria-current={index === items.length - 1 ? 'page' : undefined}>{item.label}</span>}</li>)}</ol></nav>
}

export function BottomNavigation({ children }: { children: ReactNode }) {
  return <nav className="ds-bottom-nav" aria-label="Navigasi mobile">{children}</nav>
}
