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

// QuickOpen — VS Code-style Cmd+P fuzzy file finder. Fetches a flat file index
// from the backend and opens the chosen file. Defensive: any fetch failure
// yields an empty list and the modal still closes cleanly.
export default function QuickOpen({ visible, onClose, authFetch, workspaceId, onOpenFile }) {
  const [files, setFiles] = useState([])
  const [query, setQuery] = useState('')
  const [sel, setSel] = useState(0)
  const inputRef = useRef(null)

  // Load the file index each time the finder opens (cheap; reflects new files).
  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (!visible) return
    setQuery('')
    setSel(0)
    let cancelled = false
    const url = workspaceId ? workspaceUrl(workspaceId, '/files/tree') : '/api/files/tree'
    authFetch(url)
      .then((r) => r.json())
      .then((data) => { if (!cancelled) setFiles(Array.isArray(data.files) ? data.files : []) })
      .catch(() => { if (!cancelled) setFiles([]) })
    return () => { cancelled = true }
  }, [visible, workspaceId, authFetch])
  /* eslint-enable react-hooks/set-state-in-effect */

  useEffect(() => { if (visible) inputRef.current?.focus() }, [visible])

  const results = useMemo(() => {
    if (!query) return files.slice(0, 50)
    return files
      .map((f) => ({ f, s: score(query, f) }))
      .filter((x) => x.s > 0)
      .sort((a, b) => b.s - a.s)
      .slice(0, 50)
      .map((x) => x.f)
  }, [files, query])

  const choose = useCallback((path) => {
    if (!path) return
    onOpenFile({ path, name: path.split('/').pop(), isDir: false })
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
            const name = f.split('/').pop()
            const dir = f.slice(0, f.length - name.length)
            return (
              <button
                key={f}
                onMouseEnter={() => setSel(i)}
                onClick={() => choose(f)}
                className={`w-full flex items-center gap-2 px-3 py-1.5 text-left text-[12px] transition-colors ${
                  i === sel ? 'bg-accent/10 text-text-primary' : 'text-text-secondary hover:bg-bg-hover'
                }`}>
                <File className="w-3.5 h-3.5 shrink-0 text-text-muted" />
                <span className="truncate font-medium">{name}</span>
                {dir && <span className="truncate text-[11px] text-text-muted">{dir.replace(/\/$/, '')}</span>}
              </button>
            )
          })}
        </div>
      </div>
    </div>
  )
}
