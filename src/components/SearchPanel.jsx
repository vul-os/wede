import { useState, useCallback, useRef, useMemo } from 'react'
import {
  Search, X, ChevronRight, Loader, AlertCircle,
  CaseSensitive, Regex, PenLine, RefreshCw, WholeWord,
  FileSearch, SlidersHorizontal,
} from 'lucide-react'

// Context lines requested from the backend on each text search.
const CONTEXT_LINES = 2

// ── Main panel ────────────────────────────────────────────────────────────────

/**
 * SearchPanel — VS Code–grade project search.
 *
 * Props:
 *   authFetch  (function)  – authenticated fetch wrapper
 *   onOpenFile (function)  – called with (entry, line) to open a file
 *   readOnly   (boolean)   – when true the replace UI is hidden entirely
 */
export default function SearchPanel({ authFetch, onOpenFile, readOnly = false }) {
  // ── search state ────────────────────────────────────────────────────────────
  const [query, setQuery]               = useState('')
  const [caseSensitive, setCaseSensitive] = useState(false)
  const [wholeWord, setWholeWord]       = useState(false)
  const [useRegex, setUseRegex]         = useState(false)
  const [includeGlob, setIncludeGlob]   = useState('')
  const [excludeGlob, setExcludeGlob]   = useState('')
  const [showGlobs, setShowGlobs]       = useState(false)
  const [searchMode, setSearchMode]     = useState('text') // 'text' | 'files'

  // ── result state ────────────────────────────────────────────────────────────
  const [results, setResults]       = useState(null) // null = no search yet; [] = empty
  const [fileResults, setFileResults] = useState(null)
  const [loading, setLoading]       = useState(false)
  const [error, setError]           = useState('')
  const [truncated, setTruncated]   = useState(false)
  const [count, setCount]           = useState(0)
  const [fileCount, setFileCount]   = useState(0)

  // ── replace state (hidden when readOnly) ────────────────────────────────────
  const [replaceMode, setReplaceMode]         = useState(false)
  const [replaceQuery, setReplaceQuery]       = useState('')
  const [replacing, setReplacing]             = useState(false)
  const [replaceResult, setReplaceResult]     = useState(null)
  const [replaceError, setReplaceError]       = useState('')
  const [replacePreviewing, setReplacePreviewing] = useState(false)
  const [previewResults, setPreviewResults]   = useState(null)

  // ── refs ────────────────────────────────────────────────────────────────────
  const abortRef      = useRef(null)
  const debounceRef   = useRef(null)
  const queryInputRef = useRef(null)
  // Flat array of refs to result buttons for keyboard navigation.
  const resultButtonsRef = useRef([])

  // ── derived ─────────────────────────────────────────────────────────────────
  const grouped        = useMemo(() => results      ? groupByFile(results)      : null, [results])
  const previewGrouped = useMemo(() => previewResults ? groupByFile(previewResults) : null, [previewResults])

  const displayResults = previewResults ?? results
  const displayGrouped = previewResults ? previewGrouped : grouped
  const isPreviewMode  = !!previewResults

  // Flattened matches for keyboard navigation (text mode only).
  const flatMatches = useMemo(() => {
    if (!displayResults || searchMode === 'files') return []
    return displayResults
  }, [displayResults, searchMode])

  // ── core search ─────────────────────────────────────────────────────────────
  const doSearch = useCallback(async (q, cs, ww, rx, inc, exc, mode) => {
    if (!q.trim()) {
      setResults(null)
      setFileResults(null)
      setError('')
      setPreviewResults(null)
      return
    }

    if (abortRef.current) abortRef.current.abort()
    const ctrl = new AbortController()
    abortRef.current = ctrl

    setLoading(true)
    setError('')
    setPreviewResults(null)
    resultButtonsRef.current = []

    try {
      if (mode === 'files') {
        const params = new URLSearchParams({ q: q.trim() })
        if (cs)  params.set('case',    'true')
        if (rx)  params.set('regex',   'true')
        if (inc) params.set('include', inc)
        if (exc) params.set('exclude', exc)
        const res  = await authFetch(`/api/search/files?${params}`, { signal: ctrl.signal })
        const data = await res.json()
        if (data.error) {
          setError(data.error)
          setFileResults(null)
        } else {
          setFileResults(data.files || [])
          setCount(data.count || 0)
          setTruncated(data.truncated || false)
          setResults(null)
        }
      } else {
        const params = new URLSearchParams({ q: q.trim() })
        if (cs)  params.set('case',    'true')
        if (ww)  params.set('word',    'true')
        if (rx)  params.set('regex',   'true')
        if (inc) params.set('include', inc)
        if (exc) params.set('exclude', exc)
        params.set('context', String(CONTEXT_LINES))
        const res  = await authFetch(`/api/search?${params}`, { signal: ctrl.signal })
        const data = await res.json()
        if (data.error) {
          setError(data.error)
          setResults(null)
        } else {
          setResults(data.matches || [])
          setTruncated(data.truncated || false)
          setCount(data.count || 0)
          setFileCount(data.fileCount || 0)
          setFileResults(null)
        }
      }
    } catch (e) {
      if (e.name !== 'AbortError') setError('Search failed')
    }
    setLoading(false)
  }, [authFetch])

  // Debounced trigger — cancels previous timer on each keystroke.
  const triggerSearch = useCallback((q, cs, ww, rx, inc, exc, mode) => {
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => doSearch(q, cs, ww, rx, inc, exc, mode), 350)
  }, [doSearch])

  // ── input handlers ───────────────────────────────────────────────────────────
  const handleQueryChange = (e) => {
    const v = e.target.value
    setQuery(v)
    triggerSearch(v, caseSensitive, wholeWord, useRegex, includeGlob, excludeGlob, searchMode)
  }

  const handleQueryKeyDown = (e) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      clearTimeout(debounceRef.current)
      doSearch(query, caseSensitive, wholeWord, useRegex, includeGlob, excludeGlob, searchMode)
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      const first = resultButtonsRef.current[0]
      if (first) first.focus()
    } else if (e.key === 'Escape') {
      clear()
    }
  }

  const toggleCase = () => {
    const v = !caseSensitive
    setCaseSensitive(v)
    triggerSearch(query, v, wholeWord, useRegex, includeGlob, excludeGlob, searchMode)
  }

  const toggleWholeWord = () => {
    const v = !wholeWord
    setWholeWord(v)
    triggerSearch(query, caseSensitive, v, useRegex, includeGlob, excludeGlob, searchMode)
  }

  const toggleRegex = () => {
    const v = !useRegex
    setUseRegex(v)
    triggerSearch(query, caseSensitive, wholeWord, v, includeGlob, excludeGlob, searchMode)
  }

  const handleIncludeChange = (e) => {
    const v = e.target.value
    setIncludeGlob(v)
    triggerSearch(query, caseSensitive, wholeWord, useRegex, v, excludeGlob, searchMode)
  }

  const handleExcludeChange = (e) => {
    const v = e.target.value
    setExcludeGlob(v)
    triggerSearch(query, caseSensitive, wholeWord, useRegex, includeGlob, v, searchMode)
  }

  const switchMode = (mode) => {
    setSearchMode(mode)
    setResults(null)
    setFileResults(null)
    setPreviewResults(null)
    setError('')
    if (query.trim()) {
      triggerSearch(query, caseSensitive, wholeWord, useRegex, includeGlob, excludeGlob, mode)
    }
  }

  const clear = () => {
    setQuery('')
    setResults(null)
    setFileResults(null)
    setError('')
    setPreviewResults(null)
    setReplaceResult(null)
    setReplaceError('')
    queryInputRef.current?.focus()
  }

  // ── result click / keyboard ──────────────────────────────────────────────────
  const handleResultClick = (match) => {
    onOpenFile?.({ path: match.file, name: match.file.split('/').pop(), isDir: false }, match.line)
  }

  const handleFileResultClick = (fileMatch) => {
    onOpenFile?.({ path: fileMatch.path, name: fileMatch.path.split('/').pop(), isDir: false }, 1)
  }

  // Navigate result buttons with arrow keys; Escape returns focus to query.
  const handleResultKeyDown = (e, idx) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      const next = resultButtonsRef.current[idx + 1]
      if (next) next.focus()
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (idx === 0) {
        queryInputRef.current?.focus()
      } else {
        const prev = resultButtonsRef.current[idx - 1]
        if (prev) prev.focus()
      }
    } else if (e.key === 'Escape') {
      queryInputRef.current?.focus()
    }
  }

  // ── replace handlers ─────────────────────────────────────────────────────────
  const handleReplacePreview = useCallback(async () => {
    if (!query.trim()) return
    setReplacePreviewing(true)
    setReplaceError('')
    setReplaceResult(null)
    try {
      const params = new URLSearchParams({ q: query.trim(), replace: replaceQuery })
      if (caseSensitive) params.set('case',  'true')
      if (wholeWord)     params.set('word',  'true')
      if (useRegex)      params.set('regex', 'true')
      if (includeGlob)   params.set('include', includeGlob)
      if (excludeGlob)   params.set('exclude', excludeGlob)
      const res  = await authFetch(`/api/search/replace-preview?${params}`)
      const data = await res.json()
      if (data.error) {
        setReplaceError(data.error)
      } else {
        setPreviewResults(data.matches || [])
      }
    } catch (e) {
      setReplaceError(e.message || 'Preview failed')
    }
    setReplacePreviewing(false)
  }, [authFetch, query, replaceQuery, caseSensitive, wholeWord, useRegex, includeGlob, excludeGlob])

  const handleReplaceAll = useCallback(async () => {
    if (!query.trim()) return
    setReplacing(true)
    setReplaceError('')
    setReplaceResult(null)
    setPreviewResults(null)
    try {
      const res  = await authFetch('/api/search/replace', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({
          query:         query.trim(),
          replace:       replaceQuery,
          caseSensitive,
          wholeWord,
          useRegex,
          includeGlob,
          excludeGlob,
          paths: [],
        }),
      })
      const data = await res.json()
      if (data.error) {
        setReplaceError(data.error)
      } else {
        setReplaceResult({ filesChanged: data.filesChanged, replacements: data.replacements })
        doSearch(query, caseSensitive, wholeWord, useRegex, includeGlob, excludeGlob, searchMode)
      }
    } catch (e) {
      setReplaceError(e.message || 'Replace failed')
    }
    setReplacing(false)
  }, [authFetch, query, replaceQuery, caseSensitive, wholeWord, useRegex, includeGlob, excludeGlob, searchMode, doSearch])

  const toggleReplaceMode = () => {
    setReplaceMode((v) => !v)
    setPreviewResults(null)
    setReplaceResult(null)
    setReplaceError('')
  }

  // ── summary line ─────────────────────────────────────────────────────────────
  const summaryText = (() => {
    if (isPreviewMode) {
      return `Preview: ${displayResults.length} replacement${displayResults.length !== 1 ? 's' : ''}`
    }
    if (searchMode === 'files') {
      const n = fileResults?.length ?? 0
      return truncated ? `${n}+ files` : `${n} file${n !== 1 ? 's' : ''}`
    }
    const suffix = truncated ? `${count}+ results` : `${count} result${count !== 1 ? 's' : ''}`
    return fileCount > 0 ? `${suffix} in ${fileCount} file${fileCount !== 1 ? 's' : ''}` : suffix
  })()

  // Whether there are any results to show a summary for.
  const hasDisplay = searchMode === 'files'
    ? fileResults !== null && fileResults.length > 0
    : displayGrouped !== null

  // ── render ────────────────────────────────────────────────────────────────────
  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden">

      {/* ── Header ── */}
      <div className="px-3 py-2 border-b border-border shrink-0 flex items-center justify-between gap-2">
        {/* Text / Files mode tabs */}
        <div className="flex items-center gap-0.5">
          <button
            onClick={() => switchMode('text')}
            title="Search file contents"
            className={`flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-semibold uppercase tracking-wide transition-colors ${
              searchMode === 'text'
                ? 'text-text-primary bg-bg-hover'
                : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'
            }`}
          >
            <Search className="w-3 h-3" />
            Text
          </button>
          <button
            onClick={() => switchMode('files')}
            title="Search file names"
            className={`flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-semibold uppercase tracking-wide transition-colors ${
              searchMode === 'files'
                ? 'text-text-primary bg-bg-hover'
                : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'
            }`}
          >
            <FileSearch className="w-3 h-3" />
            Files
          </button>
        </div>

        <div className="flex items-center gap-0.5 ml-auto">
          {/* Glob filter toggle */}
          <button
            onClick={() => setShowGlobs((v) => !v)}
            title="Filter by glob patterns"
            className={`p-1 rounded transition-colors ${
              showGlobs || includeGlob || excludeGlob
                ? 'text-accent bg-accent/10'
                : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'
            }`}
          >
            <SlidersHorizontal className="w-3.5 h-3.5" />
          </button>

          {/* Replace toggle — hidden in readOnly mode or when in Files mode */}
          {!readOnly && searchMode === 'text' && (
            <button
              onClick={toggleReplaceMode}
              title="Toggle replace mode"
              className={`p-1 rounded transition-colors ${
                replaceMode
                  ? 'text-accent bg-accent/10'
                  : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'
              }`}
            >
              <PenLine className="w-3.5 h-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* ── Search input + toggles ── */}
      <div className="px-3 py-2 border-b border-border shrink-0 space-y-2">
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-text-muted pointer-events-none" />
          <input
            ref={queryInputRef}
            type="text"
            value={query}
            onChange={handleQueryChange}
            onKeyDown={handleQueryKeyDown}
            placeholder={searchMode === 'files' ? 'Search file names…' : 'Search files… (Enter to run)'}
            className="w-full pl-8 pr-20 py-1.5 text-[12px] bg-bg-input border border-border rounded-md text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 focus:ring-1 focus:ring-accent/20 transition-colors"
          />
          <div className="absolute right-2 top-1/2 -translate-y-1/2 flex items-center gap-0.5">
            <button
              onClick={toggleCase}
              title="Match case (Alt+C)"
              className={`p-1 rounded transition-colors ${caseSensitive ? 'text-accent bg-accent/10' : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'}`}
            >
              <CaseSensitive className="w-3 h-3" />
            </button>
            {/* Whole-word toggle hidden in Files mode (not meaningful for paths) */}
            {searchMode === 'text' && (
              <button
                onClick={toggleWholeWord}
                title="Match whole word (Alt+W)"
                className={`p-1 rounded transition-colors ${wholeWord ? 'text-accent bg-accent/10' : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'}`}
              >
                <WholeWord className="w-3 h-3" />
              </button>
            )}
            <button
              onClick={toggleRegex}
              title="Use regular expression (Alt+R)"
              className={`p-1 rounded transition-colors ${useRegex ? 'text-accent bg-accent/10' : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'}`}
            >
              <Regex className="w-3 h-3" />
            </button>
            {query && (
              <button onClick={clear} title="Clear" className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors">
                <X className="w-3 h-3" />
              </button>
            )}
          </div>
        </div>

        {/* Glob filter rows */}
        {showGlobs && (
          <div className="space-y-1">
            <div className="relative">
              <span className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[9px] font-mono text-text-muted pointer-events-none select-none">incl</span>
              <input
                type="text"
                value={includeGlob}
                onChange={handleIncludeChange}
                placeholder="Include glob, e.g. *.go"
                className="w-full pl-8 pr-2 py-1 text-[11px] bg-bg-input border border-border rounded text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 transition-colors"
              />
            </div>
            <div className="relative">
              <span className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[9px] font-mono text-text-muted pointer-events-none select-none">excl</span>
              <input
                type="text"
                value={excludeGlob}
                onChange={handleExcludeChange}
                placeholder="Exclude glob, e.g. *.test.js"
                className="w-full pl-8 pr-2 py-1 text-[11px] bg-bg-input border border-border rounded text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 transition-colors"
              />
            </div>
          </div>
        )}

        {/* Replace UI — only when not readOnly, text mode, and replace toggle is on */}
        {!readOnly && searchMode === 'text' && replaceMode && (
          <div className="space-y-2 pt-0.5">
            <input
              type="text"
              value={replaceQuery}
              onChange={(e) => { setReplaceQuery(e.target.value); setPreviewResults(null); setReplaceResult(null) }}
              placeholder="Replace with…"
              className="w-full px-3 py-1.5 text-[12px] bg-bg-input border border-border rounded-md text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 focus:ring-1 focus:ring-accent/20 transition-colors"
            />
            <div className="flex gap-1.5">
              <button
                onClick={handleReplacePreview}
                disabled={!query.trim() || replacePreviewing}
                className="flex-1 flex items-center justify-center gap-1 py-1 text-[11px] border border-border rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-hover disabled:opacity-40 disabled:cursor-not-allowed transition-colors font-medium"
              >
                {replacePreviewing
                  ? <RefreshCw className="w-3 h-3 animate-spin" />
                  : <Search className="w-3 h-3" />
                }
                Preview
              </button>
              <button
                onClick={handleReplaceAll}
                disabled={!query.trim() || replacing}
                className="flex-1 flex items-center justify-center gap-1 py-1 text-[11px] bg-accent text-white rounded-md hover:bg-accent-hover disabled:opacity-40 disabled:cursor-not-allowed transition-colors font-medium shadow-sm shadow-accent/20"
              >
                {replacing
                  ? <RefreshCw className="w-3 h-3 animate-spin" />
                  : <PenLine className="w-3 h-3" />
                }
                Replace All
              </button>
            </div>

            {replaceResult && (
              <div className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-md text-[11px] bg-green/8 border border-green/20 text-green animate-fade-in">
                <span>
                  Replaced {replaceResult.replacements} match{replaceResult.replacements !== 1 ? 'es' : ''} in {replaceResult.filesChanged} file{replaceResult.filesChanged !== 1 ? 's' : ''}
                </span>
              </div>
            )}
            {replaceError && (
              <div className="flex items-start gap-1.5 px-2.5 py-1.5 rounded-md text-[11px] bg-red/8 border border-red/20 text-red animate-fade-in">
                <AlertCircle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
                <span>{replaceError}</span>
              </div>
            )}
          </div>
        )}
      </div>

      {/* ── Results area ── */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden min-h-0">
        {/* Busy indicator */}
        {loading && (
          <div className="flex items-center justify-center py-8 gap-2 text-text-muted">
            <Loader className="w-4 h-4 animate-spin" />
            <span className="text-[12px]">Searching…</span>
          </div>
        )}

        {/* Error */}
        {error && !loading && (
          <div className="flex items-start gap-2 px-3 py-3 text-red">
            <AlertCircle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
            <span className="text-[11px]">{error}</span>
          </div>
        )}

        {/* Empty state after a search */}
        {!loading && !error && searchMode === 'text' && results !== null && results.length === 0 && (
          <EmptyResults />
        )}
        {!loading && !error && searchMode === 'files' && fileResults !== null && fileResults.length === 0 && (
          <EmptyResults />
        )}

        {/* ── Text search results ── */}
        {!loading && !error && searchMode === 'text' && hasDisplay && (
          <>
            <div className="px-3 py-1.5 border-b border-border/40 shrink-0 flex items-center justify-between">
              <span className="text-[10px] text-text-muted">{summaryText}</span>
              {isPreviewMode && (
                <span className="text-[10px] text-amber-400 font-medium">preview</span>
              )}
            </div>

            {displayGrouped && displayGrouped.map(({ file, matches }, groupIdx) => (
              <FileGroup
                key={file}
                file={file}
                matches={matches}
                groupStartIdx={groupIdx === 0
                  ? 0
                  : displayGrouped.slice(0, groupIdx).reduce((acc, g) => acc + g.matches.length, 0)}
                onResultClick={handleResultClick}
                onResultKeyDown={handleResultKeyDown}
                resultButtonsRef={resultButtonsRef}
                isPreview={isPreviewMode}
                totalMatches={flatMatches.length}
              />
            ))}
          </>
        )}

        {/* ── Filename search results ── */}
        {!loading && !error && searchMode === 'files' && fileResults !== null && fileResults.length > 0 && (
          <>
            <div className="px-3 py-1.5 border-b border-border/40 shrink-0">
              <span className="text-[10px] text-text-muted">{summaryText}</span>
            </div>
            {fileResults.map((fm, idx) => (
              <FileResultRow
                key={fm.path}
                fileMatch={fm}
                idx={idx}
                query={query}
                caseSensitive={caseSensitive}
                onClick={() => handleFileResultClick(fm)}
                onKeyDown={(e) => handleResultKeyDown(e, idx)}
                buttonRef={(el) => { resultButtonsRef.current[idx] = el }}
              />
            ))}
          </>
        )}

        {/* Initial empty state */}
        {!loading && !error && results === null && fileResults === null && (
          <div className="flex flex-col items-center justify-center py-12 text-text-muted px-4">
            <Search className="w-8 h-8 mb-2 opacity-20" />
            <span className="text-[12px]">
              {searchMode === 'files' ? 'Type to search file names' : 'Type to search across files'}
            </span>
            {!readOnly && searchMode === 'text' && replaceMode && (
              <span className="text-[11px] mt-1 text-center">Enter replacement text above, then Preview or Replace All</span>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

// ── FileGroup ─────────────────────────────────────────────────────────────────

function FileGroup({ file, matches, groupStartIdx, onResultClick, onResultKeyDown, resultButtonsRef, isPreview }) {
  const [open, setOpen] = useState(true)
  const filename = file.split('/').pop()
  const dir      = file.includes('/') ? file.slice(0, file.lastIndexOf('/')) : ''

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
          {matches.map((m, i) => {
            const globalIdx = groupStartIdx + i
            return (
              <MatchLine
                key={i}
                match={m}
                globalIdx={globalIdx}
                onClick={() => onResultClick(m)}
                onKeyDown={(e) => onResultKeyDown(e, globalIdx)}
                buttonRef={(el) => { resultButtonsRef.current[globalIdx] = el }}
                isPreview={isPreview}
              />
            )
          })}
        </div>
      )}
    </div>
  )
}

// ── MatchLine ─────────────────────────────────────────────────────────────────

function MatchLine({ match, onClick, onKeyDown, buttonRef, isPreview }) {
  const text  = match.text
  const start = match.matchStart
  const len   = match.matchLen

  return (
    <div className="group">
      {/* Before-context lines */}
      {match.before && match.before.map((ctx, i) => (
        <div key={`b${i}`} className="flex items-baseline gap-2 px-4 py-0.5 overflow-hidden">
          <span className="shrink-0 font-mono text-[10px] text-text-muted/40 w-7 text-right select-none">
            {match.line - match.before.length + i}
          </span>
          <span className="text-[11px] text-text-muted/50 font-mono truncate leading-relaxed">{ctx}</span>
        </div>
      ))}

      {/* Match line */}
      <button
        ref={buttonRef}
        onClick={onClick}
        onKeyDown={onKeyDown}
        className="w-full flex items-baseline gap-2 px-4 py-1 hover:bg-bg-hover focus:bg-bg-hover focus:outline-none transition-colors text-left overflow-hidden"
      >
        <span className="shrink-0 font-mono text-[10px] text-text-muted w-7 text-right">{match.line}</span>
        <span className="text-[11px] text-text-secondary font-mono truncate leading-relaxed">
          {isPreview && match.replacedText
            ? <span className="bg-amber-400/20 text-amber-300 rounded-sm">{match.replacedText}</span>
            : (
              <>
                {text.slice(0, start)}
                <mark className="bg-yellow/25 text-text-primary not-italic rounded-sm">
                  {text.slice(start, start + len)}
                </mark>
                {text.slice(start + len)}
              </>
            )
          }
        </span>
      </button>

      {/* After-context lines */}
      {match.after && match.after.map((ctx, i) => (
        <div key={`a${i}`} className="flex items-baseline gap-2 px-4 py-0.5 overflow-hidden">
          <span className="shrink-0 font-mono text-[10px] text-text-muted/40 w-7 text-right select-none">
            {match.line + i + 1}
          </span>
          <span className="text-[11px] text-text-muted/50 font-mono truncate leading-relaxed">{ctx}</span>
        </div>
      ))}
    </div>
  )
}

// ── FileResultRow (filename search) ───────────────────────────────────────────

function FileResultRow({ fileMatch, idx, query, caseSensitive, onClick, onKeyDown, buttonRef }) {
  const parts = fileMatch.path.split('/')
  const filename = parts.pop()
  const dir      = parts.join('/')

  // Highlight the query within the filename portion.
  const hiFilename = highlightText(filename, query, caseSensitive)

  return (
    <button
      ref={buttonRef}
      onClick={onClick}
      onKeyDown={onKeyDown}
      className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-bg-hover focus:bg-bg-hover focus:outline-none transition-colors text-left overflow-hidden"
    >
      <span className="text-[10px] text-text-muted font-mono shrink-0 w-5 text-right">{idx + 1}</span>
      <span className="text-[12px] font-medium text-text-primary truncate">{hiFilename}</span>
      {dir && <span className="text-[10px] text-text-muted truncate shrink">{dir}</span>}
    </button>
  )
}

// ── EmptyResults ──────────────────────────────────────────────────────────────

function EmptyResults() {
  return (
    <div className="flex flex-col items-center justify-center py-12 text-text-muted px-4">
      <span className="text-[12px] font-medium">No results</span>
      <span className="text-[11px] mt-1">Try a different query</span>
    </div>
  )
}

// ── Utilities ─────────────────────────────────────────────────────────────────

function groupByFile(matches) {
  const map = new Map()
  for (const m of matches) {
    if (!map.has(m.file)) map.set(m.file, [])
    map.get(m.file).push(m)
  }
  return Array.from(map.entries()).map(([file, ms]) => ({ file, matches: ms }))
}

/**
 * Returns a React element with the first occurrence of `query` in `text`
 * wrapped in a <mark>. Falls back to plain text if query is empty.
 */
function highlightText(text, query, caseSensitive) {
  if (!query) return <>{text}</>
  const hay    = caseSensitive ? text  : text.toLowerCase()
  const needle = caseSensitive ? query : query.toLowerCase()
  const idx    = hay.indexOf(needle)
  if (idx < 0) return <>{text}</>
  return (
    <>
      {text.slice(0, idx)}
      <mark className="bg-yellow/25 text-text-primary not-italic rounded-sm">
        {text.slice(idx, idx + query.length)}
      </mark>
      {text.slice(idx + query.length)}
    </>
  )
}
