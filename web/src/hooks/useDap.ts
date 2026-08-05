// useDap — a Debug Adapter Protocol client over the workspace's /dap WebSocket.
//
// Drives the standard launch handshake (initialize → launch → on `initialized`
// set breakpoints + configurationDone), tracks the stopped state (call stack,
// scopes/variables, current line), streams debug output, and exposes stepping
// actions. Defensive: any failure leaves the session idle rather than throwing.

import { useRef, useState, useCallback } from 'react'

export type DapStatus = 'idle' | 'starting' | 'running' | 'stopped' | 'terminated'

export interface DapSource {
  name?: string
  path?: string
}

export interface DapFrame {
  id: number
  name: string
  line: number
  source?: DapSource
}

export interface DapVariable {
  name: string
  value: string
  type?: string
}

export interface DapScope {
  name: string
  variables: DapVariable[]
  expensive?: boolean
  variablesReference?: number
}

export interface DapStopLine {
  path: string
  line: number
}

// A DAP protocol message — either an outgoing "request", or an incoming
// "response"/"event" frame from the adapter. Bodies vary per command/event, so
// they're kept loosely typed (indexed by the call sites that read them).
interface DapMessage {
  seq: number
  type: 'request' | 'response' | 'event'
  command?: string
  event?: string
  arguments?: Record<string, unknown>
  request_seq?: number
  body?: Record<string, unknown>
}

export interface DapStartOptions {
  program: string
  lang: string
  breakpoints?: Record<string, number[]>
  args?: string[]
}

function buildWsUrl(workspaceId: string | null | undefined, token: string | null | undefined, lang: string): string {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const port = window.location.port
  const host = (port === '5173' || port === '5174') ? window.location.hostname + ':9090' : window.location.host
  // String(...) matches the implicit ToString coercion encodeURIComponent would
  // have applied to a null/undefined workspaceId/token in the untyped original.
  return `${proto}//${host}/api/workspaces/${encodeURIComponent(String(workspaceId))}/dap`
    + `?lang=${encodeURIComponent(lang)}&token=${encodeURIComponent(String(token))}`
}

export interface UseDapParams {
  workspaceId: string | null | undefined
  token: string | null | undefined
}

