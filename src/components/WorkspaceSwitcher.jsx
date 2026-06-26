import { useState, useRef, useEffect } from 'react'
import { Boxes, ChevronDown, Plus, Check, X } from 'lucide-react'

// WorkspaceSwitcher shows the active workspace ("workspace") and lets you switch between
// open workspaces or open a new one. Workspaces are the multi-workspace backbone: each is an
// isolated workspace served by the same wede instance.
export default function WorkspaceSwitcher({ workspacesApi }) {
  const [open, setOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [path, setPath] = useState('')
  const [name, setName] = useState('')
  const [error, setError] = useState(null)
  const [busy, setBusy] = useState(false)
  const ref = useRef(null)

  const workspaces = workspacesApi?.workspaces || []
  const activeId = workspacesApi?.activeWorkspaceId
  const active = workspaces.find((r) => r.id === activeId)

  // Close on outside click.
  useEffect(() => {
    if (!open) return
    const onDown = (e) => { if (ref.current && !ref.current.contains(e.target)) { setOpen(false); setAdding(false) } }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [open])

  const create = async () => {
    if (!path.trim()) { setError('Enter a folder path'); return }
    setBusy(true); setError(null)
    try {
      const workspace = await workspacesApi.createWorkspace(name.trim(), path.trim())
      workspacesApi.setActiveWorkspaceId(workspace.id)
      setAdding(false); setOpen(false); setPath(''); setName('')
    } catch (e) {
      setError(e.message || 'Could not open workspace')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-1.5 px-2 py-1 rounded-md text-[12px] text-text-secondary hover:text-text-primary hover:bg-bg-hover transition-colors"
        title="Switch workspace">
        <Boxes className="w-3.5 h-3.5 text-accent" />
        <span className="max-w-32 truncate font-medium">{active?.name || 'default'}</span>
        <ChevronDown className="w-3 h-3 opacity-60" />
      </button>

      {open && (
        <div className="absolute left-0 top-full mt-1 w-64 bg-bg-elevated border border-border rounded-lg shadow-xl shadow-shadow z-50 p-1 animate-fade-in">
          <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-text-muted">Workspaces</div>
          <div className="max-h-56 overflow-auto">
            {workspaces.map((r) => (
              <button
                key={r.id}
                onClick={() => { workspacesApi.setActiveWorkspaceId(r.id); setOpen(false) }}
                className="w-full flex items-center gap-2 px-2 py-1.5 rounded-md text-[12px] text-text-secondary hover:text-text-primary hover:bg-bg-hover transition-colors text-left">
                <span className="w-3.5 shrink-0">
                  {r.id === activeId && <Check className="w-3.5 h-3.5 text-accent" />}
                </span>
                <span className="flex-1 min-w-0">
                  <span className="block truncate font-medium">{r.name}</span>
                  <span className="block truncate text-[10px] text-text-muted">{r.root}</span>
                </span>
              </button>
            ))}
            {workspaces.length === 0 && (
              <div className="px-2 py-2 text-[11px] text-text-muted">No workspaces yet</div>
            )}
          </div>

          <div className="border-t border-border mt-1 pt-1">
            {!adding ? (
              <button
                onClick={() => setAdding(true)}
                className="w-full flex items-center gap-2 px-2 py-1.5 rounded-md text-[12px] text-text-secondary hover:text-text-primary hover:bg-bg-hover transition-colors">
                <Plus className="w-3.5 h-3.5" /> New workspace…
              </button>
            ) : (
              <div className="p-1.5 space-y-1.5">
                <input
                  autoFocus
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  onKeyDown={(e) => { if (e.key === 'Enter') create(); if (e.key === 'Escape') setAdding(false) }}
                  placeholder="/path/to/workspace"
                  className="w-full px-2 py-1 rounded-md bg-bg-secondary border border-border text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/40" />
                <input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  onKeyDown={(e) => { if (e.key === 'Enter') create(); if (e.key === 'Escape') setAdding(false) }}
                  placeholder="Name (optional)"
                  className="w-full px-2 py-1 rounded-md bg-bg-secondary border border-border text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/40" />
                {error && <div className="text-[11px] text-red px-0.5">{error}</div>}
                <div className="flex items-center gap-1.5">
                  <button
                    onClick={create}
                    disabled={busy}
                    className="flex-1 flex items-center justify-center gap-1 px-2 py-1 rounded-md text-[12px] bg-accent/10 text-accent hover:bg-accent/20 transition-colors disabled:opacity-50">
                    {busy ? 'Opening…' : 'Open'}
                  </button>
                  <button
                    onClick={() => { setAdding(false); setError(null) }}
                    className="flex items-center justify-center px-2 py-1 rounded-md text-[12px] text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors">
                    <X className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
