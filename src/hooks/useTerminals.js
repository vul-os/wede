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
      // Never persist `initial` (a one-shot task command) — it must not re-run on reload.
      localStorage.setItem('wede_terminals', JSON.stringify(terminals.map((t) => ({ id: t.id, name: t.name }))))
      localStorage.setItem('wede_terminal_active', String(activeId))
    } catch { /* ignore */ }
  }, [terminals, activeId])

  // addTerminal(name?, initial?) — `initial` is a command run once when the new
  // terminal's PTY connects (used by the task runner).
  const addTerminal = useCallback((name, initial) => {
    const id = nextId++
    setTerminals((prev) => [...prev, { id, name: name || `Terminal ${id}`, initial: initial || undefined }])
    setActiveId(id)
    return id
  }, [])

  // clearInitial drops a terminal's one-shot command so it can't re-run on a
  // tab-switch remount within the session.
  const clearInitial = useCallback((id) => {
    setTerminals((prev) => prev.map((t) => (t.id === id ? { ...t, initial: undefined } : t)))
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

  return { terminals, activeId, setActiveId, addTerminal, closeTerminal, clearInitial, termRefs }
}
