import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { Spinner } from '../design-system'
import RoutePlaceholder from '../pages/RoutePlaceholder'
import NotFoundPage from '../pages/NotFoundPage'
import AppLayout from './layouts/AppLayout'
import AuthLayout from './layouts/AuthLayout'
import DatabaseLayout from './layouts/DatabaseLayout'
import AdminLayout from './layouts/AdminLayout'
import { paths } from './routes'
import { AdminGate, SessionGate } from '../features/auth/guards'
import LoginPage from '../features/auth/LoginPage'
import HomePage from '../features/home/HomePage'
import DatabaseListPage from '../features/databases/DatabaseListPage'
import SchemaExplorerPage from '../features/databases/SchemaExplorerPage'
import TableDetailPage from '../features/databases/TableDetailPage'

const CatalogLayout = lazy(() => import('../catalog/CatalogLayout'))
const FoundationsPage = lazy(() => import('../catalog/FoundationsPage'))
const ComponentsPage = lazy(() => import('../catalog/ComponentsPage'))
const NavigationPage = lazy(() => import('../catalog/NavigationPage'))
const PatternsPage = lazy(() => import('../catalog/PatternsPage'))

export function AppRouter() {
  return <Suspense fallback={<div className="app-status-page"><Spinner label="Memuat halaman" /></div>}><Routes>
    <Route path="/" element={<Navigate to={paths.home} replace />} />
    <Route path="/design-system" element={<CatalogLayout />}>
      <Route index element={<Navigate to="foundations" replace />} />
      <Route path="foundations" element={<FoundationsPage />} />
      <Route path="components" element={<ComponentsPage />} />
      <Route path="navigation" element={<NavigationPage />} />
      <Route path="patterns" element={<PatternsPage />} />
      <Route path="*" element={<NotFoundPage />} />
    </Route>
    <Route element={<SessionGate guest />}><Route element={<AuthLayout />}><Route path="/login" element={<LoginPage />} /></Route></Route>
    <Route element={<SessionGate />}><Route element={<AppLayout />}>
      <Route path="/home" element={<HomePage />} />
      <Route path="/databases" element={<DatabaseListPage />} />
      <Route path="/databases/:connectionId" element={<DatabaseLayout />}>
        <Route index element={<SchemaExplorerPage />} />
        <Route path="schema" element={<SchemaExplorerPage />} />
        <Route path="schemas/:schemaName/tables/:tableName" element={<TableDetailPage />} />
        <Route path="migrations" element={<RoutePlaceholder title="Migrasi" />} />
        <Route path="migrations/new" element={<RoutePlaceholder title="Buat migrasi" />} />
        <Route path="migrations/diff" element={<RoutePlaceholder title="Schema Diff" />} />
        <Route path="migrations/:migrationId" element={<RoutePlaceholder title="Detail migrasi" />} />
        <Route path="migrations/:migrationId/preview" element={<RoutePlaceholder title="Preview migrasi" />} />
      </Route>
      <Route element={<AdminGate />}><Route path="/admin" element={<AdminLayout />}>
        <Route index element={<RoutePlaceholder title="Admin Dashboard" />} />
        <Route path="users" element={<RoutePlaceholder title="Pengguna" />} />
        <Route path="users/new" element={<RoutePlaceholder title="Tambah pengguna" />} />
        <Route path="users/:userId" element={<RoutePlaceholder title="Edit pengguna" />} />
        <Route path="groups" element={<RoutePlaceholder title="Grup" />} />
        <Route path="groups/new" element={<RoutePlaceholder title="Tambah grup" />} />
        <Route path="groups/:groupId" element={<RoutePlaceholder title="Detail grup" />} />
        <Route path="connections" element={<RoutePlaceholder title="Koneksi Database" />} />
        <Route path="connections/new" element={<RoutePlaceholder title="Tambah koneksi" />} />
        <Route path="connections/:connectionId" element={<RoutePlaceholder title="Detail koneksi" />} />
        <Route path="connections/:connectionId/edit" element={<RoutePlaceholder title="Edit koneksi" />} />
      </Route></Route>
    </Route></Route>
    <Route path="*" element={<NotFoundPage />} />
  </Routes></Suspense>
}
