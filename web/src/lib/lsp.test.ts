// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Compartment, EditorState, type Extension } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { languageServerPlugin } from 'codemirror-languageserver'
import { lspDocumentUris, lspExtension, type LspConfig } from './lsp'
import { LspSocket, flushLsp } from '../test/lspSocket'

const config: LspConfig = {
  serverUri: 'ws://localhost/api/workspaces/one/lsp?lang=rust&token=test',
  rootUri: 'file:///projects/one',
  documentUri: 'file:///projects/one/src/main.rs',
  languageId: 'rust',
}
let views: EditorView[]
function editor(extensions: Extension, doc = 'initial') {
  const view = new EditorView({ state: EditorState.create({ extensions, doc }), parent: document.body })
  views.push(view)
  return view
}
async function initialize(socket: LspSocket) {
  socket.open()
  await flushLsp()
  socket.initialized()
  await flushLsp()
}

beforeEach(() => {
  views = []
  LspSocket.instances = []
  LspSocket.peakLive = 0
  vi.stubGlobal('WebSocket', LspSocket)
})
afterEach(() => {
  for (const view of views) view.destroy()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe('server file URIs', () => {
  it('encodes path segments, not separators, and never invents a URI authority', () => {
    expect(lspDocumentUris('/home/wede/my project/', 'src/a #?%ü.rs')).toEqual({
      rootUri: 'file:///home/wede/my%20project',
      documentUri: 'file:///home/wede/my%20project/src/a%20%23%3F%25%C3%BC.rs',
    })
    expect(lspDocumentUris('/', 'main.rs')).toEqual({ rootUri: 'file:///', documentUri: 'file:///main.rs' })
  })
  it.each(['', '/main.rs', '../main.rs', 'src/../main.rs', './main.rs', 'src//main.rs', 'a\0.rs'])(
    'rejects invalid relative path %j', path => expect(lspDocumentUris('/project', path)).toBeNull(),
  )
  it('requires an absolute server root', () => expect(lspDocumentUris('project', 'main.rs')).toBeNull())
})

describe('editor-owned LSP', () => {
  it('is pure during extension/state construction and abandons same-turn destroyed views', async () => {
    const extension = lspExtension(config)
    EditorState.create({ extensions: extension })
    expect(LspSocket.instances).toHaveLength(0)
    editor(extension).destroy()
    await flushLsp()
    expect(LspSocket.instances).toHaveLength(0)
  })

  it('initializes once, opens the latest document, and closes on extension removal', async () => {
    const compartment = new Compartment()
    const extension = lspExtension(config)
    const view = editor(compartment.of(extension))
    await flushLsp()
    const socket = LspSocket.instances[0]
    socket.open()
    await flushLsp()
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: 'edited while initializing' } })
    expect(socket.sent.map(m => m.method)).toEqual(['initialize'])
    expect(socket.sent[0].params).toMatchObject({
      rootUri: config.rootUri, workspaceFolders: [{ uri: config.rootUri }],
    })
    socket.initialized()
    await flushLsp()
    expect(socket.sent.map(m => m.method)).toEqual(['initialize', 'initialized', 'textDocument/didOpen'])
    expect(socket.sent[2].params?.textDocument).toMatchObject({ uri: config.documentUri, text: 'edited while initializing', version: 0 })
    view.dispatch({ effects: compartment.reconfigure(extension) })
    view.dispatch({ changes: { from: 0, insert: 'more ' } })
    await flushLsp()
    expect(LspSocket.instances).toHaveLength(1)
    expect(socket.sent.at(-1)?.method).toBe('textDocument/didChange')
    expect(socket.sent.at(-1)?.params?.textDocument?.version).toBe(1)
    view.dispatch({ changes: { from: 0, insert: 'another ' } })
    await flushLsp()
    expect(socket.sent.at(-1)?.params?.textDocument?.version).toBe(2)
    view.dispatch({ effects: compartment.reconfigure([]) })
    expect(socket.closeCalls).toBe(1)
  })

  it('closes a connecting socket and ignores a delayed open', async () => {
    const view = editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    view.destroy()
    expect(socket.closeCalls).toBe(1)
    socket.open()
    await flushLsp()
    expect(socket.sent).toEqual([])
  })

  it('switches workspace/document atomically, closing the previous session before opening another', async () => {
    const compartment = new Compartment()
    const view = editor(compartment.of(lspExtension(config)))
    await flushLsp()
    const first = LspSocket.instances[0]
    first.open()
    await flushLsp()
    const next = { ...config, serverUri: 'ws://localhost/api/workspaces/two/lsp', rootUri: 'file:///projects/two', documentUri: 'file:///projects/two/other.rs' }
    view.dispatch({ effects: compartment.reconfigure(lspExtension(next)) })
    expect(first.closeCalls).toBe(1)
    first.initialized()
    await flushLsp()
    expect(first.sent.map(m => m.method)).toEqual(['initialize'])
    const second = LspSocket.instances[1]
    await initialize(second)
    expect(second.url).toBe(next.serverUri)
    expect(second.sent[0].params?.rootUri).toBe(next.rootUri)
    expect(second.sent[2].params?.textDocument?.uri).toBe(next.documentUri)
    expect(LspSocket.peakLive).toBe(1)
  })

  it('reuses only the pure extension, never the client, on editor/collaboration rebuild', async () => {
    const extension = lspExtension(config)
    const firstView = editor(extension)
    await flushLsp()
    const first = LspSocket.instances[0]
    await initialize(first)
    firstView.destroy()
    editor(extension, 'new collaborative text')
    await flushLsp()
    const second = LspSocket.instances[1]
    await initialize(second)
    expect(first.closeCalls).toBe(1)
    expect(second.sent[2].params?.textDocument?.text).toBe('new collaborative text')
    expect(LspSocket.peakLive).toBe(1)
  })

  it('rejects initialization queued in the same turn as destruction without leaving its timer', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const view = editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    socket.open()
    view.destroy()
    await flushLsp()
    expect(socket.sent).toEqual([])
    expect(socket.closeCalls).toBe(1)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('includes edits between plugin installation and didOpen, without an earlier didChange', async () => {
    const view = editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    socket.open()
    await flushLsp()
    const dispatch = view.dispatch.bind(view)
    vi.spyOn(view, 'dispatch').mockImplementationOnce((...args: Parameters<EditorView['dispatch']>) => {
      dispatch(...args)
      dispatch({ changes: { from: 0, to: view.state.doc.length, insert: 'latest' } })
    })
    socket.initialized()
    await flushLsp()
    expect(socket.sent.map(m => m.method)).toEqual(['initialize', 'initialized', 'textDocument/didOpen'])
    expect(socket.sent[2].params?.textDocument).toMatchObject({ text: 'latest', version: 0 })
    view.dispatch({ changes: { from: 0, insert: 'after open ' } })
    await flushLsp()
    expect(socket.sent.at(-1)?.params?.textDocument?.version).toBe(1)
  })

  it('does not install features when destroyed just after the initialize response', async () => {
    const view = editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    socket.open()
    await flushLsp()
    socket.initialized()
    view.destroy()
    await flushLsp()
    expect(socket.sent.map(m => m.method)).toEqual(['initialize'])
  })

  it('silences the upstream plugin didOpen continuation after detach', async () => {
    const view = editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    socket.open()
    await flushLsp()
    // Destroy synchronously after feature installation, before the upstream
    // plugin resumes its await initializePromise and sends didOpen.
    const dispatch = view.dispatch.bind(view)
    vi.spyOn(view, 'dispatch').mockImplementation((...args: Parameters<EditorView['dispatch']>) => {
      dispatch(...args)
      view.destroy()
    })
    socket.initialized()
    await flushLsp()
    expect(socket.sent.some(m => m.method === 'textDocument/didOpen')).toBe(false)
    expect(socket.closeCalls).toBe(1)
  })

  it.each(['close', 'error'])('leaves editing usable after capacity/connection %s, without retries', async event => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const view = editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    socket.dispatchEvent(new Event(event))
    await vi.advanceTimersByTimeAsync(120_000)
    view.dispatch({ changes: { from: 0, insert: 'still editable ' } })
    expect(view.state.doc.toString()).toBe('still editable initial')
    expect(LspSocket.instances).toHaveLength(1)
    expect(socket.closeCalls).toBe(1)
    expect(socket.sent).toEqual([])
  })

  it.each([false, true])('handles server disconnect after open (initialized=%s) without reconnecting', async initialized => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const view = editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    socket.open()
    await flushLsp()
    if (initialized) {
      socket.initialized()
      await flushLsp()
    }
    socket.close()
    await flushLsp()
    const sent = socket.sent.length
    view.dispatch({ changes: { from: 0, insert: 'offline ' } })
    await vi.advanceTimersByTimeAsync(120_000)
    expect(socket.sent).toHaveLength(sent)
    expect(LspSocket.instances).toHaveLength(1)
    expect(view.state.doc.toString()).toBe('offline initial')
  })

  it.each([false, true])('bounds %s-open connection/initialization waits and does not retry', async open => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    if (open) socket.open()
    await flushLsp()
    await vi.advanceTimersByTimeAsync(120_000)
    expect(socket.closeCalls).toBe(1)
    expect(LspSocket.instances).toHaveLength(1)
    expect(socket.sent.some(m => m.method === 'textDocument/didOpen')).toBe(false)
  })

  it('closes on failed initialization without opening a document', async () => {
    editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    socket.open()
    await flushLsp()
    socket.receive({ jsonrpc: '2.0', id: socket.sent[0].id, error: { code: -32000, message: 'unavailable' } })
    await flushLsp()
    expect(socket.closeCalls).toBe(1)
    expect(socket.sent.map(m => m.method)).toEqual(['initialize'])
  })
})

