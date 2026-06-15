import { useState, useEffect, useCallback } from 'react'
import {
  Folder, FolderOpen, ChevronRight, ChevronUp,
  Clock, HardDrive, Home, MonitorCheck
} from 'lucide-react'
import Logo from './Logo'

export default function FolderPicker({ authFetch, onOpen, recents, inline }) {
  const [currentPath, setCurrentPath] = useState('')
  const [dirs, setDirs] = useState([])
  const [roots, setRoots] = useState([])
  const [parent, setParent] = useState('')
  const [loading, setLoading] = useState(false)
  const [manualPath, setManualPath] = useState('')

  const browse = useCallback(async (path) => {
    setLoading(true)
    try {
      const res = await authFetch(`/api/workspace/browse?path=${encodeURIComponent(path || '')}`)
      const data = await res.json()
      setCurrentPath(data.path)
      setDirs(data.dirs || [])
      setRoots(data.roots || [])
      setParent(data.parent || '')
      setManualPath(data.path)
    } catch { /* ignore */ }
    setLoading(false)
  }, [authFetch])

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => { browse('') }, [browse])
  /* eslint-enable react-hooks/set-state-in-effect */

  const handleOpen = async (path) => {
    try {
      const res = await authFetch('/api/workspace/open', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path }),
      })
      const data = await res.json()
      if (data.status === 'ok') {
        onOpen(data.current)
      }
    } catch { /* ignore */ }
  }

  const handleManualOpen = (e) => {
    e.preventDefault()
    if (manualPath.trim()) handleOpen(manualPath.trim())
  }

  const rootIcons = {
    Home: Home,
    Root: HardDrive,
  }

  const content = (
    <div className={inline ? '' : 'w-full max-w-xl'}>
      {/* Path input */}
      <form onSubmit={handleManualOpen} className="mb-4">
        <div className="flex gap-2">
          <input
            type="text"
            value={manualPath}
            onChange={(e) => setManualPath(e.target.value)}
            placeholder="/path/to/project"
            className="flex-1 bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent"
          />
          <button
            type="submit"
            className="px-4 py-2 bg-accent text-bg-tertiary text-sm font-medium rounded-lg hover:bg-accent-hover transition-colors"
          >
            Open
          </button>
        </div>
      </form>

      {/* Quick access roots */}
      {roots.length > 0 && (
        <div className="mb-4">
          <div className="flex flex-wrap gap-2">
            {roots.map((root) => {
              const Icon = rootIcons[root.name] || Folder
              return (
                <button
                  key={root.path}
                  onClick={() => browse(root.path)}
                  className="flex items-center gap-1.5 px-3 py-1.5 bg-bg-secondary border border-border rounded-lg text-xs text-text-secondary hover:text-accent hover:border-accent/50 transition-colors"
                >
                  <Icon className="w-3.5 h-3.5" />
                  {root.name}
                </button>
              )
            })}
          </div>
        </div>
      )}

      {/* Recent workspaces */}
      {recents && recents.length > 0 && (
        <div className="mb-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-text-muted mb-2 flex items-center gap-1.5">
            <Clock className="w-3.5 h-3.5" /> Recent
          </h3>
          <div className="space-y-1">
            {recents.map((path) => (
              <button
                key={path}
                onClick={() => handleOpen(path)}
                className="w-full flex items-center gap-2 px-3 py-2 bg-bg-secondary border border-border rounded-lg text-sm text-text-secondary hover:text-accent hover:border-accent/50 transition-colors text-left"
              >
                <FolderOpen className="w-4 h-4 text-yellow shrink-0" />
                <span className="truncate">{path}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Directory browser */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-text-muted">Browse</h3>
          {parent && (
            <button
              onClick={() => browse(parent)}
              className="flex items-center gap-1 text-xs text-text-muted hover:text-accent transition-colors"
            >
              <ChevronUp className="w-3 h-3" /> Up
            </button>
          )}
        </div>
        <div className="text-xs text-text-muted mb-2 font-mono truncate">{currentPath}</div>
        <div className="border border-border rounded-lg overflow-hidden max-h-64 overflow-y-auto">
          {loading ? (
            <div className="p-4 text-center text-text-muted text-sm">Loading...</div>
          ) : dirs.length === 0 ? (
            <div className="p-4 text-center text-text-muted text-sm">No subdirectories</div>
          ) : (
            dirs.map((dir) => (
              <div
                key={dir.path}
                className="flex items-center border-b border-border last:border-b-0 hover:bg-bg-hover transition-colors"
              >
                <button
                  onClick={() => browse(dir.path)}
                  className="flex-1 flex items-center gap-2 px-3 py-2 text-sm text-text-secondary hover:text-text-primary text-left"
                >
                  <Folder className="w-4 h-4 text-yellow shrink-0" />
                  <span className="truncate">{dir.name}</span>
                </button>
                <button
                  onClick={() => handleOpen(dir.path)}
                  className="px-3 py-2 text-xs text-accent hover:text-accent-hover font-medium transition-colors"
                >
                  Open
                </button>
              </div>
            ))
          )}
        </div>
        {/* Open current browsed directory */}
        <button
          onClick={() => handleOpen(currentPath)}
          className="mt-3 w-full flex items-center justify-center gap-2 px-4 py-2.5 bg-accent/10 border border-accent/30 text-accent text-sm font-medium rounded-lg hover:bg-accent/20 transition-colors"
        >
          <FolderOpen className="w-4 h-4" />
          Open "{currentPath.split('/').pop() || currentPath}"
        </button>
      </div>
    </div>
  )

  if (inline) return content

  return (
    <div className="min-h-screen bg-bg-tertiary flex items-center justify-center p-4">
      <div className="w-full max-w-xl">
        <div className="text-center mb-6">
          <div className="inline-flex items-center justify-center w-14 h-14 rounded-2xl bg-bg-primary border border-border mb-3 overflow-hidden">
            <Logo size={34} />
          </div>
          <h1 className="text-xl font-semibold text-text-primary">Open a Folder</h1>
          <p className="text-text-muted text-sm mt-1">Choose a project directory to get started</p>
        </div>
        <div className="bg-bg-primary border border-border rounded-xl p-6 shadow-xl">
          {content}
        </div>
      </div>
    </div>
  )
}
