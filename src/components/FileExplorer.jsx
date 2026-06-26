import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import {
  ChevronRight, ChevronDown, File, Folder, FolderOpen,
  FilePlus, FolderPlus, RefreshCw, Copy, Clipboard, Trash2, Pencil
} from 'lucide-react'

/* ── File type icon colours (VS Code style) ── */
const EXT_ICON = {
  js:         { color: '#f7df1e', label: 'JS' },
  jsx:        { color: '#61dafb', label: 'JSX' },
  ts:         { color: '#3178c6', label: 'TS' },
  tsx:        { color: '#3178c6', label: 'TSX' },
  go:         { color: '#00add8', label: 'GO' },
  py:         { color: '#3776ab', label: 'PY' },
  rs:         { color: '#dea584', label: 'RS' },
  rb:         { color: '#cc342d', label: 'RB' },
  java:       { color: '#ed8b00', label: 'JA' },
  php:        { color: '#777bb4', label: 'PHP' },
  c:          { color: '#a8b9cc', label: 'C' },
  cpp:        { color: '#00599c', label: 'C++' },
  h:          { color: '#a8b9cc', label: 'H' },
  html:       { color: '#e34f26', label: '<>' },
  htm:        { color: '#e34f26', label: '<>' },
  css:        { color: '#1572b6', label: '#' },
  scss:       { color: '#cf649a', label: 'SC' },
  json:       { color: '#f7df1e', label: '{}' },
  xml:        { color: '#e37933', label: 'XML' },
  md:         { color: '#519aba', label: 'MD' },
  txt:        { color: '#8b91ab', label: 'TXT' },
  svg:        { color: '#f7a41d', label: 'SVG' },
  yml:        { color: '#cb171e', label: 'YML' },
  yaml:       { color: '#cb171e', label: 'YML' },
  toml:       { color: '#9c4121', label: 'TM' },
  sql:        { color: '#e38c00', label: 'SQL' },
  sh:         { color: '#4eaa25', label: 'SH' },
  bash:       { color: '#4eaa25', label: 'SH' },
  mod:        { color: '#00add8', label: 'MOD' },
  sum:        { color: '#00add8', label: 'SUM' },
  lock:       { color: '#8b91ab', label: 'LK' },
  env:        { color: '#ecd53f', label: 'ENV' },
  gitignore:  { color: '#f05032', label: 'GI' },
}

function FileIcon({ name }) {
  const ext = name.includes('.') ? name.split('.').pop().toLowerCase() : name.toLowerCase()
  const info = EXT_ICON[ext]
  if (info) {
    return (
      <span
        className="w-[18px] h-[18px] flex items-center justify-center rounded-[3px] text-[7px] font-bold shrink-0 leading-none select-none"
        style={{ backgroundColor: info.color + '22', color: info.color }}
      >
        {info.label}
      </span>
    )
  }
  return <File className="w-[14px] h-[14px] shrink-0 text-text-muted" />
}

/* ── Git status styling ── */
const GIT_COLOR = {
  modified:  'text-yellow',
  added:     'text-green',
  deleted:   'text-red line-through',
  untracked: 'text-green',
  renamed:   'text-accent',
}

const GIT_BADGE = {
  modified:  { label: 'M', cls: 'text-yellow' },
  added:     { label: 'A', cls: 'text-green' },
  deleted:   { label: 'D', cls: 'text-red' },
  untracked: { label: 'U', cls: 'text-green' },
  renamed:   { label: 'R', cls: 'text-accent' },
}

