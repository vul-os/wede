// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { createDocumentSaves, writeDocument } from './documentSaves'
import { lspExtension } from './lsp'
import { LspSocket, flushLsp } from '../test/lspSocket'

let views: EditorView[]
function mount(saves: ReturnType<typeof createDocumentSaves>) {
  const view = new EditorView({ state: EditorState.create({ doc: 'original', extensions: lspExtension({
    serverUri: 'ws://localhost/lsp', rootUri: 'file:///project', documentUri: 'file:///project/src/main.rs', languageId: 'rust',
    persistence: { events: saves, workspaceId: 'one', path: 'src/main.rs' },
  }) }), parent: document.body })
  views.push(view)
  return view
}
const authFetch = () => Promise.resolve(new Response(JSON.stringify({ status: 'ok' })))

beforeEach(() => {
  views = []
  LspSocket.instances = []
  vi.stubGlobal('WebSocket', LspSocket)
})
afterEach(() => {
  for (const view of views) view.destroy()
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

it.each([true, false])('sends didSave after acknowledgement, honoring includeText=%s and preserving versions', async includeText => {
  const saves = createDocumentSaves()
  const view = mount(saves)
  await flushLsp()
  const socket = LspSocket.instances[0]
  socket.open()
  await flushLsp()
  expect(socket.sent[0].params?.capabilities).toMatchObject({ textDocument: { synchronization: { didSave: true } } })
  socket.initialized({ textDocumentSync: { openClose: true, change: 1, save: { includeText } } })
  await flushLsp()
  view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: 'saved snapshot' } })
  await flushLsp()
  expect(socket.sent.some(m => m.method === 'textDocument/didSave')).toBe(false)
  let acknowledge!: (response: Response) => void
  const writing = writeDocument(() => new Promise(resolve => { acknowledge = resolve }), saves, 'one', 'src/main.rs', 'saved snapshot')
  view.dispatch({ changes: { from: 0, insert: 'unsaved ' } })
  await flushLsp()
  expect(socket.sent.some(m => m.method === 'textDocument/didSave')).toBe(false)
  acknowledge(new Response(JSON.stringify({ status: 'ok' })))
  await writing
  await flushLsp()
  expect(socket.sent.at(-1)).toEqual({ jsonrpc: '2.0', method: 'textDocument/didSave', params: {
    textDocument: { uri: 'file:///project/src/main.rs' }, ...(includeText ? { text: 'saved snapshot' } : {}),
  } })
  expect(socket.sent.filter(m => m.method === 'textDocument/didChange').map(m => m.params?.textDocument?.version)).toEqual([1, 2])
  expect(view.state.doc.toString()).toBe('unsaved saved snapshot')
})

it('queues an acknowledged save until after delayed didOpen, never another workspace/file save', async () => {
  const saves = createDocumentSaves()
  mount(saves)
  await flushLsp()
  const socket = LspSocket.instances[0]
  socket.open()
  await flushLsp()
  await writeDocument(authFetch, saves, 'one', 'src/main.rs', 'confirmed')
  await writeDocument(authFetch, saves, 'other', 'src/main.rs', 'wrong workspace')
  await writeDocument(authFetch, saves, 'one', 'other.rs', 'wrong file')
  expect(socket.sent.map(m => m.method)).toEqual(['initialize'])
  socket.initialized({ textDocumentSync: { save: { includeText: true } } })
  await flushLsp()
  expect(socket.sent.map(m => m.method)).toEqual(['initialize', 'initialized', 'textDocument/didOpen', 'textDocument/didSave'])
  expect(socket.sent.at(-1)?.params?.text).toBe('confirmed')
})

it.each([2, { save: false }, {}])('does not send unrequested save notifications (sync=%j)', async textDocumentSync => {
  const saves = createDocumentSaves()
  mount(saves)
  await flushLsp()
  const socket = LspSocket.instances[0]
  socket.open()
  await flushLsp()
  socket.initialized({ textDocumentSync })
  await flushLsp()
  await writeDocument(authFetch, saves, 'one', 'src/main.rs', 'confirmed')
  await flushLsp()
  expect(socket.sent.some(m => m.method === 'textDocument/didSave')).toBe(false)
})

it('supports save:true and never sends queued saves after destruction', async () => {
  const saves = createDocumentSaves()
  const view = mount(saves)
  await flushLsp()
  const socket = LspSocket.instances[0]
  socket.open()
  await flushLsp()
  socket.initialized({ textDocumentSync: { save: true } })
  await flushLsp()
  const acknowledge = saves.capture('one', 'src/main.rs')
  acknowledge('first')
  await flushLsp()
  expect(socket.sent.at(-1)?.method).toBe('textDocument/didSave')
  expect(socket.sent.at(-1)?.params).not.toHaveProperty('text')
  acknowledge('second')
  view.destroy()
  await flushLsp()
  expect(socket.sent.filter(m => m.method === 'textDocument/didSave')).toHaveLength(1)
})
