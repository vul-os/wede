import { useState } from 'react'
import { Plus, X, TerminalSquare, Maximize2, Minimize2, PictureInPicture2 } from 'lucide-react'
import Terminal from './Terminal'
import TerminalToolbar from './TerminalToolbar'
import { useTheme } from '../hooks/useTheme'

// TerminalPanel — the docked, tabbed terminal panel. Terminal state lives in the
// shared useTerminals hook (passed in as `term`) so it stays in sync with the
// floating window manager.
export default function TerminalPanel({ token, workspaceId, term, visible, isFullscreen, onToggleFullscreen, onPopOut, isMobile }) {
  const { terminalTheme } = useTheme()
  const { terminals, activeId, setActiveId, addTerminal, closeTerminal, clearInitial, renameTerminal, autoNameTerminal, termRefs } = term

  // Inline rename: double-click a tab to edit its name.
  const [editingId, setEditingId] = useState(null)
  const [editName, setEditName] = useState('')
  const startRename = (t) => { setEditingId(t.id); setEditName(t.name) }
  const commitRename = () => { if (editingId != null) renameTerminal(editingId, editName); setEditingId(null) }

  const handleToolbarSend = (data) => {
    const ref = termRefs.current[activeId]
    if (ref) ref.send(data)
  }

  if (!visible) return null

  return (
    <div className="h-full flex flex-col bg-bg-tertiary">
      {/* Terminal tab bar */}
      <div className="flex items-center border-b border-border compact-touch shrink-0" style={{ height: 34 }}>
        <div className="flex items-center shrink-0 px-2.5 border-r border-border h-full">
          <TerminalSquare className="w-3.5 h-3.5 text-text-muted" />
        </div>

        <div className="flex items-center flex-1 overflow-x-auto h-full" style={{ scrollbarWidth: 'none' }}>
          {terminals.map((t) => {
            const isActive = activeId === t.id
            return (
              <button
                key={t.id}
                onClick={() => setActiveId(t.id)}
                className={`relative flex items-center gap-1.5 px-3 h-full text-[12px] font-medium border-r border-border shrink-0 transition-colors ${
                  isActive
                    ? 'bg-bg-primary text-text-primary'
                    : 'text-text-muted hover:text-text-secondary hover:bg-bg-hover'
                }`}
              >
                {isActive && <span className="absolute top-0 left-0 right-0 h-[1.5px] bg-accent" />}
                {editingId === t.id ? (
                  <input
                    autoFocus
                    value={editName}
                    onChange={(e) => setEditName(e.target.value)}
                    onBlur={commitRename}
                    onClick={(e) => e.stopPropagation()}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') commitRename()
                      else if (e.key === 'Escape') setEditingId(null)
                    }}
                    className="bg-bg-input border border-accent/50 rounded px-1 py-0.5 text-[12px] text-text-primary w-24 focus:outline-none focus:border-accent"
                  />
                ) : (
                  <span onDoubleClick={(e) => { e.stopPropagation(); startRename(t) }} title="Double-click to rename">{t.name}</span>
                )}
                {terminals.length > 1 && (
                  <span
                    onClick={(e) => { e.stopPropagation(); closeTerminal(t.id) }}
                    className="ml-0.5 w-4 h-4 flex items-center justify-center rounded text-text-muted hover:text-text-primary hover:bg-bg-active transition-colors"
                  >
                    <X className="w-2.5 h-2.5" />
                  </span>
                )}
              </button>
            )
          })}

          <button
            onClick={() => addTerminal()}
            className="flex items-center justify-center w-7 h-7 mx-1.5 text-text-muted hover:text-text-primary hover:bg-bg-hover rounded-md transition-colors shrink-0"
            title="New Terminal"
          >
            <Plus className="w-3.5 h-3.5" />
          </button>
        </div>

        {onPopOut && (
          <button
            onClick={onPopOut}
            className="p-1.5 mx-0.5 text-text-muted hover:text-text-primary hover:bg-bg-hover rounded-md transition-colors shrink-0"
            title="Open terminals as floating windows"
          >
            <PictureInPicture2 className="w-3.5 h-3.5" />
          </button>
        )}
        {onToggleFullscreen && (
          <button
            onClick={onToggleFullscreen}
            className="p-1.5 mx-1.5 text-text-muted hover:text-text-primary hover:bg-bg-hover rounded-md transition-colors shrink-0"
            title={isFullscreen ? 'Exit Fullscreen' : 'Fullscreen'}
          >
            {isFullscreen ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5" />}
          </button>
        )}
      </div>

      {/* Terminal instances */}
      <div className="flex-1 min-h-0">
        {terminals.map((t) => (
          <Terminal
            key={t.id}
            ref={(r) => { if (r) termRefs.current[t.id] = r; else delete termRefs.current[t.id] }}
            token={token}
            workspaceId={workspaceId}
            sessionId={`term-${t.id}`}
            visible={activeId === t.id && visible}
            terminalTheme={terminalTheme}
            initialCommand={t.initial}
            onInitialRun={() => clearInitial?.(t.id)}
            onTitle={(title) => autoNameTerminal(t.id, title)}
          />
        ))}
      </div>

      {isMobile && <TerminalToolbar onSend={handleToolbarSend} />}
    </div>
  )
}
