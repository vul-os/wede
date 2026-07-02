import { useState, useMemo } from 'react'
import { ChevronRight, ChevronDown, GitBranch } from 'lucide-react'
import GitPanel from './GitPanel'
import { makeWsFetch } from '../lib/wsScope'

// GitPanels — multi-root Source Control. Each open workspace is its own git repo
// (commits/branches are inherently per-repo), so we stack a full GitPanel per
// root in collapsible sections, exactly like VS Code shows multiple repositories.
// A single root renders the bare GitPanel so the common case is unchanged.
//
// Each section gets a workspace-scoped fetch so its GitPanel — unmodified — acts
// on that repo alone.
export default function GitPanels({ authFetch, workspaces = [], visible, readOnly = false, onOpenGraph, workspaceId }) {
  const roots = workspaces.length ? workspaces : (workspaceId ? [{ id: workspaceId, name: '' }] : [])

  if (roots.length <= 1) {
    return <GitPanel authFetch={authFetch} visible={visible} readOnly={readOnly} onOpenGraph={onOpenGraph} />
  }

  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden">
      <div className="flex-1 overflow-y-auto min-h-0">
        {roots.map((ws) => (
          <RepoSection key={ws.id} ws={ws} authFetch={authFetch} parentVisible={visible} readOnly={readOnly} onOpenGraph={onOpenGraph} />
        ))}
      </div>
    </div>
  )
}

function RepoSection({ ws, authFetch, parentVisible, readOnly, onOpenGraph }) {
  const [open, setOpen] = useState(true)

  // Pin every /api/git|files/... call this GitPanel makes to this workspace.
  const wsFetch = useMemo(() => makeWsFetch(authFetch, ws.id), [authFetch, ws.id])

  return (
    <div className="border-b border-border/50">
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center gap-1.5 px-2 h-[26px] sticky top-0 z-10 bg-bg-secondary hover:bg-bg-hover/60 select-none">
        {open ? <ChevronDown className="w-3 h-3 text-text-muted shrink-0" /> : <ChevronRight className="w-3 h-3 text-text-muted shrink-0" />}
        <GitBranch className="w-3 h-3 text-text-muted shrink-0" />
        <span className="text-[11px] font-bold uppercase tracking-wide text-text-secondary truncate" title={ws.root}>{ws.name}</span>
      </button>
      {open && (
        // Bounded height so several repos share the sidebar without one dominating.
        <div className="max-h-[60vh] overflow-hidden flex flex-col">
          <GitPanel authFetch={wsFetch} visible={parentVisible && open} readOnly={readOnly} onOpenGraph={onOpenGraph} />
        </div>
      )}
    </div>
  )
}
