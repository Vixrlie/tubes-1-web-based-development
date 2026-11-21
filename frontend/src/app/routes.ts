/** SPA routes. These are browser URLs, never backend API URLs. */
export const paths = {
  home: '/home',
  login: '/login',
  databases: '/databases',
  database: (id: string) => `/databases/${encodeURIComponent(id)}`,
  schema: (id: string) => `/databases/${encodeURIComponent(id)}/schema`,
  table: (id: string, schema: string, table: string) => `/databases/${encodeURIComponent(id)}/schemas/${encodeURIComponent(schema)}/tables/${encodeURIComponent(table)}`,
  migrations: (id: string) => `/databases/${encodeURIComponent(id)}/migrations`,
  admin: '/admin',
  users: '/admin/users',
  groups: '/admin/groups',
  connections: '/admin/connections',
  catalog: '/design-system/foundations',
} as const
