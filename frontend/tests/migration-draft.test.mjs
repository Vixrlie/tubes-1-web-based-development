import test from 'node:test'
import assert from 'node:assert/strict'
import { createMockDraftSource, validateDraft, DraftValidationError, clearMockDraftStorage } from '../src/features/migrations/draft-source.ts'
import { paths } from '../src/app/routes.ts'

function memoryStorage() {
  const values = new Map()
  return { get length() { return values.size }, key: index => [...values.keys()][index] ?? null, getItem: key => values.get(key) ?? null, setItem: (key, value) => { values.set(key, value) }, removeItem: key => { values.delete(key) } }
}
const signal = () => new AbortController().signal
const input = { name: '  add_product_status  ', sqlUp: "ALTER TABLE products ADD COLUMN status TEXT DEFAULT 'active';", sqlDown: 'ALTER TABLE products DROP COLUMN status;', irreversible: false }
function source(storage, overrides = {}) { return createMockDraftSource({ ownerId: '7', author: 'arina', storage: () => storage, delay: 0, ...overrides }) }

test('draft validation requires name and SQL Up; SQL Down is required only for reversible changes', () => {
  assert.deepEqual(Object.keys(validateDraft({ name: ' ', sqlUp: '\n', sqlDown: '', irreversible: false })), ['name', 'sqlUp', 'sqlDown'])
  assert.deepEqual(validateDraft(input), {})
  assert.deepEqual(validateDraft({ ...input, sqlDown: '', irreversible: true }), {})
  assert.ok(validateDraft({ ...input, name: 'x'.repeat(121) }).name)
  assert.ok(validateDraft({ ...input, sqlUp: 'x'.repeat(50001) }).sqlUp)
  assert.ok(validateDraft({ ...input, sqlDown: 'x'.repeat(50001) }).sqlDown)
})

test('saved draft survives a fresh adapter, preserves SQL and is not an execution', async () => {
  const storage = memoryStorage()
  const adapter = source(storage)
  assert.equal(await adapter.nextVersion('mock-shop', signal()), 'v015')
  const draft = await adapter.save('mock-shop', input, signal())
  assert.match(draft.id, /^draft-/)
  assert.equal(draft.version, 'v015')
  assert.equal(draft.name, 'add_product_status')
  assert.equal(draft.status, 'draft')
  assert.equal(draft.sqlUp, input.sqlUp)
  assert.equal(draft.sqlDown, input.sqlDown)
  assert.deepEqual(await source(storage).list('mock-shop', signal()), [draft])
  draft.name = 'consumer mutation'
  assert.equal((await source(storage).list('mock-shop', signal()))[0].name, 'add_product_status')
})

test('accounts and connections do not share drafts; each connection has its own version sequence', async () => {
  const storage = memoryStorage()
  const adapter = source(storage)
  const shop = await adapter.save('mock-shop', input, signal())
  assert.deepEqual(await adapter.list('mock-inventory', signal()), [])
  assert.deepEqual(await source(storage, { ownerId: '8' }).list('mock-shop', signal()), [])
  const inventory = await adapter.save('mock-inventory', { ...input, name: 'inventory_change' }, signal())
  assert.equal(inventory.version, 'v001')
  assert.equal(shop.version, 'v015')
  await assert.rejects(adapter.save('mock-inventory', input, signal(), shop.id), /Draft tidak ditemukan/)
})

test('edit replaces a draft without changing ID, version, created time or execution state', async () => {
  const storage = memoryStorage()
  const adapter = source(storage)
  const first = await adapter.save('mock-shop', input, signal())
  const updated = await adapter.save('mock-shop', { ...input, name: 'delete_archive', sqlUp: 'DROP TABLE archive;', irreversible: true }, signal(), first.id)
  assert.equal(updated.id, first.id)
  assert.equal(updated.version, first.version)
  assert.equal(updated.createdAt, first.createdAt)
  assert.equal(updated.sqlDown, '')
  assert.equal(updated.irreversible, true)
  assert.equal(updated.status, 'draft')
  assert.deepEqual(await adapter.list('mock-shop', signal()), [updated])
})

