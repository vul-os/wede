// Deterministic browser transport; tests still use the real language-server
// package, CodeMirror plugins, React effects, and Yjs documents.
export interface RpcMessage {
  jsonrpc?: string
  id?: number | string
  method?: string
  result?: unknown
  error?: { code: number; message: string }
  params?: {
    capabilities?: unknown
    rootUri?: string
    workspaceFolders?: { uri: string }[]
    textDocument?: { uri: string; text?: string; version?: number }
    contentChanges?: { text: string }[]
    text?: string
  }
}

export class LspSocket extends EventTarget {
  static OPEN = 1
  static instances: LspSocket[] = []
  static peakLive = 0
  readyState = 0
  sent: RpcMessage[] = []
  closeCalls = 0

  constructor(readonly url: string) {
    super()
    LspSocket.instances.push(this)
    LspSocket.peakLive = Math.max(LspSocket.peakLive, LspSocket.instances.filter(s => s.readyState < 2).length)
  }

  open() {
    this.readyState = 1
    this.dispatchEvent(new Event('open'))
  }

  send(raw: string) {
    if (this.readyState !== 1) throw new Error('send on closed socket')
    this.sent.push(JSON.parse(raw) as RpcMessage)
  }

  receive(message: unknown) {
    this.dispatchEvent(new MessageEvent('message', { data: JSON.stringify(message) }))
  }

  initialized(capabilities: unknown = {}) {
    const request = this.sent.find(m => m.method === 'initialize')
    if (!request) throw new Error('No initialize request')
    this.receive({ jsonrpc: '2.0', id: request.id, result: { capabilities } })
  }

  close() {
    this.closeCalls++
    if (this.readyState === 3) return
    this.readyState = 3
    this.dispatchEvent(new Event('close'))
  }
}

export async function flushLsp() {
  // Drain initialization, feature installation, and the plugin's didOpen await.
  for (let i = 0; i < 12; i++) await Promise.resolve()
}