/* ── Context menu ── */
function ContextMenu({ x, y, items, onClose }) {
  const ref = useRef(null)

  useEffect(() => {
    const handler = (e) => { if (!ref.current?.contains(e.target)) onClose() }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [onClose])

  const clamped = { left: Math.min(x, window.innerWidth - 180), top: Math.min(y, window.innerHeight - 200) }

  return (
    <div ref={ref} className="fixed z-50 animate-fade-in" style={clamped}>
      <div className="bg-bg-elevated border border-border rounded-lg shadow-xl shadow-shadow-lg py-1.5 min-w-[160px]">
        {items.map((item, i) => item.separator ? (
          <div key={i} className="border-t border-border/60 my-1" />
        ) : (
          <button key={i} onClick={() => { item.action(); onClose() }}
            className="w-full flex items-center gap-2.5 px-3 py-1.5 text-[12px] text-text-secondary hover:bg-bg-hover hover:text-text-primary transition-colors text-left">
            {item.icon && <item.icon className="w-3.5 h-3.5 text-text-muted shrink-0" />}
            <span>{item.label}</span>
          </button>
        ))}
      </div>
    </div>
  )
}

/* ── Inline input for new file / rename ── */
function InlineInput({ placeholder, value, onChange, onSubmit, onBlur }) {
  return (
    <form onSubmit={onSubmit} className="px-2 py-1.5 border-b border-border bg-bg-primary">
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full bg-bg-input border border-accent/50 rounded-md px-2.5 py-1 text-[12px] text-text-primary focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent/20 transition-colors"
        autoFocus
        onBlur={onBlur}
      />
    </form>
  )
}

/* ── Tree node ── */
function TreeNode({
  entry, depth, onSelect, onToggle, expanded, authFetch,
  onRefresh, selectedPath, gitMap, presenceMap, clipboard, setClipboard,
  onPaste, onDelete, onRename, onFocusDir,
}) {
  const [children, setChildren] = useState(null)
  const [ctx, setCtx] = useState(null)
  const isOpen     = expanded.has(entry.path)
  const isSelected = selectedPath === entry.path
  const gitStatus  = gitMap?.[entry.path]
  const viewers    = presenceMap?.[entry.path]
  const nameColor  = gitStatus ? GIT_COLOR[gitStatus] : 'text-text-primary'
  const badge      = gitStatus ? GIT_BADGE[gitStatus] : null

  const loadChildren = useCallback(async () => {
    if (!entry.isDir) return
    try {
      const res = await authFetch(`/api/files?path=${encodeURIComponent(entry.path)}`)
      const data = await res.json()
      setChildren(Array.isArray(data) ? data : [])
    } catch { setChildren([]) }
  }, [entry.path, entry.isDir, authFetch])

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (isOpen && children === null) loadChildren()
  }, [isOpen, children, loadChildren])
  /* eslint-enable react-hooks/set-state-in-effect */

  const handleClick = () => {
    if (entry.isDir) {
      onToggle(entry.path)
      if (!isOpen) loadChildren()
      onFocusDir(entry.path)
    } else {
      onSelect(entry) // single click → preview tab
    }
  }

  // Double-click pins the file as a permanent (non-preview) tab, like VS Code.
  const handleDoubleClick = () => {
    if (!entry.isDir) onSelect(entry, { preview: false })
  }

  const contextItems = [
    ...(entry.isDir ? [] : [
      { label: 'Open', icon: File, action: () => onSelect(entry) },
    ]),
    // Copy available for both files and directories (directories use recursive backend copy).
    { label: 'Copy', icon: Copy, action: () => setClipboard({ path: entry.path, op: 'copy', isDir: entry.isDir }) },
    ...(entry.isDir ? [{ label: 'Paste', icon: Clipboard, action: () => onPaste(entry.path) }] : []),
    { separator: true },
    { label: 'Rename', icon: Pencil, action: () => onRename(entry.path) },
    { label: 'Delete', icon: Trash2, action: () => onDelete(entry.path) },
  ]

  const indentPx = depth * 12 + 4

  return (
    <div>
      <div
        onClick={handleClick}
        onDoubleClick={handleDoubleClick}
        onContextMenu={(e) => { e.preventDefault(); setCtx({ x: e.clientX, y: e.clientY }) }}
        className={`relative flex items-center h-[26px] cursor-pointer text-[12px] transition-colors select-none group ${
          isSelected
            ? 'bg-accent/10 text-text-primary'
            : 'hover:bg-bg-hover/80'
        }`}
        style={{ paddingLeft: `${indentPx}px` }}
      >
        {/* Indent guide lines */}
        {depth > 0 && Array.from({ length: depth }).map((_, i) => (
          <span
            key={i}
            className="absolute top-0 bottom-0 border-l border-border-subtle/60 pointer-events-none"
            style={{ left: `${i * 12 + 10}px` }}
          />
        ))}

        {/* Arrow + folder icon */}
        {entry.isDir ? (
          <>
            <span className="w-4 h-4 flex items-center justify-center shrink-0">
              {isOpen
                ? <ChevronDown className="w-3 h-3 text-text-muted" />
                : <ChevronRight className="w-3 h-3 text-text-muted" />
              }
            </span>
            {isOpen
              ? <FolderOpen className="w-[14px] h-[14px] mr-1.5 shrink-0 text-yellow" />
              : <Folder    className="w-[14px] h-[14px] mr-1.5 shrink-0 text-yellow/70" />
            }
          </>
        ) : (
          <>
            <span className="w-4 shrink-0" />
            <span className="mr-1.5"><FileIcon name={entry.name} /></span>
          </>
        )}

        {/* Filename */}
        <span className={`truncate flex-1 leading-tight ${nameColor}`}>{entry.name}</span>

        {/* Presence dots — who is viewing this file */}
        {viewers && viewers.length > 0 && (
          <span
            className="flex items-center -space-x-1 mr-2 shrink-0"
            title={viewers.map((v) => v.username || 'anon').join(', ')}>
            {viewers.slice(0, 3).map((v) => (
              <span
                key={v.id}
                className="w-2 h-2 rounded-full ring-1 ring-bg-secondary"
                style={{ backgroundColor: v.color || '#888' }}
              />
            ))}
          </span>
        )}

        {/* Git badge */}
        {badge && (
          <span className={`text-[10px] font-bold mr-2.5 shrink-0 ${badge.cls}`}>{badge.label}</span>
        )}
      </div>

      {ctx && <ContextMenu x={ctx.x} y={ctx.y} items={contextItems} onClose={() => setCtx(null)} />}

      {entry.isDir && isOpen && Array.isArray(children) && children.length > 0 && (
        <div className="relative">
          {children.map((child) => (
            <TreeNode
              key={child.path} entry={child} depth={depth + 1}
              onSelect={onSelect} onToggle={onToggle} expanded={expanded}
              authFetch={authFetch} onRefresh={onRefresh} selectedPath={selectedPath}
              gitMap={gitMap} presenceMap={presenceMap} clipboard={clipboard} setClipboard={setClipboard}
              onPaste={onPaste} onDelete={onDelete} onRename={onRename}
              onFocusDir={onFocusDir}
            />
          ))}
        </div>
      )}
    </div>
  )
}

