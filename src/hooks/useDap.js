// useDap — a Debug Adapter Protocol client over the workspace's /dap WebSocket.
//
// Drives the standard launch handshake (initialize → launch → on `initialized`
// set breakpoints + configurationDone), tracks the stopped state (call stack,
// scopes/variables, current line), streams debug output, and exposes stepping
// actions. Defensive: any failure leaves the session idle rather than throwing.

import { useRef, useState, useCallback } from 'react'

function buildWsUrl(workspaceId, token, lang) {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const port = window.location.port
  const host = (port === '5173' || port === '5174') ? window.location.hostname + ':9090' : window.location.host
  return `${proto}//${host}/api/workspaces/${encodeURIComponent(workspaceId)}/dap`
    + `?lang=${encodeURIComponent(lang)}&token=${encodeURIComponent(token)}`
}

export function useDap({ workspaceId, token }) {
  const [status, setStatus] = useState('idle')   // idle | starting | running | stopped | terminated
  const [frames, setFrames] = useState([])         // call stack (current thread)
  const [scopes, setScopes] = useState([])         // [{ name, variables: [{name,value,type}] }]
  const [output, setOutput] = useState([])         // console lines
  const [stopLine, setStopLine] = useState(null)   // { path, line } — for the editor marker

  const wsRef = useRef(null)
  const seqRef = useRef(1)
  const pendingRef = useRef({})
  const cfgRef = useRef({ breakpoints: {} })

  const send = useCallback((command, args) => {
    const ws = wsRef.current
    if (!ws || ws.readyState !== WebSocket.OPEN) return Promise.resolve(null)
    const seq = seqRef.current++
    ws.send(JSON.stringify({ seq, type: 'request', command, arguments: args || {} }))
    return new Promise((resolve) => { pendingRef.current[seq] = resolve })
  }, [])

  const loadStopState = useCallback(async (threadId) => {
    const st = await send('stackTrace', { threadId, startFrame: 0, levels: 20 })
    const fr = st?.body?.stackFrames || []
    setFrames(fr)
    if (!fr[0]) return
    setStopLine({ path: fr[0].source?.path || '', line: fr[0].line })
    const sc = await send('scopes', { frameId: fr[0].id })
    const out = []
    for (const s of (sc?.body?.scopes || []).slice(0, 4)) {
      if (s.expensive) { out.push({ name: s.name, variables: [] }); continue }
      const v = await send('variables', { variablesReference: s.variablesReference })
      out.push({ name: s.name, variables: (v?.body?.variables || []).map((x) => ({ name: x.name, value: x.value, type: x.type })) })
    }
    setScopes(out)
  }, [send])

  const handleEvent = useCallback(async (msg) => {
    switch (msg.event) {
      case 'output':
        setOutput((o) => [...o.slice(-400), msg.body?.output || ''])
        break
      case 'stopped':
        setStatus('stopped')
        await loadStopState(msg.body?.threadId)
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

  const start = useCallback(({ program, lang, breakpoints, args }) => {
    stop()
    cfgRef.current = { breakpoints: breakpoints || {} }
    setStatus('starting'); setOutput([]); setFrames([]); setScopes([]); setStopLine(null)
    let ws
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
    ws.onmessage = (e) => {
      let msg
      try { msg = JSON.parse(e.data) } catch { return }
      if (msg.type === 'response') {
        const fn = pendingRef.current[msg.request_seq]
        if (fn) { delete pendingRef.current[msg.request_seq]; fn(msg) }
      } else if (msg.type === 'event') {
        handleEvent(msg)
      }
    }
    ws.onclose = () => { if (wsRef.current === ws) { wsRef.current = null; setStatus((s) => (s === 'idle' ? 'idle' : 'terminated')) } }
    ws.onerror = () => { /* onclose handles it */ }
  }, [workspaceId, token, send, handleEvent, stop])

  // Stepping — DAP needs a threadId; the top stack frame's thread is implicit in
  // most adapters, so we request threads lazily when needed.
  const withThread = useCallback(async (fn) => {
    const t = await send('threads', {})
    const threadId = t?.body?.threads?.[0]?.id ?? 1
    setStatus('running'); setStopLine(null)
    await fn(threadId)
  }, [send])

  const cont    = useCallback(() => withThread((id) => send('continue', { threadId: id })), [withThread, send])
  const stepOver = useCallback(() => withThread((id) => send('next', { threadId: id })), [withThread, send])
  const stepIn  = useCallback(() => withThread((id) => send('stepIn', { threadId: id })), [withThread, send])
  const stepOut = useCallback(() => withThread((id) => send('stepOut', { threadId: id })), [withThread, send])

  // Push a breakpoint update mid-session (also updates the config for next launch).
  const syncBreakpoints = useCallback((path, lines) => {
    cfgRef.current.breakpoints = { ...cfgRef.current.breakpoints, [path]: lines }
    send('setBreakpoints', { source: { path }, breakpoints: (lines || []).map((l) => ({ line: l })) })
  }, [send])

  return { status, frames, scopes, output, stopLine, start, stop, cont, stepOver, stepIn, stepOut, syncBreakpoints }
}
