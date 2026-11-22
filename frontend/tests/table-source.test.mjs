import test from 'node:test'
import assert from 'node:assert/strict'
import { createMockTableSource, formatTableCell, parseTablePage, parseTableTab } from '../src/features/databases/table-source.ts'
import { createMockSchemaSource, SchemaSourceError } from '../src/features/databases/schema-source.ts'
import { paginationItems } from '../src/design-system/pagination.ts'

const identity = { connectionId: 'mock-shop', schemaName: 'public', tableName: 'users' }
const signal = () => new AbortController().signal
const source = () => createMockTableSource({ delay: 0 })

test('all table descriptions agree with schema counts, PKs, and FK targets', async () => {
  for (const connectionId of ['mock-shop', 'mock-inventory', 'mock-analytics']) {
    const workspace = await createMockSchemaSource('ready', 0).load(connectionId, signal())
    for (const schema of workspace.schemas) for (const table of schema.tables) {
      const description = await source().describe({ connectionId, schemaName: schema.name, tableName: table.name }, signal())
      assert.equal(description.columns.length, table.columnCount)
      assert.deepEqual(description.columns.filter(column => column.primaryKey).map(column => column.name), table.primaryKey)
      for (const column of description.columns) if (column.references) {
        const target = await source().describe({ connectionId, schemaName: column.references.schema, tableName: column.references.table }, signal())
        assert.ok(target.columns.some(item => item.name === column.references.column))
      }
      for (const item of [...description.constraints, ...description.indexes]) for (const column of item.columns) assert.ok(description.columns.some(item => item.name === column))
    }
  }
})

test('pagination loads distinct rows and clamps oversized pages to the actual last page', async () => {
  const table = source()
  const first = await table.rows({ ...identity, page: 1, direction: 'asc' }, signal())
  const second = await table.rows({ ...identity, page: 2, direction: 'asc' }, signal())
  const last = await table.rows({ ...identity, page: 99999, direction: 'asc' }, signal())
  assert.equal(first.total, 1250)
  assert.deepEqual(first.rows.map(row => row.id), [1, 2, 3, 4])
  assert.deepEqual(second.rows.map(row => row.id), [5, 6, 7, 8])
  assert.equal(last.page, 313)
  assert.deepEqual(last.rows.map(row => row.id), [1249, 1250])
  assert.ok(first.rows.every(row => Object.keys(row).join(',') === first.columns.join(',')))
})

test('descending sort precedes pagination and negative/NaN pages recover safely', async () => {
  const table = source()
  const rows = await table.rows({ ...identity, page: 2, direction: 'desc' }, signal())
  assert.deepEqual(rows.rows.map(row => row.id), [1246, 1245, 1244, 1243])
  for (const page of [-3, NaN, Infinity, 1.2]) assert.equal((await table.rows({ ...identity, page, direction: 'asc' }, signal())).page, 1)
})

test('changing table/database changes fields and cloned descriptions never mutate the next load', async () => {
  const table = source()
  const description = await table.describe(identity, signal())
  description.columns[0].name = 'modified'
  description.indexes[0].columns[0] = 'modified'
  assert.equal((await table.describe(identity, signal())).columns[0].name, 'id')
  const products = await table.rows({ ...identity, tableName: 'products', page: 1, direction: 'asc' }, signal())
  const analytics = await table.rows({ ...identity, connectionId: 'mock-analytics', page: 1, direction: 'asc' }, signal())
  assert.equal(products.columns.length, 6)
  assert.ok(!products.columns.includes('email'))
  assert.ok(analytics.columns.includes('username'))
  assert.equal(analytics.rows[0].email, 'analyst1@mail.com')
})

test('read-only mode preserves permission; empty rows keep column metadata', async () => {
  const readonly = await createMockTableSource({ workspacePreview: 'readonly', delay: 0 }).describe(identity, signal())
  assert.equal(readonly.permission, 'view')
  const emptySource = createMockTableSource({ preview: 'empty', delay: 0 })
  const page = await emptySource.rows({ ...identity, page: 12, direction: 'asc' }, signal())
  assert.deepEqual(page.rows, [])
  assert.deepEqual(page.columns, ['id', 'email', 'created_at'])
  assert.equal(page.page, 1)
  assert.equal(page.pages, 1)
  assert.equal(page.total, 0)
  assert.equal((await emptySource.describe(identity, signal())).columns.length, 3)
})

test('unknown schema/table, disconnection and errors cannot return fallback user data', async () => {
  const table = source()
  for (const changed of [{ schemaName: 'unknown' }, { tableName: 'unknown' }, { connectionId: 'unknown' }]) await assert.rejects(table.describe({ ...identity, ...changed }, signal()), error => error instanceof SchemaSourceError && error.code === 'not-found')
  await assert.rejects(table.describe({ ...identity, connectionId: 'mock-hr' }, signal()))
  await assert.rejects(createMockTableSource({ preview: 'error', delay: 0 }).describe(identity, signal()))
})

test('nullable values, false, zero and empty strings retain their meaning', async () => {
  const page = await source().rows({ ...identity, page: 2, direction: 'asc' }, signal())
  assert.equal(page.rows[0].created_at, null)
  assert.equal(formatTableCell(null), 'NULL')
  assert.equal(formatTableCell(false), 'false')
  assert.equal(formatTableCell(0), '0')
  assert.equal(formatTableCell(''), '""')
  assert.equal(formatTableCell('9007199254740993'), '9007199254740993')
})

test('URL parsing and numbered pagination remain bounded at first, middle and last pages', () => {
  for (const page of [null, '0', '-1', 'NaN', '1.5', '9007199254740993']) assert.equal(parseTablePage(page), 1)
  assert.equal(parseTablePage('313'), 313)
  assert.equal(parseTableTab('data'), 'data')
  assert.equal(parseTableTab('constraints'), 'constraints')
  assert.equal(parseTableTab('unknown'), 'columns')
  assert.deepEqual(paginationItems(1, 313), [1, 2, 'ellipsis-right', 313])
  assert.deepEqual(paginationItems(313, 313), [1, 'ellipsis-left', 312, 313])
  assert.deepEqual(paginationItems(1, 1), [1])
  assert.deepEqual(paginationItems(5, 10), [1, 'ellipsis-left', 4, 5, 6, 'ellipsis-right', 10])
})

test('aborted metadata and row requests reject instead of restoring a stale table', async () => {
  const controller = new AbortController()
  const table = createMockTableSource({ delay: 20 })
  const pending = table.rows({ ...identity, page: 1, direction: 'asc' }, controller.signal)
  controller.abort()
  await assert.rejects(pending, { name: 'AbortError' })
  await assert.rejects(table.describe(identity, controller.signal), { name: 'AbortError' })
})
