// GitGraphView — the commit graph + history opened full-width in the editor area
// (like VS Code's Git Graph), instead of cramped in the sidebar. It fetches its
// own log + wires the commit context-menu actions, and reuses the GitGraph
// renderer from GitPanel (graph lanes, refs, commit detail/diff on click).

import { useState, useEffect, useCallback } from 'react'
import { GitBranch, RefreshCw, CloudDownload, Download, Upload } from 'lucide-react'
import { GitGraph } from './GitPanel'

const PAGE = 100

export default function GitGraphView({ authFetch, readOnly = false }) {
  const [log, setLog] = useState([])
  const [count, setCount] = useState(PAGE)
  const [loadingMore, setLoadingMore] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [op, setOp] = useState('')          // running fetch/pull/push label
  const [opMsg, setOpMsg] = useState(null)   // { text, error }

  const load = useCallback(async () => {
    try {
      const res = await authFetch(`/api/git/log?count=${count}`)
      const data = await res.json()
      setLog(data.entries || [])
    } catch { /* keep prior */ } finally {
      setLoadingMore(false)
      setRefreshing(false)
    }
  }, [authFetch, count])

  useEffect(() => { load() }, [load])

  const onLoadMore = () => { setLoadingMore(true); setCount((c) => c + PAGE) }
  const refresh = () => { setRefreshing(true); load() }

  const runOp = async (name, url) => {
    if (readOnly || op) return
    setOp(name); setOpMsg(null)
    try {
      const res = await authFetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })
      const data = await res.json().catch(() => ({}))
      if (data.error) setOpMsg({ text: `${name} failed: ${data.error}`, error: true })
      else { setOpMsg({ text: `${name} complete`, error: false }); load() }
    } catch {
      setOpMsg({ text: `${name} failed`, error: true })
    } finally {
      setOp('')
      setTimeout(() => setOpMsg(null), 4000)
    }
  }

  const onCommitAction = async (action, hash, extra) => {
    if (readOnly) return
    const post = (url, body) => authFetch(url, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
    })
    const actions = {
      checkout: () => post('/api/git/checkout', { branch: hash }),
      branchHere: () => post('/api/git/branch', { name: extra, checkout: true }),
      cherryPick: () => post('/api/git/cherry-pick', { hash }),
      revert: () => post('/api/git/revert', { hash }),
      resetSoft: () => post('/api/git/reset', { hash, mode: 'soft' }),
      resetHard: () => post('/api/git/reset', { hash, mode: 'hard' }),
    }
    if (actions[action]) { await actions[action](); load() }
  }

  return (
    <div className="h-full flex flex-col bg-bg-secondary min-w-0">
      <div className="px-3 py-2 border-b border-border flex items-center gap-2 shrink-0">
        <GitBranch className="w-3.5 h-3.5 text-text-muted" />
        <span className="text-[12px] font-semibold text-text-secondary">Git Graph</span>
        <span className="text-[11px] text-text-muted">{log.length} commit{log.length !== 1 ? 's' : ''}</span>
        {opMsg && (
          <span className={`text-[11px] truncate ${opMsg.error ? 'text-red' : 'text-green'}`}>· {opMsg.text}</span>
        )}
        <div className="flex-1" />
        {/* Fetch / Pull / Push — like VS Code's sync actions */}
        {!readOnly && (
          <div className="flex items-center gap-0.5">
            <button onClick={() => runOp('Fetch', '/api/git/fetch')} disabled={!!op} title="Fetch"
              className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors disabled:opacity-40">
              <CloudDownload className={`w-3.5 h-3.5 ${op === 'Fetch' ? 'animate-pulse' : ''}`} />
            </button>
            <button onClick={() => runOp('Pull', '/api/git/pull')} disabled={!!op} title="Pull"
              className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors disabled:opacity-40">
              <Download className={`w-3.5 h-3.5 ${op === 'Pull' ? 'animate-pulse' : ''}`} />
            </button>
            <button onClick={() => runOp('Push', '/api/git/push')} disabled={!!op} title="Push"
              className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors disabled:opacity-40">
              <Upload className={`w-3.5 h-3.5 ${op === 'Push' ? 'animate-pulse' : ''}`} />
            </button>
            <div className="w-px h-4 bg-border mx-0.5" />
          </div>
        )}
        <button onClick={refresh} title="Refresh"
          className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors">
          <RefreshCw className={`w-3.5 h-3.5 ${refreshing ? 'animate-spin' : ''}`} />
        </button>
      </div>
      <div className="flex-1 min-h-0">
        <GitGraph
          entries={log}
          authFetch={authFetch}
          onCommitAction={onCommitAction}
          readOnly={readOnly}
          totalCount={count}
          onLoadMore={onLoadMore}
          loadingMore={loadingMore}
        />
      </div>
    </div>
  )
}
