// useChat — connects to a workspace's chat WebSocket for live + persisted chat.
//
// Opens /api/workspaces/{id}/chat?token=&username=&color= (mirroring useCollab.js's
// URL / dev-port logic), parses incoming frames:
//   {type:"history", messages:[...]}  — full history on join
//   {type:"msg",     message:{...}}   — live message
//
// Exposes { messages, sendMessage(text) }.
//
// Defensive by design: any failure leaves chat inactive (empty messages), never
// throwing into the IDE.

import { useEffect, useRef, useState, useCallback } from 'react'

function buildWsUrl(workspaceId, token, username, color) {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  // In dev (Vite on 5173/5174) the WS must hit the backend on :9090 directly.
  const port = window.location.port
  const host = (port === '5173' || port === '5174')
    ? window.location.hostname + ':9090'
    : window.location.host
  return `${proto}//${host}/api/workspaces/${encodeURIComponent(workspaceId)}/chat`
    + `?token=${encodeURIComponent(token || '')}`
    + `&username=${encodeURIComponent(username || '')}`
    + `&color=${encodeURIComponent(color || '#888888')}`
}

export function useChat(workspaceId, token, username, color) {
  const [messages, setMessages] = useState([])
  const wsRef = useRef(null)

  useEffect(() => {
    if (!workspaceId || !token) return undefined
    let ws
    try {
      ws = new WebSocket(buildWsUrl(workspaceId, token, username, color))
      wsRef.current = ws
      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data)
          if (!msg) return
          if (msg.type === 'history' && Array.isArray(msg.messages)) {
            setMessages(msg.messages)
          } else if (msg.type === 'msg' && msg.message) {
            setMessages((prev) => [...prev, msg.message])
          }
        } catch { /* ignore malformed frames */ }
      }
      ws.onclose = () => {
        if (wsRef.current === ws) wsRef.current = null
      }
      ws.onerror = () => { /* non-fatal; onclose handles cleanup */ }
    } catch { /* construction failed → chat inactive */ }

    return () => {
      try { if (ws) ws.close() } catch { /* ignore */ }
      if (wsRef.current === ws) wsRef.current = null
    }
  }, [workspaceId, token, username, color])

  const sendMessage = useCallback((text) => {
    const ws = wsRef.current
    if (!ws || ws.readyState !== WebSocket.OPEN || !text?.trim()) return
    try {
      ws.send(JSON.stringify({ type: 'msg', text: text.trim() }))
    } catch { /* ignore */ }
  }, [])

  return { messages, sendMessage }
}
