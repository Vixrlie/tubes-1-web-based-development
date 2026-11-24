import test from 'node:test'
import assert from 'node:assert/strict'
import { createMockSchemaSource, SchemaSourceError, selectSchema, withWorkspacePreview } from '../src/features/databases/schema-source.ts'
import { paths } from '../src/app/routes.ts'

const signal = () => new AbortController().signal

test('schema fixture foreign keys resolve to existing tables and respect database metadata', async () => {
  const source = createMockSchemaSource('ready', 0)
  const workspace = await source.load('mock-shop', signal())
  const schema = workspace.schemas[0]
  assert.equal(workspace.database.name, 'shop_db')
  assert.deepEqual(schema.tables.map(table => [table.name, table.columnCount]), [['products', 6], ['users', 3], ['orders', 5]])
  assert.equal(workspace.database.schemaCount, workspace.schemas.length)
  assert.equal(workspace.database.tableCount, schema.tables.length)
  for (const relation of schema.relations) {
    assert.ok(schema.tables.find(table => table.name === relation.fromTable))
    const target = workspace.schemas.find(item => item.name === relation.toSchema)?.tables.find(table => table.name === relation.toTable)
    assert.ok(target?.primaryKey.includes(relation.toColumn))
  }
})

test('navigation between databases and consumer mutations cannot leak another database schema', async () => {
  const source = createMockSchemaSource('ready', 0)
  const shop = await source.load('mock-shop', signal())
  shop.schemas[0].tables[0].primaryKey[0] = 'modified'
  shop.schemas[0].relations[0].toTable = 'modified'
  const inventory = await source.load('mock-inventory', signal())
  assert.equal(inventory.database.name, 'inventory_db')
  assert.deepEqual(inventory.schemas[0].tables.map(table => table.name), ['items', 'warehouses', 'stock'])
  const restored = await source.load('mock-shop', signal())
  assert.deepEqual(restored.schemas[0].tables[0].primaryKey, ['id'])
  assert.equal(restored.schemas[0].relations[0].toTable, 'users')
})

test('unknown, disconnected, and forbidden workspaces produce distinct errors', async () => {
  const source = createMockSchemaSource('ready', 0)
  await assert.rejects(source.load('nonexistent', signal()), error => error instanceof SchemaSourceError && error.code === 'not-found')
  await assert.rejects(source.load('mock-hr', signal()), error => error instanceof SchemaSourceError && error.code === 'unavailable')
  await assert.rejects(createMockSchemaSource('denied', 0).load('mock-shop', signal()), error => error instanceof SchemaSourceError && error.code === 'denied')
})

test('read-only preview keeps all table permissions read-only without mutating normal fixtures', async () => {
  const readonly = await createMockSchemaSource('readonly', 0).load('mock-shop', signal())
  assert.equal(readonly.database.permission, 'view')
  assert.ok(readonly.schemas.every(schema => schema.tables.every(table => table.permission === 'view')))
  const normal = await createMockSchemaSource('ready', 0).load('mock-shop', signal())
  assert.equal(normal.database.permission, 'edit')
})

test('empty/multiple schema selection distinguishes no tables, no schema, and unknown schema', async () => {
  const empty = await createMockSchemaSource('empty', 0).load('mock-shop', signal())
  assert.deepEqual(empty.schemas[0].tables, [])
  assert.deepEqual(empty.schemas[0].relations, [])
  assert.equal(empty.database.tableCount, 0)
  const none = await createMockSchemaSource('noschema', 0).load('mock-shop', signal())
  assert.equal(selectSchema(none.schemas), undefined)
  const multiple = await createMockSchemaSource('multiple', 0).load('mock-shop', signal())
  assert.equal(selectSchema(multiple.schemas)?.name, 'public')
  assert.equal(selectSchema(multiple.schemas, 'audit')?.name, 'audit')
  assert.equal(selectSchema(multiple.schemas, 'unknown'), undefined)
  assert.equal(withWorkspacePreview(paths.schema('mock-shop') + '?schema=audit', 'multiple'), '/databases/mock-shop/schema?schema=audit&preview=multiple')
})

test('aborted schema loads reject before returning a stale workspace', async () => {
  const source = createMockSchemaSource('ready', 20)
  const controller = new AbortController()
  const pending = source.load('mock-shop', controller.signal)
  controller.abort()
  await assert.rejects(pending, { name: 'AbortError' })
  await assert.rejects(source.load('mock-shop', controller.signal), { name: 'AbortError' })
})