describe('bidirectional JSON-RPC', () => {
  it('handles server request ID 0 without resolving client initialize ID 0', async () => {
    editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    socket.open()
    await flushLsp()
    expect(socket.sent[0]).toMatchObject({ id: 0, method: 'initialize' })
    socket.receive({ jsonrpc: '2.0', id: 0, method: 'workspace/configuration', params: {
      items: [{ section: 'rust-analyzer' }, { scopeUri: config.documentUri, section: 'unknown' }],
    } })
    await flushLsp()
    expect(socket.sent).toHaveLength(2)
    expect(socket.sent[1]).toEqual({ jsonrpc: '2.0', id: 0, result: [null, null] })
    expect(socket.closeCalls).toBe(0)
    socket.initialized()
    await flushLsp()
    expect(socket.sent.filter(m => m.method).map(m => m.method)).toEqual(['initialize', 'initialized', 'textDocument/didOpen'])
  })

  it('advertises configuration and folders, but not unimplemented dynamic registration or progress', async () => {
    editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    await initialize(socket)
    const capabilities = socket.sent[0].params?.capabilities
    expect(capabilities).toMatchObject({
      workspace: { configuration: true, workspaceFolders: true, applyEdit: false, didChangeConfiguration: { dynamicRegistration: false } },
      window: { workDoneProgress: false },
    })
    for (const feature of ['hover', 'synchronization', 'completion', 'signatureHelp', 'declaration', 'definition', 'typeDefinition', 'implementation', 'documentHighlight', 'formatting', 'rangeFormatting', 'rename']) {
      expect(capabilities).toMatchObject({ textDocument: { [feature]: { dynamicRegistration: false } } })
    }
  })

  it('preserves a pending hover when a server request reuses its ID', async () => {
    const view = editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    await initialize(socket)
    const client = view.plugin(languageServerPlugin)!.client
    const pending = client.textDocumentHover({ textDocument: { uri: config.documentUri }, position: { line: 0, character: 0 } })
    const settled = vi.fn()
    void pending.then(settled)
    await flushLsp()
    const request = socket.sent.at(-1)!
    expect(request).toMatchObject({ method: 'textDocument/hover', id: 1 })
    socket.receive({ jsonrpc: '2.0', id: request.id, method: 'workspace/workspaceFolders' })
    await flushLsp()
    expect(settled).not.toHaveBeenCalled()
    expect(socket.sent.at(-1)).toEqual({ jsonrpc: '2.0', id: request.id, result: [{ uri: config.rootUri, name: config.rootUri }] })
    const hover = { contents: { kind: 'plaintext', value: 'actual hover response' } }
    socket.receive({ jsonrpc: '2.0', id: request.id, result: hover })
    await expect(pending).resolves.toEqual(hover)
  })

  it.each([0, 'server-0'])('answers message requests with ID %j exactly once', async id => {
    editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    await initialize(socket)
    const before = socket.sent.length
    socket.receive({ jsonrpc: '2.0', id, method: 'window/showMessageRequest', params: { type: 3, message: 'Choose', actions: [{ title: 'OK' }] } })
    await flushLsp()
    expect(socket.sent.slice(before)).toEqual([{ jsonrpc: '2.0', id, result: null }])
  })

  it.each([
    'client/registerCapability', 'client/unregisterCapability', 'workspace/applyEdit',
    'window/workDoneProgress/create', 'workspace/semanticTokens/refresh', 'unknown/request',
  ])('returns MethodNotFound for unadvertised/unsupported %s without consuming initialize', async method => {
    editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    socket.open()
    await flushLsp()
    socket.receive({ jsonrpc: '2.0', id: 0, method, params: {} })
    await flushLsp()
    expect(socket.sent).toHaveLength(2)
    expect(socket.sent[1]).toMatchObject({ jsonrpc: '2.0', id: 0, error: { code: -32601 } })
    expect(socket.sent[1]).not.toHaveProperty('result')
    socket.initialized()
    await flushLsp()
    expect(socket.sent.at(-1)?.method).toBe('textDocument/didOpen')
  })

  it.each([{}, { items: null }, { items: [null] }])('rejects malformed configuration params %j', async params => {
    editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    await initialize(socket)
    socket.receive({ jsonrpc: '2.0', id: 'config', method: 'workspace/configuration', params })
    await flushLsp()
    expect(socket.sent.at(-1)).toMatchObject({ id: 'config', error: { code: -32602 } })
  })

  it('does not answer server requests after teardown', async () => {
    const view = editor(lspExtension(config))
    await flushLsp()
    const socket = LspSocket.instances[0]
    await initialize(socket)
    view.destroy()
    const count = socket.sent.length
    socket.receive({ jsonrpc: '2.0', id: 0, method: 'workspace/workspaceFolders' })
    await flushLsp()
    expect(socket.sent).toHaveLength(count)
  })
})