test('concurrent saves retain both drafts and allocate distinct versions', async () => {
  const storage = memoryStorage()
  const adapter = source(storage)
  const drafts = await Promise.all([adapter.save('mock-shop', input, signal()), adapter.save('mock-shop', { ...input, name: 'another_change' }, signal())])
  assert.equal(new Set(drafts.map(draft => draft.id)).size, 2)
  assert.equal(new Set(drafts.map(draft => draft.version)).size, 2)
  assert.equal((await adapter.list('mock-shop', signal())).length, 2)
})

test('read-only, denied, disconnected, unknown and invalid submissions never write drafts', async () => {
  const storage = memoryStorage()
  await assert.rejects(source(storage, { preview: 'readonly' }).save('mock-shop', input, signal()), /hanya/)
  await assert.rejects(source(storage, { preview: 'denied' }).save('mock-shop', input, signal()), error => error.code === 'denied')
  await assert.rejects(source(storage).save('mock-hr', input, signal()), error => error.code === 'unavailable')
  await assert.rejects(source(storage).save('unknown', input, signal()), error => error.code === 'not-found')
  await assert.rejects(source(storage).save('mock-shop', { ...input, name: '' }, signal()), error => error instanceof DraftValidationError && !!error.fields.name)
  assert.equal(storage.length, 0)
})

test('aborted saves and an expired session cannot restore or write drafts', async () => {
  const storage = memoryStorage()
  const controller = new AbortController()
  const pending = source(storage, { delay: 20 }).save('mock-shop', input, controller.signal)
  controller.abort()
  await assert.rejects(pending, { name: 'AbortError' })
  let active = true
  const expired = source(storage, { delay: 20, isActive: () => active })
  const save = expired.save('mock-shop', input, signal())
  active = false
  await assert.rejects(save, /Sesi sudah berubah/)
  assert.equal(storage.length, 0)
})

test('quota failure reports failure; corrupt storage cannot masquerade as a valid draft', async () => {
  const storage = memoryStorage()
  const broken = { ...storage, setItem() { throw new Error('QuotaExceededError') } }
  await assert.rejects(source(broken).save('mock-shop', input, signal()), /Draft belum dapat disimpan/)
  assert.equal(storage.length, 0)
  await source(storage).save('mock-shop', input, signal())
  const key = storage.key(0)
  storage.setItem(key, '{invalid')
  await assert.rejects(source(storage).list('mock-shop', signal()), /tidak dapat dibaca/)
  storage.setItem(key, JSON.stringify([{ status: 'executed' }]))
  await assert.rejects(source(storage).list('mock-shop', signal()), /tidak valid/)
})

test('session cleanup preserves only the current owner and never clears unrelated browser storage', async () => {
  const storage = memoryStorage()
  await source(storage).save('mock-shop', input, signal())
  await source(storage, { ownerId: '8' }).save('mock-shop', input, signal())
  storage.setItem('other-feature', 'keep')
  clearMockDraftStorage(storage, '8')
  assert.deepEqual(await source(storage).list('mock-shop', signal()), [])
  assert.equal((await source(storage, { ownerId: '8' }).list('mock-shop', signal())).length, 1)
  clearMockDraftStorage(storage)
  assert.equal(storage.length, 1)
  assert.equal(storage.getItem('other-feature'), 'keep')
})

test('draft route builders encode IDs separately from execution routes', () => {
  assert.equal(paths.newMigration('db /1'), '/databases/db%20%2F1/migrations/new')
  assert.equal(paths.migrationDraft('db /1', 'draft /a'), '/databases/db%20%2F1/migrations/drafts/draft%20%2Fa')
  assert.equal(paths.editMigrationDraft('db /1', 'draft /a'), '/databases/db%20%2F1/migrations/drafts/draft%20%2Fa/edit')
})
