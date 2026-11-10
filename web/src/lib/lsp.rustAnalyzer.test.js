// @vitest-environment jsdom
// Opt in with WEDE_TEST_RUST_ANALYZER=/absolute/path/to/rust-analyzer.
// A real stdio server behind a WebSocket-shaped test bridge; no backend needed.
import { Buffer } from 'node:buffer'
import { spawn } from 'node:child_process'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { env, kill, platform } from 'node:process'
import { pathToFileURL } from 'node:url'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { languageServerPlugin } from 'codemirror-languageserver'
import { expect, it, vi } from 'vitest'
import { lspExtension } from './lsp'
import { createDocumentSaves, writeDocument } from './documentSaves'


const analyzer = env.WEDE_TEST_RUST_ANALYZER

it.skipIf(!analyzer).each([false, true])('real rust-analyzer initialization/configuration (save diagnostics=%s)', async checkSave => {
  const directory = mkdtempSync(join(tmpdir(), 'wede-lsp-'))
  const rootUri = pathToFileURL(directory).href
  const documentUri = pathToFileURL(join(directory, 'main.rs')).href
  const initial = 'fn main() { let _value: u32 = 0; }\n'
  writeFileSync(join(directory, 'main.rs'), initial)
  if (checkSave) {
    // A tiny, dependency-free throwaway crate; never touch a user's project.
    writeFileSync(join(directory, 'Cargo.toml'), '[package]\nname = "wede_lsp_save_test"\nversion = "0.1.0"\nedition = "2021"\n[[bin]]\nname = "test"\npath = "main.rs"\n')
  }
  const saves = createDocumentSaves()
  const sockets = []
  class AnalyzerSocket extends EventTarget {
    static OPEN = 1
    readyState = 0
    sent = []
    received = []
    stderr = ''
    child = spawn(analyzer, [], {
      cwd: directory, detached: platform !== 'win32', stdio: 'pipe',
      env: { ...env, RAYON_NUM_THREADS: '2', CARGO_BUILD_JOBS: '1', CARGO_NET_OFFLINE: 'true' },
    })

    constructor() {
      super()
      sockets.push(this)
      this.exited = new Promise(resolve => this.child.once('close', () => {
        this.readyState = 3
        this.dispatchEvent(new Event('close'))
        resolve()
      }))
      this.child.on('error', () => this.dispatchEvent(new Event('error')))
      this.child.stdin.on('error', () => this.dispatchEvent(new Event('error')))
      this.child.stderr.on('data', chunk => { this.stderr += chunk.toString() })
      let buffer = Buffer.alloc(0)
      this.child.stdout.on('data', chunk => {
        buffer = Buffer.concat([buffer, chunk])
        for (;;) {
          const headerEnd = buffer.indexOf('\r\n\r\n')
          if (headerEnd < 0) return
          const length = Number(/Content-Length: (\d+)/i.exec(buffer.subarray(0, headerEnd).toString())?.[1])
          if (!Number.isFinite(length)) throw new Error('Invalid LSP frame')
          const end = headerEnd + 4 + length
          if (buffer.length < end) return
          const raw = buffer.subarray(headerEnd + 4, end).toString()
          buffer = buffer.subarray(end)
          this.received.push(JSON.parse(raw))
          this.dispatchEvent(new MessageEvent('message', { data: raw }))
        }
      })
      this.child.once('spawn', () => {
        this.readyState = 1
        this.dispatchEvent(new Event('open'))
      })
    }

    send(raw) {
      this.sent.push(JSON.parse(raw))
      this.child.stdin.write(`Content-Length: ${Buffer.byteLength(raw)}\r\n\r\n${raw}`)
    }

    close() {
      if (this.readyState === 3) return
      this.readyState = 3
      try {
        if (platform !== 'win32' && this.child.pid) kill(-this.child.pid, 'SIGKILL')
        else this.child.kill('SIGKILL')
      } catch { /* process already exited */ }
    }
  }

  vi.stubGlobal('WebSocket', AnalyzerSocket)
  const view = new EditorView({
    state: EditorState.create({ doc: initial, extensions: lspExtension({
      rootUri, documentUri, languageId: 'rust', serverUri: 'ws://test/rust-analyzer',
      persistence: { events: saves, workspaceId: 'test', path: 'main.rs' },
    }) }),
    parent: document.body,
  })
  try {
    await vi.waitFor(() => expect(view.plugin(languageServerPlugin)?.client.ready).toBe(true), { timeout: 10_000 })
    const socket = sockets[0]
    expect(socket.sent[0].params?.capabilities).toMatchObject({ workspace: { configuration: true, workspaceFolders: true } })
    // Request a settings refresh through the real server's notification handler.
    // This must produce an actual workspace/configuration request and array reply.
    socket.send(JSON.stringify({ jsonrpc: '2.0', method: 'workspace/didChangeConfiguration', params: { settings: null } }))
    await vi.waitFor(() => {
      const requests = socket.received.filter(m => m.method === 'workspace/configuration')
      expect(requests.length, socket.stderr).toBeGreaterThan(0)
      for (const request of requests) {
        expect(request.params).toMatchObject({ items: expect.arrayContaining([{ section: 'rust-analyzer' }]) })
        const reply = socket.sent.find(m => m.method === undefined && m.id === request.id)
        expect(reply).toMatchObject({ result: [null] })
        expect(reply).not.toHaveProperty('error')
      }
    }, { timeout: 10_000 })
    view.dispatch({ changes: { from: 0, insert: '// edit\n' } })
    view.dispatch({ changes: { from: 0, insert: '// another edit\n' } })
    await vi.waitFor(() => {
      const documents = socket.sent.filter(m => m.method === 'textDocument/didOpen' || m.method === 'textDocument/didChange')
      expect(documents.map(m => m.params?.textDocument?.version)).toEqual([0, 1, 2])
    })
    // A round trip after configuration confirms the real server accepted our
    // reply and is still processing requests, not just that we wrote valid JSON.
    const hover = await view.plugin(languageServerPlugin).client.textDocumentHover({
      textDocument: { uri: documentUri }, position: { line: 2, character: 3 },
    })
    expect(hover === null || typeof hover === 'object').toBe(true)
    expect(socket.received.some(m => m.method === 'client/registerCapability')).toBe(false)
    expect(socket.readyState).toBe(1)
    if (checkSave) {
      const client = view.plugin(languageServerPlugin).client
      await vi.waitFor(async () => {
        const loaded = await client.textDocumentHover({ textDocument: { uri: documentUri }, position: { line: 2, character: 4 } })
        expect(loaded).not.toBeNull()
      }, { timeout: 20_000 })
      const mismatch = 'fn main() { let _value: u32 = "wrong"; }\n'
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: mismatch } })
      const mismatches = () => socket.received.filter(m => m.method === 'textDocument/publishDiagnostics' && m.params?.uri === documentUri)
        .flatMap(m => m.params.diagnostics).filter(d => d.code === 'E0308')
      await new Promise(resolve => setTimeout(resolve, 250))
      expect(mismatches()).toEqual([])
      expect(socket.sent.some(m => m.method === 'textDocument/didSave')).toBe(false)
      // Disk write and HTTP acknowledgement take the same path as the IDE's
      // manual/auto saves. Delaying the acknowledgement must delay didSave.
      let acknowledge
      const writing = writeDocument(() => {
        writeFileSync(join(directory, 'main.rs'), mismatch)
        return new Promise(resolve => { acknowledge = resolve })
      }, saves, 'test', 'main.rs', mismatch)
      expect(socket.sent.some(m => m.method === 'textDocument/didSave')).toBe(false)
      acknowledge(new Response(JSON.stringify({ status: 'ok' })))
      await writing
      await vi.waitFor(() => expect(socket.sent.some(m => m.method === 'textDocument/didSave')).toBe(true))
      await vi.waitFor(() => expect(mismatches().length, socket.stderr).toBeGreaterThan(0), { timeout: 20_000 })
    }
  } finally {
    view.destroy()
    for (const socket of sockets) socket.close()
    await Promise.all(sockets.map(socket => socket.exited))
    vi.unstubAllGlobals()
    rmSync(directory, { recursive: true, force: true })
  }
}, 60_000)
