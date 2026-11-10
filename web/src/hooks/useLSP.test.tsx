// @vitest-environment jsdom
import { commit } from '../test/react'
import { act, StrictMode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import * as Y from 'yjs'
import { Awareness } from 'y-protocols/awareness'
import { useLSP, type UseLSPParams } from './useLSP'
import Editor from '../components/Editor'
import LspStatusIndicator from '../components/LspStatusIndicator'
import type { UseYDocState } from './useYDoc'
import { LspSocket, flushLsp } from '../test/lspSocket'

vi.mock('./useTheme', () => ({ useTheme: () => ({ isDark: true }) }))
const workspaces = [{ id: 'one', name: 'one', root: '/projects/one' }, { id: 'two', name: 'two', root: '/projects/two' }]
const file = { path: 'one:src/main.rs', rel: 'src/main.rs', workspaceId: 'one', name: 'main.rs' }
const authFetch = vi.fn(() => Promise.resolve(new Response(JSON.stringify({ available: { rust: true } }))))
const defaults: UseLSPParams = { file, workspaces, authFetch, token: 'token', lspEnabled: true }
const noop = () => {}
let root: Root
let container: HTMLDivElement

function Harness({ params = defaults, content = 'fn main() {}', collab = null, showEditor = true }: {
  params?: UseLSPParams
  content?: string
  collab?: UseYDocState | null
  showEditor?: boolean
}) {
  const { extension, status } = useLSP(params)
  return <>
    {showEditor && <Editor file={params.file ?? null} content={content} onChange={noop} onSave={noop} lspExtension={extension} collab={collab} />}
    <LspStatusIndicator status={status} />
  </>
}

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('WebSocket', LspSocket)
  LspSocket.instances = []
  LspSocket.peakLive = 0
  // jsdom has no layout; supply only the Range geometry CodeMirror measures.
  const createRange = document.createRange.bind(document)
  vi.spyOn(document, 'createRange').mockImplementation(() => {
    const range = createRange()
    range.getClientRects = () => Object.assign([], { item: () => null })
    range.getBoundingClientRect = () => new DOMRect()
    return range
  })
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})
afterEach(async () => {
  await commit(() => root.unmount())
  container.remove()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it('shows capacity failure in the status bar without reconnecting and isolates the next workspace', async () => {
  await commit(() => root.render(<Harness />))
  expect(container.querySelector('[role="status"]')?.textContent).toBe('LSP connecting…')
  const first = LspSocket.instances[0]
  await act(async () => {
    first.open()
    await flushLsp()
    first.receive({ jsonrpc: '2.0', method: 'window/showMessage', params: { type: 1, message: 'wede: failed to start language server: session limit reached' } })
    first.close()
    await flushLsp()
  })
  expect(container.querySelector('[role="status"]')?.textContent).toBe('LSP unavailable')
  expect(container.querySelector('[role="status"]')?.getAttribute('title')).toContain('session limit reached')
  expect(container.querySelector('[role="status"]')?.getAttribute('title')).toContain('No automatic retry')
  await commit(() => root.render(<Harness content="still editable" />))
  expect(LspSocket.instances).toHaveLength(1)
  const next = { ...defaults, file: { ...file, workspaceId: 'two', path: 'two:src/main.rs' } }
  await commit(() => root.render(<Harness params={next} />))
  expect(container.querySelector('[role="status"]')?.textContent).toBe('LSP connecting…')
  await act(async () => {
    first.receive({ jsonrpc: '2.0', method: 'window/showMessage', params: { type: 1, message: 'stale failure' } })
    const second = LspSocket.instances[1]
    second.open()
    await flushLsp()
    second.initialized()
    await flushLsp()
  })
  expect(container.querySelector('[role="status"]')?.textContent).toBe('LSP ready')
  await commit(() => root.render(<Harness params={{ ...next, lspEnabled: false }} />))
  expect(container.querySelector('[role="status"]')).toBeNull()
})

it('shows a generic connection failure when the server supplies no reason', async () => {
  await commit(() => root.render(<Harness />))
  await commit(() => LspSocket.instances[0].dispatchEvent(new Event('error')))
  expect(container.querySelector('[role="status"]')?.textContent).toBe('LSP unavailable')
  expect(container.querySelector('[role="status"]')?.getAttribute('title')).toContain('connection failed')
})

it('opens nothing when the hook renders without an editor', async () => {
  await commit(() => root.render(<Harness showEditor={false} />))
  expect(LspSocket.instances).toHaveLength(0)
})

it('survives StrictMode, content renders, file switches, and disabling LSP with one owned socket', async () => {
  await commit(() => root.render(<StrictMode><Harness /></StrictMode>))
  expect(LspSocket.instances).toHaveLength(1)
  const first = LspSocket.instances[0]
  await commit(() => root.render(<StrictMode><Harness content="changed" /></StrictMode>))
  expect(LspSocket.instances).toHaveLength(1)
  const next = { ...defaults, file: { ...file, path: 'one:other.rs', rel: 'other.rs' } }
  await commit(() => root.render(<StrictMode><Harness params={next} /></StrictMode>))
  expect(first.closeCalls).toBe(1)
  expect(LspSocket.instances).toHaveLength(2)
  expect(LspSocket.peakLive).toBe(1)
  await commit(() => root.render(<StrictMode><Harness params={{ ...next, lspEnabled: false }} /></StrictMode>))
  expect(LspSocket.instances[1].closeCalls).toBe(1)
  expect(LspSocket.instances).toHaveLength(2)
})

it('uses the tab workspace root and endpoint together, not the globally focused workspace', async () => {
  await commit(() => root.render(<Harness />))
  const next = { ...defaults, file: { ...file, path: 'two:src/a #%.rs', workspaceId: 'two', rel: 'src/a #%.rs' } }
  await commit(() => root.render(<Harness params={next} />))
  const socket = LspSocket.instances[1]
  expect(new URL(socket.url).pathname).toBe('/api/workspaces/two/lsp')
  await act(async () => {
    socket.open()
    await flushLsp()
    socket.initialized()
    await flushLsp()
  })
  expect(socket.sent[0].params?.rootUri).toBe('file:///projects/two')
  expect(socket.sent[2].params?.textDocument?.uri).toBe('file:///projects/two/src/a%20%23%25.rs')
  expect(LspSocket.instances[0].closeCalls).toBe(1)
  expect(LspSocket.peakLive).toBe(1)
})

it.each([
  { file: null },
  { token: null },
  { workspaces: [] },
  { file: { ...file, type: 'browser' } },
  { file: { ...file, fileType: 'binary' } },
  { file: { ...file, fileType: 'image' } },
  { file: { ...file, rel: 'readme.txt' } },
])('does not connect without an eligible document: %j', async override => {
  await commit(() => root.render(<Harness params={{ ...defaults, ...override }} />))
  expect(LspSocket.instances).toHaveLength(0)
})

it('does not reconnect on equivalent availability refreshes and closes when its workspace disappears', async () => {
  await commit(() => root.render(<Harness />))
  const refreshedFetch = () => authFetch()
  await commit(() => root.render(<Harness params={{ ...defaults, authFetch: refreshedFetch }} />))
  expect(LspSocket.instances).toHaveLength(1)
  await commit(() => root.render(<Harness params={{ ...defaults, workspaces: [] }} />))
  expect(LspSocket.instances[0].closeCalls).toBe(1)
})

it.each([
  { token: 'rotated-token' },
  { workspaces: [{ ...workspaces[0], root: '/moved/root' }] },
])('replaces the session when credentials or the root change without a file switch: %j', async override => {
  await commit(() => root.render(<Harness />))
  await commit(() => root.render(<Harness params={{ ...defaults, ...override }} />))
  expect(LspSocket.instances).toHaveLength(2)
  expect(LspSocket.instances[0].closeCalls).toBe(1)
  expect(LspSocket.peakLive).toBe(1)
})

it('creates a fresh client when collaborative Y.Text rebuilds the actual Editor', async () => {
  await commit(() => root.render(<Harness />))
  const first = LspSocket.instances[0]
  const doc = new Y.Doc()
  const awareness = new Awareness(doc)
  const ytext = doc.getText('content')
  ytext.insert(0, 'collaborative text')
  try {
    await commit(() => root.render(<Harness collab={{ ytext, awareness, provider: null }} />))
    expect(first.closeCalls).toBe(1)
    expect(LspSocket.instances).toHaveLength(2)
    const second = LspSocket.instances[1]
    await act(async () => {
      second.open()
      await flushLsp()
      second.initialized()
      await flushLsp()
    })
    expect(second.sent[2].params?.textDocument?.text).toBe('collaborative text')
    // Yjs synchronization is not an acknowledgement of the backend disk write.
    expect(second.sent.some(m => m.method === 'textDocument/didSave')).toBe(false)
    expect(LspSocket.peakLive).toBe(1)
    await commit(() => root.render(<Harness showEditor={false} />))
    expect(second.closeCalls).toBe(1)
  } finally {
    awareness.destroy()
    doc.destroy()
  }
})
