import { useState, useEffect, useRef, useCallback } from 'react'
import {
  Search, File, Save, FolderOpen, Terminal as TerminalIcon,
  Settings as SettingsIcon, GitBranch, Globe, Files,
  Plus, Trash2, RefreshCw, Moon, Sun, LogOut, X
} from 'lucide-react'

/* ── Fuzzy match: returns score > 0 if query matches label, 0 otherwise ── */
function fuzzyScore(query, label) {
  if (!query) return 1
  const q = query.toLowerCase()
  const l = label.toLowerCase()
  // Exact substring — best match
  if (l.includes(q)) return 100 - l.indexOf(q)
  // All query chars present in order
  let qi = 0
  for (let i = 0; i < l.length && qi < q.length; i++) {
    if (l[i] === q[qi]) qi++
  }
  if (qi === q.length) return 50
  return 0
}

/* ── Highlight matched chars ── */
function HighlightMatch({ label, query }) {
  if (!query) return <span>{label}</span>
  const q = query.toLowerCase()
  const l = label.toLowerCase()
  const idx = l.indexOf(q)
  if (idx >= 0) {
    return (
      <span>
        {label.slice(0, idx)}
        <mark className="bg-accent/25 text-accent rounded-[2px] not-italic">{label.slice(idx, idx + q.length)}</mark>
        {label.slice(idx + q.length)}
      </span>
    )
  }
  // Fuzzy highlight
  const chars = []
  let qi = 0
  for (let i = 0; i < label.length; i++) {
    if (qi < q.length && label[i].toLowerCase() === q[qi]) {
      chars.push(<mark key={i} className="bg-accent/25 text-accent rounded-[2px] not-italic">{label[i]}</mark>)
      qi++
    } else {
      chars.push(label[i])
    }
  }
  return <span>{chars}</span>
}

