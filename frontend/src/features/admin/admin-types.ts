export type AdminDashboardStats = {
  totalUsers: number
  activeUsers: number
  totalGroups: number
  totalConnections: number
}

export type AdminUser = {
  id: string
  username: string
  fullName: string
  email: string
  isPlatformAdmin: boolean
  isActive: boolean
  groups: string[]
  createdAt: string
}

export type AdminEffectiveAccess = {
  connectionId: string
  connectionName: string
  groupId: string
  groupName: string
  level: string
}

export type AdminUserDetail = AdminUser & {
  isSelf: boolean
  effectiveAccess: AdminEffectiveAccess[]
}

export type CreateUserPayload = {
  username: string
  fullName: string
  email: string
  password: string
  isPlatformAdmin: boolean
}

export type UpdateUserPayload = {
  fullName: string
  email: string
  isPlatformAdmin: boolean
}

export type AdminGroupAccess = {
  connectionId?: string
  connectionName: string
  level: 'view' | 'edit' | string
}

export type AdminGroup = {
  id: string
  name: string
  memberCount: number
  accesses: AdminGroupAccess[]
  createdAt: string
}

export type AdminMember = {
  id: string
  username: string
  fullName: string
  email: string
  isPlatformAdmin: boolean
  isActive: boolean
}

export type AdminConnRef = {
  id: string
  name: string
}

export type AdminGroupDetail = {
  id: string
  name: string
  createdAt: string
  members: AdminMember[]
  candidateUsers: AdminMember[]
  grants: AdminGroupAccess[]
  availableConnections: AdminConnRef[]
  /** Anggota yang menyetujui migration atas nama grup; null bila belum ditetapkan. */
  leaderId: string | null
}

export type AdminConnectionGroup = {
  name: string
  memberCount: number
  level: string
}

export type AdminConnection = {
  id: string
  name: string
  description: string
  dbms: 'postgres' | 'mysql' | string
  host: string
  port: string
  database: string
  username: string
  groups: AdminConnectionGroup[]
  createdAt: string
}

export type ConnectionFormPayload = {
  id?: string
  dbms: 'postgres' | 'mysql'
  name: string
  description?: string
  host: string
  port: string
  database: string
  username: string
  password: string
}

export type ConnectionTestResult = {
  ok: boolean
  latencyMs: number
  message: string
}