export function useDap({ workspaceId, token }: UseDapParams) {
  const [status, setStatus] = useState<DapStatus>('idle')   // idle | starting | running | stopped | terminated
  const [frames, setFrames] = useState<DapFrame[]>([])         // call stack (current thread)
  const [scopes, setScopes] = useState<DapScope[]>([])         // [{ name, variables: [{name,value,type}] }]
  const [output, setOutput] = useState<string[]>([])         // console lines
  const [stopLine, setStopLine] = useState<DapStopLine | null>(null)   // { path, line } — for the editor marker

  const wsRef = useRef<WebSocket | null>(null)
  const seqRef = useRef(1)
  const pendingRef = useRef<Record<number, (msg: DapMessage) => void>>({})
  const cfgRef = useRef<{ breakpoints: Record<string, number[]> }>({ breakpoints: {} })

  const send = useCallback((command: string, args?: Record<string, unknown>): Promise<DapMessage | null> => {
    const ws = wsRef.current
    if (!ws || ws.readyState !== WebSocket.OPEN) return Promise.resolve(null)
    const seq = seqRef.current++
    ws.send(JSON.stringify({ seq, type: 'request', command, arguments: args || {} }))
    return new Promise((resolve) => { pendingRef.current[seq] = resolve })
  }, [])

  const loadStopState = useCallback(async (threadId: number) => {
    const st = await send('stackTrace', { threadId, startFrame: 0, levels: 20 })
    const fr = (st?.body?.stackFrames as DapFrame[] | undefined) || []
    setFrames(fr)
    if (!fr[0]) return
    setStopLine({ path: fr[0].source?.path || '', line: fr[0].line })
    const sc = await send('scopes', { frameId: fr[0].id })
    const out: DapScope[] = []
    for (const s of ((sc?.body?.scopes as DapScope[] | undefined) || []).slice(0, 4)) {
      if (s.expensive) { out.push({ name: s.name, variables: [] }); continue }
      const v = await send('variables', { variablesReference: s.variablesReference })
      const vars = (v?.body?.variables as DapVariable[] | undefined) || []
      out.push({ name: s.name, variables: vars.map((x) => ({ name: x.name, value: x.value, type: x.type })) })
    }
    setScopes(out)
  }, [send])

  const handleEvent = useCallback(async (msg: DapMessage) => {
    switch (msg.event) {
      case 'output':
        setOutput((o) => [...o.slice(-400), (msg.body?.output as string | undefined) || ''])
        break
      case 'stopped':
        setStatus('stopped')
        await loadStopState(msg.body?.threadId as number)
        break
      case 'continued':
        setStatus('running'); setStopLine(null); setFrames([]); setScopes([])
        break
      case 'terminated':
      case 'exited':
        setStatus('terminated'); setStopLine(null); setFrames([]); setScopes([])
        break
      case 'initialized': {
        const bps = cfgRef.current.breakpoints || {}
        for (const [path, lines] of Object.entries(bps)) {
          await send('setBreakpoints', { source: { path }, breakpoints: (lines || []).map((l) => ({ line: l })) })
        }
        await send('configurationDone', {})
        break
      }
      default:
        break
    }
  }, [send, loadStopState])

  const stop = useCallback(() => {
    const ws = wsRef.current
    if (ws) {
      try { if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ seq: seqRef.current++, type: 'request', command: 'disconnect', arguments: { terminateDebuggee: true } })) } catch { /* ignore */ }
      try { ws.close() } catch { /* ignore */ }
    }
    wsRef.current = null
    pendingRef.current = {}
  }, [])

  const start = useCallback(({ program, lang, breakpoints, args }: DapStartOptions) => {
    stop()
    cfgRef.current = { breakpoints: breakpoints || {} }
    setStatus('starting'); setOutput([]); setFrames([]); setScopes([]); setStopLine(null)
    let ws: WebSocket
    try { ws = new WebSocket(buildWsUrl(workspaceId, token, lang)) } catch { setStatus('idle'); return }
    wsRef.current = ws
    ws.onopen = async () => {
      const init = await send('initialize', {
        clientID: 'wede', adapterID: lang, locale: 'en',
        linesStartAt1: true, columnsStartAt1: true, pathFormat: 'path',
        supportsRunInTerminalRequest: false,
      })
      if (init === null) return
      await send('launch', { request: 'launch', name: 'wede', type: lang, mode: 'debug', program, args: args || [], stopOnEntry: false })
      setStatus((s) => (s === 'starting' ? 'running' : s))
    }
    ws.onmessage = (e: MessageEvent) => {
      let msg: DapMessage
      try { msg = JSON.parse(e.data) } catch { return }
      if (msg.type === 'response') {
        const fn = msg.request_seq !== undefined ? pendingRef.current[msg.request_seq] : undefined
        if (fn && msg.request_seq !== undefined) { delete pendingRef.current[msg.request_seq]; fn(msg) }
      } else if (msg.type === 'event') {
        // handleEvent has no realistic rejection path today (every field read
        // below it is optional-chained or `|| []`-guarded), but this is a raw
        // WS message handler receiving server-controlled DAP frames cast with
        // `as` rather than validated — if a future shape assumption ever does
        // throw, surface it instead of letting it vanish as a silent rejection.
        handleEvent(msg).catch((err: unknown) => { console.error('[dap] event handler failed', err) })
      }
    }
    ws.onclose = () => { if (wsRef.current === ws) { wsRef.current = null; setStatus((s) => (s === 'idle' ? 'idle' : 'terminated')) } }
    ws.onerror = () => { /* onclose handles it */ }
  }, [workspaceId, token, send, handleEvent, stop])

  // Stepping — DAP needs a threadId; the top stack frame's thread is implicit in
  // most adapters, so we request threads lazily when needed.
  const withThread = useCallback(async (fn: (threadId: number) => Promise<unknown>) => {
    const t = await send('threads', {})
    const threads = t?.body?.threads as { id: number }[] | undefined
    const threadId = threads?.[0]?.id ?? 1
    setStatus('running'); setStopLine(null)
    await fn(threadId)
  }, [send])

  const cont    = useCallback(() => withThread((id) => send('continue', { threadId: id })), [withThread, send])
  const stepOver = useCallback(() => withThread((id) => send('next', { threadId: id })), [withThread, send])
  const stepIn  = useCallback(() => withThread((id) => send('stepIn', { threadId: id })), [withThread, send])
  const stepOut = useCallback(() => withThread((id) => send('stepOut', { threadId: id })), [withThread, send])

  // Push a breakpoint update mid-session (also updates the config for next launch).
  const syncBreakpoints = useCallback((path: string, lines: number[]) => {
    cfgRef.current.breakpoints = { ...cfgRef.current.breakpoints, [path]: lines }
    // Fire-and-forget: callers don't wait on the ack, and send()'s Promise
    // executor (line 94) only ever resolves, never rejects.
    void send('setBreakpoints', { source: { path }, breakpoints: (lines || []).map((l) => ({ line: l })) })
  }, [send])

  return { status, frames, scopes, output, stopLine, start, stop, cont, stepOver, stepIn, stepOut, syncBreakpoints }
}
