import { useEffect, useRef } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { AdminWorkspaceNavigation } from '../../features/admin/AdminWorkspaceNavigation'

export default function AdminLayout() {
  const { pathname } = useLocation()
  const contentRef = useRef<HTMLDivElement>(null)
  useEffect(() => { if (contentRef.current) contentRef.current.scrollTop = 0 }, [pathname])
  return <div className="app-workspace"><aside><AdminWorkspaceNavigation /></aside><div className="app-admin-content" ref={contentRef} role="region" aria-label="Konten administrasi" tabIndex={0}><Outlet /></div></div>
}
