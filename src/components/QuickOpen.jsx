import { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import { File, Search } from 'lucide-react'
import { workspaceUrl } from '../api'

// Fuzzy score: substring match ranks highest, then subsequence; 0 = no match.
function score(query, path) {
  if (!query) return 1
  const q = query.toLowerCase()
  const p = path.toLowerCase()
  const base = p.slice(p.lastIndexOf('/') + 1)
  if (base.includes(q)) return 1000 - base.indexOf(q) // match in filename ranks best
  const idx = p.includes(q) ? p.indexOf(q) : -1
  if (idx >= 0) return 500 - idx
  // subsequence
  let qi = 0
  for (let i = 0; i < p.length && qi < q.length; i++) if (p[i] === q[qi]) qi++
  return qi === q.length ? 100 : 0
}

// QuickOpen — VS Code-style Cmd+P fuzzy file finder. In multi-root workspaces it
// indexes EVERY open root and merges the results (each labeled with its folder),
// so one search spans all folders. Defensive: any fetch failure yields an empty
// list and the modal still closes cleanly.
export default function QuickOpen({ visible, onClose, authFetch, workspaces = [], workspaceId, onOpenFile }) {
  // Each entry: { path (rel), workspaceId, rootName }.
  const [files, setFiles] = useState([])
  const [query, setQuery] = useState('')
  const [sel, setSel] = useState(0)
  const inputRef = useRef(null)

  // The set of roots to index — the full workspace list, or a synthetic single
  // entry from workspaceId for callers that don't pass the list.
  const roots = useMemo(() => (
    workspaces.length ? workspaces : (workspaceId ? [{ id: workspaceId, name: '' }] : [])
  ), [workspaces, workspaceId])
  const multiRoot = roots.length > 1

  // Load the file index each time the finder opens (cheap; reflects new files).
  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (!visible) return
    setQuery('')
    setSel(0)
    let cancelled = false
    Promise.all(roots.map((ws) =>
      authFetch(workspaceUrl(ws.id, '/files/tree'))
        .then((r) => r.json())
        .then((data) => (Array.isArray(data.files) ? data.files : []).map((path) => ({ path, workspaceId: ws.id, rootName: ws.name })))
        .catch(() => [])
    )).then((lists) => { if (!cancelled) setFiles(lists.flat()) })
    return () => { cancelled = true }
  }, [visible, roots, authFetch])
  /* eslint-enable react-hooks/set-state-in-effect */

  useEffect(() => { if (visible) inputRef.current?.focus() }, [visible])

  const results = useMemo(() => {
    if (!query) return files.slice(0, 50)
    return files
      .map((f) => ({ f, s: score(query, f.path) }))
      .filter((x) => x.s > 0)
      .sort((a, b) => b.s - a.s)
      .slice(0, 50)
      .map((x) => x.f)
  }, [files, query])

  const choose = useCallback((entry) => {
    if (!entry) return
    onOpenFile({ workspaceId: entry.workspaceId, rel: entry.path, path: entry.path, name: entry.path.split('/').pop(), isDir: false })
    onClose()
  }, [onOpenFile, onClose])

  const onKeyDown = (e) => {
    if (e.key === 'Escape') { onClose(); return }
    if (e.key === 'ArrowDown') { e.preventDefault(); setSel((s) => Math.min(s + 1, results.length - 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setSel((s) => Math.max(s - 1, 0)) }
    else if (e.key === 'Enter') { e.preventDefault(); choose(results[sel]) }
  }

  if (!visible) return null

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[12vh] bg-black/30" onClick={onClose}>
      <div
        className="w-full max-w-xl bg-bg-elevated border border-border rounded-xl shadow-2xl shadow-shadow overflow-hidden animate-fade-in"
        onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center gap-2 px-3 border-b border-border">
          <Search className="w-4 h-4 text-text-muted shrink-0" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => { setQuery(e.target.value); setSel(0) }}
            onKeyDown={onKeyDown}
            placeholder="Go to file…"
            className="flex-1 bg-transparent py-3 text-sm text-text-primary placeholder:text-text-muted focus:outline-none" />
        </div>
        <div className="max-h-80 overflow-auto py-1">
          {results.length === 0 && (
            <div className="px-3 py-6 text-center text-[12px] text-text-muted">No matching files</div>
          )}
          {results.map((f, i) => {
            const name = f.path.split('/').pop()
            const dir = f.path.slice(0, f.path.length - name.length)
            return (
              <button
                key={`${f.workspaceId}:${f.path}`}
                onMouseEnter={() => setSel(i)}
                onClick={() => choose(f)}
                className={`w-full flex items-center gap-2 px-3 py-1.5 text-left text-[12px] transition-colors ${
                  i === sel ? 'bg-accent/10 text-text-primary' : 'text-text-secondary hover:bg-bg-hover'
                }`}>
                <File className="w-3.5 h-3.5 shrink-0 text-text-muted" />
                <span className="truncate font-medium">{name}</span>
                {dir && <span className="truncate text-[11px] text-text-muted">{dir.replace(/\/$/, '')}</span>}
                {multiRoot && f.rootName && (
                  <span className="ml-auto shrink-0 text-[10px] px-1.5 py-0.5 rounded bg-bg-hover text-text-muted">{f.rootName}</span>
                )}
              </button>
            )
          })}
        </div>
      </div>
    </div>
  )
}