export default function CommandPalette({
  visible,
  onClose,
  // IDE actions passed in as props
  onSaveFile,
  onSaveAll,
  onNewFile,
  onNewFolder,
  onOpenFolder,
  onToggleTerminal,
  onOpenSettings,
  onFocusExplorer,
  onFocusGit,
  onOpenBrowser,
  onToggleTheme,
  onCloseTab,
  onRefreshExplorer,
  onGitStageAll,
  onGitUnstageAll,
  onLogout,
  isDark,
  hasActiveTab,
  hasModified,
}) {
  const [query, setQuery] = useState('')
  const [selectedIdx, setSelectedIdx] = useState(0)
  const inputRef = useRef(null)
  const listRef = useRef(null)

  /* ── Command definitions ── */
  const commands = [
    hasActiveTab && {
      id: 'save',
      label: 'Save File',
      description: 'Save the active editor tab',
      icon: Save,
      shortcut: 'Ctrl/Cmd+S',
      action: onSaveFile,
    },
    hasModified && {
      id: 'save-all',
      label: 'Save All',
      description: 'Save all modified tabs',
      icon: Save,
      action: onSaveAll,
    },
    {
      id: 'new-file',
      label: 'New File',
      description: 'Create a new file in the explorer',
      icon: Plus,
      action: onNewFile,
    },
    {
      id: 'new-folder',
      label: 'New Folder',
      description: 'Create a new folder in the explorer',
      icon: FolderOpen,
      action: onNewFolder,
    },
    {
      id: 'open-folder',
      label: 'Open Folder…',
      description: 'Change the workspace directory',
      icon: FolderOpen,
      action: onOpenFolder,
    },
    {
      id: 'toggle-terminal',
      label: 'Toggle Terminal',
      description: 'Show or hide the terminal panel',
      icon: TerminalIcon,
      action: onToggleTerminal,
    },
    {
      id: 'open-settings',
      label: 'Open Settings',
      description: 'Show the settings panel',
      icon: SettingsIcon,
      action: onOpenSettings,
    },
    {
      id: 'focus-explorer',
      label: 'Focus File Explorer',
      description: 'Switch focus to the file explorer sidebar',
      icon: Files,
      action: onFocusExplorer,
    },
    {
      id: 'focus-git',
      label: 'Focus Source Control',
      description: 'Switch to the git panel',
      icon: GitBranch,
      action: onFocusGit,
    },
    {
      id: 'open-browser',
      label: 'Open Browser Preview',
      description: 'Open a browser preview tab',
      icon: Globe,
      action: onOpenBrowser,
    },
    hasActiveTab && {
      id: 'close-tab',
      label: 'Close Tab',
      description: 'Close the active editor tab',
      icon: X,
      action: onCloseTab,
    },
    {
      id: 'refresh-explorer',
      label: 'Refresh File Explorer',
      description: 'Reload the file tree from disk',
      icon: RefreshCw,
      action: onRefreshExplorer,
    },
    {
      id: 'git-stage-all',
      label: 'Git: Stage All Changes',
      description: 'Stage all modified files',
      icon: GitBranch,
      action: onGitStageAll,
    },
    {
      id: 'git-unstage-all',
      label: 'Git: Unstage All',
      description: 'Unstage all staged files',
      icon: GitBranch,
      action: onGitUnstageAll,
    },
    {
      id: 'toggle-theme',
      label: isDark ? 'Switch to Light Theme' : 'Switch to Dark Theme',
      description: isDark ? 'Activate the Daylight theme' : 'Activate the Midnight theme',
      icon: isDark ? Sun : Moon,
      action: onToggleTheme,
    },
    {
      id: 'logout',
      label: 'Log Out',
      description: 'End session and return to login',
      icon: LogOut,
      action: onLogout,
    },
  ].filter(Boolean)

  /* ── Filtered + ranked results ── */
  const results = commands
    .map((cmd) => ({ ...cmd, score: fuzzyScore(query, cmd.label) + fuzzyScore(query, cmd.description || '') * 0.3 }))
    .filter((cmd) => cmd.score > 0)
    .sort((a, b) => b.score - a.score)

  /* ── Reset selection when query changes, focus input when palette opens ── */
  // Using useEffect here matches the pattern used throughout the existing codebase.
  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => { setSelectedIdx(0) }, [query])
  useEffect(() => {
    if (visible) {
      setQuery('')
      setSelectedIdx(0)
      setTimeout(() => inputRef.current?.focus(), 20)
    }
  }, [visible])
  /* eslint-enable react-hooks/set-state-in-effect */

  /* ── Keyboard navigation ── */
  const handleKeyDown = useCallback((e) => {
    if (!visible) return
    if (e.key === 'Escape') { e.preventDefault(); onClose(); return }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setSelectedIdx((i) => Math.min(i + 1, results.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setSelectedIdx((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const cmd = results[selectedIdx]
      if (cmd) { onClose(); cmd.action?.() }
    }
  }, [visible, results, selectedIdx, onClose])

  /* ── Scroll selected item into view ── */
  useEffect(() => {
    const el = listRef.current?.children[selectedIdx]
    el?.scrollIntoView({ block: 'nearest' })
  }, [selectedIdx])

  if (!visible) return null

  return (
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center pt-[12vh] bg-black/60 backdrop-blur-[2px]"
      onMouseDown={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div
        className="w-full max-w-[600px] mx-4 bg-bg-elevated border border-border rounded-xl shadow-2xl shadow-shadow-lg overflow-hidden animate-fade-in"
        onKeyDown={handleKeyDown}
      >
        {/* Search input */}
        <div className="flex items-center gap-2.5 px-4 py-3 border-b border-border">
          <Search className="w-4 h-4 text-text-muted shrink-0" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Type a command…"
            className="flex-1 bg-transparent text-sm text-text-primary placeholder:text-text-muted focus:outline-none"
          />
          <kbd className="hidden sm:block text-[10px] font-mono text-text-muted bg-bg-active px-1.5 py-0.5 rounded border border-border shrink-0">
            Esc
          </kbd>
        </div>

        {/* Results list */}
        <div ref={listRef} className="overflow-y-auto max-h-[360px] py-1">
          {results.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-10 text-text-muted">
              <File className="w-6 h-6 mb-2 opacity-30" />
              <span className="text-[12px]">No commands match</span>
            </div>
          ) : (
            results.map((cmd, i) => {
              const Icon = cmd.icon
              const isSelected = i === selectedIdx
              return (
                <button
                  key={cmd.id}
                  className={`w-full flex items-center gap-3 px-4 py-2.5 text-left transition-colors ${
                    isSelected
                      ? 'bg-accent/10 text-text-primary'
                      : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'
                  }`}
                  onMouseEnter={() => setSelectedIdx(i)}
                  onClick={() => { onClose(); cmd.action?.() }}
                >
                  <div className={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 ${
                    isSelected ? 'bg-accent/15' : 'bg-bg-active'
                  }`}>
                    <Icon className={`w-3.5 h-3.5 ${isSelected ? 'text-accent' : 'text-text-muted'}`} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="text-[13px] font-medium leading-tight truncate">
                      <HighlightMatch label={cmd.label} query={query} />
                    </div>
                    {cmd.description && (
                      <div className="text-[11px] text-text-muted truncate mt-0.5">{cmd.description}</div>
                    )}
                  </div>
                  {cmd.shortcut && (
                    <kbd className="shrink-0 text-[10px] font-mono text-text-muted bg-bg-active px-1.5 py-0.5 rounded border border-border">
                      {cmd.shortcut}
                    </kbd>
                  )}
                </button>
              )
            })
          )}
        </div>
      </div>
    </div>
  )
}