/* ── Confirmation dialog ── */
function ConfirmDialog({ message, onConfirm, onCancel }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="bg-bg-elevated border border-border rounded-xl shadow-xl shadow-shadow-lg p-5 max-w-xs w-full mx-4">
        <p className="text-sm text-text-primary mb-4">{message}</p>
        <div className="flex gap-2 justify-end">
          <button
            onClick={onCancel}
            className="px-3 py-1.5 text-xs rounded-lg border border-border text-text-secondary hover:bg-bg-hover transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="px-3 py-1.5 text-xs rounded-lg bg-red/15 border border-red/30 text-red hover:bg-red/25 transition-colors font-medium"
          >
            Delete
          </button>
        </div>
      </div>
    </div>
  )
}

/* ── Main explorer ── */
export default function FileExplorer({ authFetch, onFileSelect, selectedPath, workspace, onRegisterActions, roster = [] }) {
  // Map file path -> members currently viewing it (for presence dots).
  const presenceMap = useMemo(() => {
    const m = {}
    for (const mem of roster) {
      if (!mem.file) continue
      ;(m[mem.file] ||= []).push(mem)
    }
    return m
  }, [roster])
  const [files, _setFiles] = useState([])
  const setFiles = (v) => _setFiles(Array.isArray(v) ? v : [])
  const [expanded, setExpanded] = useState(new Set())
  const [showNew, setShowNew] = useState(null)
  const [newName, setNewName] = useState('')
  const [clipboard, setClipboard] = useState(null)
  const [gitMap, setGitMap] = useState({})
  const [renaming, setRenaming] = useState(null)
  const [renameName, setRenameName] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(null) // path to confirm-delete
  const [focusedDir, setFocusedDir] = useState('') // last directory clicked in tree

  const loadRoot = useCallback(async () => {
    try {
      const res = await authFetch('/api/files?path=')
      const data = await res.json()
      if (Array.isArray(data)) setFiles(data)
    } catch { /* ignore */ }
  }, [authFetch])

  const loadGitStatus = useCallback(async () => {
    try {
      const res = await authFetch('/api/git/status')
      const data = await res.json()
      const map = {}
      for (const f of (data.files || [])) { map[f.path] = f.status }
      setGitMap(map)
    } catch { /* ignore */ }
  }, [authFetch])

  useEffect(() => {
    setFiles([])
    setExpanded(new Set())
    loadRoot()
    loadGitStatus()
    const interval = setInterval(loadGitStatus, 8000)
    return () => clearInterval(interval)
  }, [loadRoot, loadGitStatus, workspace])

  // Register refresh + new-file/folder triggers with the parent (for command palette).
  useEffect(() => {
    onRegisterActions?.({
      refresh: () => { loadRoot(); loadGitStatus() },
      newFile: () => setShowNew('file'),
      newFolder: () => setShowNew('folder'),
    })
  }, [onRegisterActions, loadRoot, loadGitStatus])

  const toggleExpand = (path) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      next.has(path) ? next.delete(path) : next.add(path)
      return next
    })
  }

  const handleCreate = async (e) => {
    e.preventDefault()
    if (!newName.trim()) return
    await authFetch('/api/files/create', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: newName, isDir: showNew === 'folder' }),
    })
    setShowNew(null); setNewName(''); loadRoot()
  }

  const handlePaste = async (targetDir) => {
    if (!clipboard) return
    const name = clipboard.path.split('/').pop()
    const dest = targetDir ? `${targetDir}/${name}` : name
    try {
      // Use the recursive copy endpoint for both files and directories.
      await authFetch('/api/files/copy', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ src: clipboard.path, dst: dest }),
      })
      loadRoot()
    } catch { /* ignore */ }
  }

  const handleDelete = (path) => {
    setConfirmDelete(path)
  }

  const confirmAndDelete = async () => {
    if (!confirmDelete) return
    const path = confirmDelete
    setConfirmDelete(null)
    await authFetch(`/api/files/delete?path=${encodeURIComponent(path)}`, { method: 'DELETE' })
    loadRoot()
  }

  const handleRename = (path) => {
    setRenaming(path)
    setRenameName(path.split('/').pop())
  }

  const submitRename = async (e) => {
    e.preventDefault()
    if (!renameName.trim() || !renaming) return
    const dir = renaming.includes('/') ? renaming.slice(0, renaming.lastIndexOf('/')) : ''
    const newPath = dir ? `${dir}/${renameName}` : renameName
    await authFetch('/api/files/rename', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ oldPath: renaming, newPath }),
    })
    setRenaming(null); setRenameName(''); loadRoot()
  }

  useEffect(() => {
    const handler = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'v' && clipboard) {
        // Paste into the focused directory if one is tracked; otherwise workspace root.
        handlePaste(focusedDir)
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [clipboard, focusedDir]) // eslint-disable-line react-hooks/exhaustive-deps

  const folderLabel = workspace ? workspace.split('/').pop() : 'Explorer'

  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-border shrink-0">
        <span className="text-[10px] font-bold uppercase tracking-widest text-text-muted truncate">
          {folderLabel}
        </span>
        <div className="flex items-center gap-0.5">
          <IconBtn icon={FilePlus} title="New File"   onClick={() => setShowNew('file')} />
          <IconBtn icon={FolderPlus} title="New Folder" onClick={() => setShowNew('folder')} />
          <IconBtn icon={RefreshCw}  title="Refresh"    onClick={() => { loadRoot(); loadGitStatus() }} />
        </div>
      </div>

      {/* New file/folder */}
      {showNew && (
        <InlineInput
          placeholder={showNew === 'file' ? 'filename.ext' : 'folder-name'}
          value={newName}
          onChange={setNewName}
          onSubmit={handleCreate}
          onBlur={() => { setShowNew(null); setNewName('') }}
        />
      )}

      {/* Rename */}
      {renaming && (
        <InlineInput
          placeholder="new name"
          value={renameName}
          onChange={setRenameName}
          onSubmit={submitRename}
          onBlur={() => setRenaming(null)}
        />
      )}

      {/* File tree */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden py-0.5 select-none">
        {Array.isArray(files) && files.map((entry) => (
          <TreeNode
            key={entry.path} entry={entry} depth={0}
            onSelect={onFileSelect} onToggle={toggleExpand} expanded={expanded}
            authFetch={authFetch} onRefresh={loadRoot} selectedPath={selectedPath}
            gitMap={gitMap} presenceMap={presenceMap} clipboard={clipboard} setClipboard={setClipboard}
            onPaste={handlePaste} onDelete={handleDelete} onRename={handleRename}
            onFocusDir={setFocusedDir}
          />
        ))}
      </div>

      {/* Delete confirmation dialog */}
      {confirmDelete && (
        <ConfirmDialog
          message={`Delete "${confirmDelete.split('/').pop()}"? This cannot be undone.`}
          onConfirm={confirmAndDelete}
          onCancel={() => setConfirmDelete(null)}
        />
      )}
    </div>
  )
}

// eslint-disable-next-line no-unused-vars
function IconBtn({ icon: Icon, title, onClick }) {
  return (
    <button
      onClick={onClick}
      title={title}
      className="w-6 h-6 flex items-center justify-center rounded-md text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors"
    >
      <Icon className="w-3.5 h-3.5" />
    </button>
  )
}

