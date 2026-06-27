// FloatingTerminals — a window manager that renders each terminal session as a
// movable, resizable floating window over the editor area. Same sessions as the
// docked panel (shared via useTerminals), so popping out reconnects to the same
// server-side PTYs.

import { useState, useEffect, useRef, useCallback } from 'react'
import { TerminalSquare, X, Plus, PanelBottom } from 'lucide-react'
import Terminal from './Terminal'
import { useTheme } from '../hooks/useTheme'

function FloatingWindow({ geo, title, focused, onFocus, onMove, onResize, onClose, children }) {
  const startDrag = (e) => {
    if (e.button !== 0) return
    onFocus()
    const sx = e.clientX, sy = e.clientY, ox = geo.x, oy = geo.y
    const move = (ev) => onMove(Math.max(0, ox + ev.clientX - sx), Math.max(0, oy + ev.clientY - sy))
    const up = () => { document.removeEventListener('mousemove', move); document.removeEventListener('mouseup', up) }
    document.addEventListener('mousemove', move); document.addEventListener('mouseup', up)
  }
  const startResize = (e) => {
    e.stopPropagation(); e.preventDefault()
    const sx = e.clientX, sy = e.clientY, ow = geo.w, oh = geo.h
    const move = (ev) => onResize(Math.max(300, ow + ev.clientX - sx), Math.max(160, oh + ev.clientY - sy))
    const up = () => { document.removeEventListener('mousemove', move); document.removeEventListener('mouseup', up) }
    document.addEventListener('mousemove', move); document.addEventListener('mouseup', up)
  }
  return (
    <div onMouseDown={onFocus}
      style={{ left: geo.x, top: geo.y, width: geo.w, height: geo.h, zIndex: geo.z }}
      className={`absolute pointer-events-auto bg-bg-tertiary border rounded-lg shadow-2xl flex flex-col overflow-hidden ${focused ? 'border-accent/50 ring-1 ring-accent/20' : 'border-border'}`}>
      <div onMouseDown={startDrag} className="flex items-center gap-1.5 px-2.5 h-7 bg-bg-secondary border-b border-border cursor-move select-none shrink-0">
        <TerminalSquare className="w-3 h-3 text-text-muted shrink-0" />
        <span className="text-[11px] font-medium text-text-secondary truncate flex-1">{title}</span>
        <button onClick={onClose} title="Close" className="w-4 h-4 flex items-center justify-center rounded text-text-muted hover:text-red hover:bg-bg-active"><X className="w-3 h-3" /></button>
      </div>
      <div className="flex-1 min-h-0 relative">{children}</div>
      <div onMouseDown={startResize} className="absolute bottom-0 right-0 w-4 h-4 cursor-nwse-resize z-10">
        <div className="absolute bottom-1 right-1 w-1.5 h-1.5 border-r-2 border-b-2 border-text-muted/50" />
      </div>
    </div>
  )
}

export default function FloatingTerminals({ token, workspaceId, term, onDock, sendWindow, onWindow }) {
  const { terminalTheme } = useTheme()
  const { terminals, addTerminal, closeTerminal, clearInitial, termRefs } = term
  const [geos, setGeos] = useState({})
  const geosRef = useRef(geos)
  const zRef = useRef(20)
  useEffect(() => { geosRef.current = geos }, [geos])

  // Apply window geometry pushed by a collaborator (no echo — we only broadcast
  // from the drag/resize handlers below, never when applying a peer's update).
  useEffect(() => {
    if (!onWindow) return undefined
    return onWindow((win, geo) => {
      const id = Number(String(win).replace('term-', ''))
      if (!id || !geo) return
      setGeos((p) => (p[id] ? { ...p, [id]: { ...p[id], x: geo.x, y: geo.y, w: geo.w, h: geo.h } } : p))
    })
  }, [onWindow])

  useEffect(() => {
    setGeos((prev) => {
      const next = { ...prev }
      let n = Object.keys(prev).length
      terminals.forEach((t) => {
        if (!next[t.id]) {
          next[t.id] = { x: 70 + (n % 6) * 38, y: 60 + (n % 6) * 34, w: 540, h: 320, z: ++zRef.current }
          n++
        }
      })
      Object.keys(next).forEach((id) => { if (!terminals.find((t) => String(t.id) === id)) delete next[id] })
      return next
    })
  }, [terminals])

  const focus = useCallback((id) => setGeos((p) => (p[id] ? { ...p, [id]: { ...p[id], z: ++zRef.current } } : p)), [])
  const move = (id, x, y) => {
    setGeos((p) => ({ ...p, [id]: { ...p[id], x, y } }))
    sendWindow?.(`term-${id}`, { ...(geosRef.current[id] || {}), x, y })
  }
  const resize = (id, w, h) => {
    setGeos((p) => ({ ...p, [id]: { ...p[id], w, h } }))
    sendWindow?.(`term-${id}`, { ...(geosRef.current[id] || {}), w, h })
  }

  const topZ = Math.max(0, ...Object.values(geos).map((g) => g.z))

  return (
    <div className="absolute inset-0 pointer-events-none z-30">
      {/* Floating toolbar */}
      <div className="absolute top-2 left-1/2 -translate-x-1/2 pointer-events-auto flex items-center gap-1 bg-bg-secondary border border-border rounded-lg shadow-lg px-1.5 py-1" style={{ zIndex: topZ + 1 }}>
        <button onClick={addTerminal} title="New terminal" className="p-1 text-text-muted hover:text-text-primary hover:bg-bg-hover rounded"><Plus className="w-3.5 h-3.5" /></button>
        <div className="w-px h-4 bg-border" />
        <button onClick={onDock} title="Dock terminals to the bottom" className="flex items-center gap-1 px-2 py-0.5 text-[11px] text-text-muted hover:text-text-primary hover:bg-bg-hover rounded"><PanelBottom className="w-3.5 h-3.5" /> Dock</button>
      </div>
      {terminals.map((t) => {
        const geo = geos[t.id]
        if (!geo) return null
        return (
          <FloatingWindow key={t.id} geo={geo} title={t.name} focused={geo.z === topZ}
            onFocus={() => focus(t.id)} onMove={(x, y) => move(t.id, x, y)} onResize={(w, h) => resize(t.id, w, h)}
            onClose={() => closeTerminal(t.id)}>
            <Terminal
              ref={(r) => { if (r) termRefs.current[t.id] = r; else delete termRefs.current[t.id] }}
              token={token} workspaceId={workspaceId} sessionId={`term-${t.id}`} visible terminalTheme={terminalTheme}
              initialCommand={t.initial} onInitialRun={() => clearInitial?.(t.id)} />
          </FloatingWindow>
        )
      })}
    </div>
  )
}
