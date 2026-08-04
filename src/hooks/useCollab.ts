// useCollab — connects to a workspace's collaboration WebSocket for presence
// plus ephemeral peer signals (live mouse cursors + shared window geometry).
//
// Opens /api/workspaces/{id}/collab (auth via ?token=, matching useLSP and the auth
// middleware which reads ?token= on WS upgrades), parses {type:'presence',
// members:[...]} roster broadcasts, and exposes:
//   setViewing(file, line)      publish the local text cursor    (throttled)
//   sendMouse(fx, fy)           publish local mouse as viewport fractions (throttled)
//   sendWindow(win, geo)        publish a shared window's geometry
//   mice                        { id: {x, y} }  other members' mouse fractions
//   onWindow(cb)                subscribe to peer window-geometry updates
//
// Defensive by design: any failure leaves collab simply inactive (empty roster),
// never throwing into the IDE.

import { useEffect, useRef, useState, useCallback } from 'react'
import type { PresenceMember, MousePosition, TerminalSession } from '../types'

function buildWsUrl(workspaceId: string, token: string, username: string | null | undefined): string {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  // In dev (Vite on 5173/5174) the WS must hit the backend on :9090 directly.
  const port = window.location.port
  const host = (port === '5173' || port === '5174')
    ? window.location.hostname + ':9090'
    : window.location.host
  return `${proto}//${host}/api/workspaces/${encodeURIComponent(workspaceId)}/collab`
    + `?token=${encodeURIComponent(token)}`
    + `&username=${encodeURIComponent(username || '')}`
}

const THROTTLE_MS = 150
const MOUSE_THROTTLE_MS = 45

// WindowGeo is a shared window's geometry, published/received piecemeal (a
// move sends {x,y}, a resize sends {w,h}; consumers merge onto prior state).
export interface WindowGeo {
  x?: number
  y?: number
  w?: number
  h?: number
  [key: string]: unknown
}

type WindowSub = (win: string, geo: WindowGeo, id: string) => void
type TerminalsSub = (list: Pick<TerminalSession, 'id' | 'name'>[]) => void

interface CollabMessage {
  type?: string
  members?: PresenceMember[]
  id?: string
  x?: number
  y?: number
  win?: string
  geo?: WindowGeo
  list?: Pick<TerminalSession, 'id' | 'name'>[]
}

export interface UseCollabResult {
  roster: PresenceMember[]
  setViewing: (file: string | null | undefined, line?: number) => void
  mice: Record<string, MousePosition>
  sendMouse: (fx: number, fy: number) => void
  sendWindow: (win: string, geo: WindowGeo) => void
  onWindow: (cb: WindowSub) => () => void
  sendTerminals: (list: Pick<TerminalSession, 'id' | 'name'>[]) => void
  onTerminals: (cb: TerminalsSub) => () => void
}

export function useCollab(
  workspaceId: string | null | undefined,
  token: string | null | undefined,
  username: string | null | undefined,
): UseCollabResult {
  const [roster, setRoster] = useState<PresenceMember[]>([])
  const [mice, setMice] = useState<Record<string, MousePosition>>({})
  const wsRef = useRef<WebSocket | null>(null)
  const lastSentRef = useRef<{ file: string | null; line: number }>({ file: null, line: -1 })
  const pendingRef = useRef<{ file: string; line: number } | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const mouseTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pendingMouseRef = useRef<MousePosition | null>(null)
  const windowSubsRef = useRef<Set<WindowSub>>(new Set())
  const terminalsSubsRef = useRef<Set<TerminalsSub>>(new Set())

  useEffect(() => {
    if (!workspaceId || !token) return undefined
    let disposed = false
    let ws: WebSocket | undefined
    try {
      ws = new WebSocket(buildWsUrl(workspaceId, token, username))
      wsRef.current = ws
      ws.onmessage = (ev: MessageEvent) => {
        try {
          const msg: CollabMessage = JSON.parse(ev.data)
          if (!msg) return
          if (msg.type === 'presence' && Array.isArray(msg.members)) {
            setRoster(msg.members)
            // Drop mouse cursors for members who have left.
            const ids = new Set(msg.members.map((m) => m.id))
            setMice((prev) => {
              const next: Record<string, MousePosition> = {}
              for (const k of Object.keys(prev)) if (ids.has(k)) next[k] = prev[k]
              return next
            })
          } else if (msg.type === 'mouse' && msg.id) {
            const id = msg.id
            setMice((prev) => ({ ...prev, [id]: { x: msg.x ?? 0, y: msg.y ?? 0 } }))
          } else if (msg.type === 'window' && msg.id) {
            const { win, geo, id } = msg
            windowSubsRef.current.forEach((cb) => { try { cb(win ?? '', geo ?? {}, id ?? '') } catch { /* ignore */ } })
          } else if (msg.type === 'terminals' && Array.isArray(msg.list)) {
            terminalsSubsRef.current.forEach((cb) => { try { cb(msg.list ?? []) } catch { /* ignore */ } })
          }
        } catch { /* ignore malformed frames */ }
      }
      ws.onclose = () => {
        if (wsRef.current === ws) wsRef.current = null
        if (!disposed) { setRoster([]); setMice({}) }
      }
      ws.onerror = () => { /* non-fatal; onclose handles cleanup */ }
    } catch { /* construction failed → collab inactive */ }

    return () => {
      disposed = true
      setRoster([]); setMice({})
      lastSentRef.current = { file: null, line: -1 }
      if (timerRef.current) { clearTimeout(timerRef.current); timerRef.current = null }
      if (mouseTimerRef.current) { clearTimeout(mouseTimerRef.current); mouseTimerRef.current = null }
      try { if (ws) ws.close() } catch { /* ignore */ }
      if (wsRef.current === ws) wsRef.current = null
    }
  }, [workspaceId, token, username])

  const send = useCallback((obj: unknown) => {
    const ws = wsRef.current
    if (ws && ws.readyState === WebSocket.OPEN) {
      try { ws.send(JSON.stringify(obj)) } catch { /* ignore */ }
    }
  }, [])

  // setViewing publishes the local cursor, throttled (leading + trailing) so
  // rapid tab/cursor changes don't flood the socket.
  const setViewing = useCallback((file: string | null | undefined, line = 0) => {
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
    if (timerRef.current) return
    flush()
    timerRef.current = setTimeout(flush, THROTTLE_MS)
  }, [])

  // sendMouse publishes the local mouse position as viewport fractions, throttled.
  const sendMouse = useCallback((fx: number, fy: number) => {
    pendingMouseRef.current = { x: fx, y: fy }
    if (mouseTimerRef.current) return
    const flush = () => {
      mouseTimerRef.current = null
      const p = pendingMouseRef.current
      if (p) send({ type: 'mouse', x: p.x, y: p.y })
    }
    flush()
    mouseTimerRef.current = setTimeout(flush, MOUSE_THROTTLE_MS)
  }, [send])

  const sendWindow = useCallback((win: string, geo: WindowGeo) => send({ type: 'window', win, geo }), [send])

  const onWindow = useCallback((cb: WindowSub) => {
    windowSubsRef.current.add(cb)
    return () => windowSubsRef.current.delete(cb)
  }, [])

  const sendTerminals = useCallback((list: Pick<TerminalSession, 'id' | 'name'>[]) => send({ type: 'terminals', list }), [send])
  const onTerminals = useCallback((cb: TerminalsSub) => {
    terminalsSubsRef.current.add(cb)
    return () => terminalsSubsRef.current.delete(cb)
  }, [])

  return { roster, setViewing, mice, sendMouse, sendWindow, onWindow, sendTerminals, onTerminals }
}
