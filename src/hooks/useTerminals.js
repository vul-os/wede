// useTerminals — owns the list of terminal sessions (id + name) and the active
// one, shared between the docked TerminalPanel and the floating window manager so
// both show the *same* server-side PTY sessions (keyed by sessionId `term-<id>`).

import { useState, useCallback, useEffect, useRef } from 'react'
import { workspaceUrl } from '../api'

function loadTerminals() {
  try {
    const saved = localStorage.getItem('wede_terminals')
    if (saved) {
      const parsed = JSON.parse(saved)
      if (Array.isArray(parsed) && parsed.length > 0) return parsed
    }
  } catch { /* ignore */ }
  return null
}

let nextId = (() => {
  const saved = loadTerminals()
  return saved ? Math.max(...saved.map((t) => t.id)) + 1 : 1
})()

export function useTerminals(authFetch, workspaceId) {
  const [terminals, setTerminals] = useState(() => loadTerminals() || [{ id: nextId++, name: 'Terminal 1' }])
  const [activeId, setActiveId] = useState(() => {
    const saved = localStorage.getItem('wede_terminal_active')
    return saved ? Number(saved) : (loadTerminals()?.[0]?.id || 1)
  })
  const reconciledRef = useRef(false)
  const termRefs = useRef({})

  // Reconcile with live server sessions on mount (so refresh keeps PTYs).
  useEffect(() => {
    if (reconciledRef.current || !authFetch) return
    reconciledRef.current = true
    authFetch(workspaceId ? workspaceUrl(workspaceId, '/terminal/sessions') : '/api/terminal/sessions')
      .then((res) => res.json())
      .then((data) => {
        const serverSessions = new Set(data.sessions || [])
        if (serverSessions.size === 0) return
        setTerminals((prev) => {
          const alive = prev.filter((t) => serverSessions.has(`term-${t.id}`))
          const known = new Set(prev.map((t) => `term-${t.id}`))
          const orphans = [...serverSessions].filter((s) => s.startsWith('term-') && !known.has(s)).map((s) => {
            const id = Number(s.replace('term-', ''))
            if (id >= nextId) nextId = id + 1
            return { id, name: `Terminal ${id}` }
          })
          return (alive.length || orphans.length) ? [...alive, ...orphans] : prev
        })
      })
      .catch(() => {})
  }, [authFetch, workspaceId])

  useEffect(() => {
    try {
      localStorage.setItem('wede_terminals', JSON.stringify(terminals))
      localStorage.setItem('wede_terminal_active', String(activeId))
    } catch { /* ignore */ }
  }, [terminals, activeId])

  const addTerminal = useCallback(() => {
    const id = nextId++
    setTerminals((prev) => [...prev, { id, name: `Terminal ${id}` }])
    setActiveId(id)
    return id
  }, [])

  const closeTerminal = useCallback((id) => {
    setTerminals((prev) => {
      const next = prev.filter((t) => t.id !== id)
      if (next.length === 0) {
        const newId = nextId++
        next.push({ id: newId, name: `Terminal ${newId}` })
        setActiveId(newId)
      } else if (activeId === id) {
        setActiveId(next[next.length - 1].id)
      }
      return next
    })
  }, [activeId])

  return { terminals, activeId, setActiveId, addTerminal, closeTerminal, termRefs }
}
