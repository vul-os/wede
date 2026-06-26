// ApiCollections — the API client's collections tree, rendered in the IDE sidebar
// (like the file explorer). Clicking a request loads it into the shared state and
// asks the IDE to open the request-editor tab.

import { useState } from 'react'
import { Plus, FolderPlus, Trash2, ChevronRight, ChevronDown, Globe2 } from 'lucide-react'
import { parseReq } from '../lib/apiRequest'

const METHOD_COLOR = {
  GET: 'text-green', POST: 'text-yellow', PUT: 'text-accent', PATCH: 'text-accent',
  DELETE: 'text-red', HEAD: 'text-text-muted', OPTIONS: 'text-text-muted',
}

function TreeNode({ node, depth, onOpen, onDelete, readOnly, activePath }) {
  const [open, setOpen] = useState(true)
  if (node.type === 'folder') {
    return (
      <div>
        <div className="flex items-center gap-1 px-1 py-1 hover:bg-bg-hover rounded cursor-pointer group"
          style={{ paddingLeft: depth * 12 + 4 }} onClick={() => setOpen(!open)}>
          {open ? <ChevronDown className="w-3 h-3 text-text-muted" /> : <ChevronRight className="w-3 h-3 text-text-muted" />}
          <span className="text-[12px] text-text-secondary truncate flex-1">{node.name}</span>
          {!readOnly && <button onClick={(e) => { e.stopPropagation(); onDelete(node) }} className="opacity-0 group-hover:opacity-100 text-text-muted hover:text-red"><Trash2 className="w-3 h-3" /></button>}
        </div>
        {open && node.children?.map((c) => (
          <TreeNode key={c.path} node={c} depth={depth + 1} onOpen={onOpen} onDelete={onDelete} readOnly={readOnly} activePath={activePath} />
        ))}
      </div>
    )
  }
  const parsed = parseReq(node.request)
  const m = parsed.method || 'GET'
  return (
    <div className={`flex items-center gap-1.5 px-1 py-1 rounded cursor-pointer group ${activePath === node.path ? 'bg-bg-active' : 'hover:bg-bg-hover'}`}
      style={{ paddingLeft: depth * 12 + 16 }} onClick={() => onOpen(node)}>
      <span className={`text-[9px] font-bold w-9 shrink-0 ${METHOD_COLOR[m] || 'text-text-muted'}`}>{m}</span>
      <span className="text-[12px] text-text-primary truncate flex-1">{parsed.name || node.name}</span>
      {!readOnly && <button onClick={(e) => { e.stopPropagation(); onDelete(node) }} className="opacity-0 group-hover:opacity-100 text-text-muted hover:text-red"><Trash2 className="w-3 h-3" /></button>}
    </div>
  )
}

export default function ApiCollections({ api, readOnly = false, onOpenRequest }) {
  const open = (node) => { api.openRequest(node); onOpenRequest?.() }
  const newReq = () => { api.newRequest(); onOpenRequest?.() }

  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden">
      <div className="flex items-center justify-between px-3 py-2 border-b border-border shrink-0">
        <span className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">API Client</span>
        {!readOnly && (
          <div className="flex items-center gap-0.5">
            <button title="New request" onClick={newReq} className="p-1 text-text-muted hover:text-text-primary hover:bg-bg-hover rounded"><Plus className="w-3.5 h-3.5" /></button>
            <button title="New folder" onClick={api.newFolder} className="p-1 text-text-muted hover:text-text-primary hover:bg-bg-hover rounded"><FolderPlus className="w-3.5 h-3.5" /></button>
          </div>
        )}
      </div>

      <div className="flex-1 overflow-y-auto p-1 min-h-0">
        {api.tree.length === 0 ? (
          <p className="text-[11px] text-text-muted p-3 text-center leading-relaxed">No saved requests yet. Open the editor, build a request, and hit Save.</p>
        ) : api.tree.map((n) => (
          <TreeNode key={n.path} node={n} depth={0} onOpen={open} onDelete={api.deleteItem} readOnly={readOnly} activePath={api.savePath ? api.savePath + '.json' : null} />
        ))}
      </div>

      {/* Environment selector */}
      <div className="border-t border-border px-3 py-2 flex items-center gap-1.5 shrink-0">
        <Globe2 className="w-3.5 h-3.5 text-text-muted shrink-0" />
        <select value={api.activeEnv} onChange={(e) => api.setActiveEnv(e.target.value)}
          className="flex-1 bg-bg-input border border-border rounded px-1.5 py-1 text-[11px] text-text-primary focus:outline-none focus:border-accent/60">
          <option value="">No environment</option>
          {api.environments.map((e) => <option key={e.name} value={e.name}>{e.name}</option>)}
        </select>
      </div>
    </div>
  )
}
