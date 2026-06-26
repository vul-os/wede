// useCollab — connects to a room's collaboration WebSocket for presence.
//
// Opens /api/rooms/{id}/collab (auth via ?token=, matching useLSP and the auth
// middleware which reads ?token= on WS upgrades), parses {type:'presence',
// members:[...]} roster broadcasts, and exposes setViewing(file, line) to publish
// the local cursor as {type:'cursor', file, line} (throttled). The CRDT document
// channel layers onto this same socket in a later wave.
//
// Defensive by design: any failure leaves collab simply inactive (empty roster),
// never throwing into the IDE.

import { useEffect, useRef, useState, useCallback } from 'react'

function buildWsUrl(roomId, token, username) {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  // In dev (Vite on 5173/5174) the WS must hit the backend on :9090 directly.
  const port = window.location.port
  const host = (port === '5173' || port === '5174')
    ? window.location.hostname + ':9090'
    : window.location.host
  return `${proto}//${host}/api/rooms/${encodeURIComponent(roomId)}/collab`
    + `?token=${encodeURIComponent(token)}`
    + `&username=${encodeURIComponent(username || '')}`
}

const THROTTLE_MS = 150

export function useCollab(roomId, token, username) {
  const [roster, setRoster] = useState([])
  const wsRef = useRef(null)
  const lastSentRef = useRef({ file: null, line: -1 })
  const pendingRef = useRef(null)
  const timerRef = useRef(null)

  useEffect(() => {
    if (!roomId || !token) return undefined
    let disposed = false
    let ws
    try {
      ws = new WebSocket(buildWsUrl(roomId, token, username))
      wsRef.current = ws
      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data)
          if (msg && msg.type === 'presence' && Array.isArray(msg.members)) {
            setRoster(msg.members)
          }
        } catch { /* ignore malformed frames */ }
      }
      ws.onclose = () => {
        if (wsRef.current === ws) wsRef.current = null
        if (!disposed) setRoster([])
      }
      ws.onerror = () => { /* non-fatal; onclose handles cleanup */ }
    } catch { /* construction failed → collab inactive */ }

    return () => {
      disposed = true
      setRoster([])
      lastSentRef.current = { file: null, line: -1 }
      if (timerRef.current) { clearTimeout(timerRef.current); timerRef.current = null }
      try { if (ws) ws.close() } catch { /* ignore */ }
      if (wsRef.current === ws) wsRef.current = null
    }
  }, [roomId, token, username])

  // setViewing publishes the local cursor, throttled (leading + trailing) so
  // rapid tab/cursor changes don't flood the socket.
  const setViewing = useCallback((file, line = 0) => {
    pendingRef.current = { file: file || '', line: line || 0 }

    const flush = () => {
      timerRef.current = null
      const p = pendingRef.current
      const ws = wsRef.current
      if (!p || !ws || ws.readyState !== WebSocket.OPEN) return
      if (p.file === lastSentRef.current.file && p.line === lastSentRef.current.line) return
      lastSentRef.current = p
      try { ws.send(JSON.stringify({ type: 'cursor', file: p.file, line: p.line })) } catch { /* ignore */ }
    }

    if (timerRef.current) return // trailing edge already scheduled
    flush()                       // leading edge
    timerRef.current = setTimeout(flush, THROTTLE_MS)
  }, [])

  return { roster, setViewing }
}
