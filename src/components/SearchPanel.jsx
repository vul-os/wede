import { useState, useCallback, useRef } from 'react'
import { Search, X, ChevronRight, Loader, AlertCircle, CaseSensitive, Regex } from 'lucide-react'

export default function SearchPanel({ authFetch, onOpenFile }) {
  const [query, setQuery] = useState('')
  const [caseSensitive, setCaseSensitive] = useState(false)
  const [useRegex, setUseRegex] = useState(false)
  const [results, setResults] = useState(null) // null = no search yet
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [truncated, setTruncated] = useState(false)
  const [count, setCount] = useState(0)

  // Group results by file for display
  const grouped = results ? groupByFile(results) : null

  const abortRef = useRef(null)

  const doSearch = useCallback(async (q, cs, rx) => {
    if (!q.trim()) {
      setResults(null)
      setError('')
      return
    }

    if (abortRef.current) abortRef.current.abort()
    const ctrl = new AbortController()
    abortRef.current = ctrl

    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams({ q: q.trim() })
      if (cs) params.set('case', 'true')
      if (rx) params.set('regex', 'true')
      const res = await authFetch(`/api/search?${params}`, { signal: ctrl.signal })
      const data = await res.json()
      if (data.error) {
        setError(data.error)
        setResults(null)
      } else {
        setResults(data.matches || [])
        setTruncated(data.truncated || false)
        setCount(data.count || 0)
      }
    } catch (e) {
      if (e.name !== 'AbortError') setError('Search failed')
    }
    setLoading(false)
  }, [authFetch])

  // Debounce: 350 ms after last keystroke
  const debounceRef = useRef(null)
  const triggerSearch = useCallback((q, cs, rx) => {
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => doSearch(q, cs, rx), 350)
  }, [doSearch])

  const handleQueryChange = (e) => {
    const v = e.target.value
    setQuery(v)
    triggerSearch(v, caseSensitive, useRegex)
  }

  const toggleCase = () => {
    const v = !caseSensitive
    setCaseSensitive(v)
    triggerSearch(query, v, useRegex)
  }

  const toggleRegex = () => {
    const v = !useRegex
    setUseRegex(v)
    triggerSearch(query, caseSensitive, v)
  }

  const clear = () => {
    setQuery('')
    setResults(null)
    setError('')
  }

  const handleResultClick = (match) => {
    onOpenFile?.({ path: match.file, name: match.file.split('/').pop(), isDir: false }, match.line)
  }

  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden">
      {/* Header */}
      <div className="px-3 py-2 border-b border-border shrink-0">
        <span className="text-[10px] font-bold uppercase tracking-widest text-text-muted">
          Search
        </span>
      </div>

      {/* Search input */}
      <div className="px-3 py-2 border-b border-border shrink-0">
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-text-muted pointer-events-none" />
          <input
            type="text"
            value={query}
            onChange={handleQueryChange}
            placeholder="Search files…"
            className="w-full pl-8 pr-16 py-1.5 text-[12px] bg-bg-input border border-border rounded-md text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 focus:ring-1 focus:ring-accent/20 transition-colors"
          />
          <div className="absolute right-2 top-1/2 -translate-y-1/2 flex items-center gap-0.5">
            <button
              onClick={toggleCase}
              title="Match case"
              className={`p-1 rounded transition-colors ${caseSensitive ? 'text-accent bg-accent/10' : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'}`}
            >
              <CaseSensitive className="w-3 h-3" />
            </button>
            <button
              onClick={toggleRegex}
              title="Use regular expression"
              className={`p-1 rounded transition-colors ${useRegex ? 'text-accent bg-accent/10' : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'}`}
            >
              <Regex className="w-3 h-3" />
            </button>
            {query && (
              <button onClick={clear} className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors">
                <X className="w-3 h-3" />
              </button>
            )}
          </div>
        </div>
      </div>

      {/* Results */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden min-h-0">
        {loading && (
          <div className="flex items-center justify-center py-8 gap-2 text-text-muted">
            <Loader className="w-4 h-4 animate-spin" />
            <span className="text-[12px]">Searching…</span>
          </div>
        )}

        {error && !loading && (
          <div className="flex items-start gap-2 px-3 py-3 text-red">
            <AlertCircle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
            <span className="text-[11px]">{error}</span>
          </div>
        )}

        {!loading && !error && results !== null && results.length === 0 && (
          <div className="flex flex-col items-center justify-center py-12 text-text-muted px-4">
            <span className="text-[12px] font-medium">No results</span>
            <span className="text-[11px] mt-1">Try a different query</span>
          </div>
        )}

        {!loading && !error && grouped && (
          <>
            {/* Summary */}
            <div className="px-3 py-1.5 border-b border-border/40 shrink-0">
              <span className="text-[10px] text-text-muted">
                {truncated ? `${count}+ results` : `${count} result${count !== 1 ? 's' : ''}`}
              </span>
            </div>

            {/* File groups */}
            {grouped.map(({ file, matches }) => (
              <FileGroup key={file} file={file} matches={matches} onResultClick={handleResultClick} />
            ))}
          </>
        )}

        {!loading && !error && results === null && !query && (
          <div className="flex flex-col items-center justify-center py-12 text-text-muted px-4">
            <Search className="w-8 h-8 mb-2 opacity-20" />
            <span className="text-[12px]">Type to search across files</span>
          </div>
        )}
      </div>
    </div>
  )
}

function FileGroup({ file, matches, onResultClick }) {
  const [open, setOpen] = useState(true)
  const filename = file.split('/').pop()
  const dir = file.includes('/') ? file.slice(0, file.lastIndexOf('/')) : ''

  return (
    <div className="border-b border-border/30">
      {/* File header */}
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center gap-1.5 px-2 py-1.5 hover:bg-bg-hover transition-colors text-left"
      >
        <ChevronRight className={`w-3 h-3 text-text-muted shrink-0 transition-transform ${open ? 'rotate-90' : ''}`} />
        <span className="text-[12px] font-medium text-text-primary truncate">{filename}</span>
        {dir && <span className="text-[10px] text-text-muted truncate shrink">{dir}</span>}
        <span className="ml-auto shrink-0 text-[10px] text-text-muted font-mono">{matches.length}</span>
      </button>

      {/* Match lines */}
      {open && (
        <div>
          {matches.map((m, i) => (
            <MatchLine key={i} match={m} onClick={() => onResultClick(m)} />
          ))}
        </div>
      )}
    </div>
  )
}

function MatchLine({ match, onClick }) {
  const text = match.text
  const start = match.matchStart
  const len = match.matchLen

  return (
    <button
      onClick={onClick}
      className="w-full flex items-baseline gap-2 px-4 py-1 hover:bg-bg-hover transition-colors text-left group overflow-hidden"
    >
      <span className="shrink-0 font-mono text-[10px] text-text-muted w-7 text-right">{match.line}</span>
      <span className="text-[11px] text-text-secondary font-mono truncate leading-relaxed">
        {text.slice(0, start)}
        <mark className="bg-yellow/25 text-text-primary not-italic rounded-sm">{text.slice(start, start + len)}</mark>
        {text.slice(start + len)}
      </span>
    </button>
  )
}

function groupByFile(matches) {
  const map = new Map()
  for (const m of matches) {
    if (!map.has(m.file)) map.set(m.file, [])
    map.get(m.file).push(m)
  }
  return Array.from(map.entries()).map(([file, matches]) => ({ file, matches }))
}
