import { useState, useEffect, useCallback, useRef, useMemo, type ComponentType, type FormEvent, type MouseEvent } from 'react'
import {
  ChevronRight, ChevronDown, File, Folder, FolderOpen,
  FilePlus, FolderPlus, RefreshCw, Copy, Clipboard, Trash2, Pencil, FolderPlus as AddRoot, X as XIcon,
} from 'lucide-react'
import { makeWsFetch } from '../lib/wsScope'
import { fileKey } from '../lib/fileKey'
import type { AuthFetch, WorkspaceSummary, FileEntry, PresenceMember, ExplorerActions } from '../types'

interface TreeEntry {
  path: string
  name: string
  isDir: boolean
}

type GitStatus = 'modified' | 'added' | 'deleted' | 'untracked' | 'renamed'
type GitMap = Record<string, GitStatus>

interface ClipboardEntry {
  path: string
  op: 'copy'
  isDir: boolean
}

interface ContextMenuItem {
  label?: string
  icon?: ComponentType<{ className?: string }>
  action?: () => void
  separator?: boolean
}

/* ── File type icon colours (VS Code style) ── */
const EXT_ICON: Record<string, { color: string; label: string }> = {
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

function FileIcon({ name }: { name: string }) {
  const ext = name.includes('.') ? (name.split('.').pop() || '').toLowerCase() : name.toLowerCase()
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
const GIT_COLOR: Record<GitStatus, string> = {
  modified:  'text-yellow',
  added:     'text-green',
  deleted:   'text-red line-through',
  untracked: 'text-green',
  renamed:   'text-accent',
}

const GIT_BADGE: Record<GitStatus, { label: string; cls: string }> = {
  modified:  { label: 'M', cls: 'text-yellow' },
  added:     { label: 'A', cls: 'text-green' },
  deleted:   { label: 'D', cls: 'text-red' },
  untracked: { label: 'U', cls: 'text-green' },
  renamed:   { label: 'R', cls: 'text-accent' },
}

interface ContextMenuProps {
  x: number
  y: number
  items: ContextMenuItem[]
  onClose: () => void
}

/* ── Context menu ── */
function ContextMenu({ x, y, items, onClose }: ContextMenuProps) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handler = (e: globalThis.MouseEvent) => { if (!ref.current?.contains(e.target as Node)) onClose() }
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
          <button key={i} onClick={() => { item.action?.(); onClose() }}
            className="w-full flex items-center gap-2.5 px-3 py-1.5 text-[12px] text-text-secondary hover:bg-bg-hover hover:text-text-primary transition-colors text-left">
            {item.icon && <item.icon className="w-3.5 h-3.5 text-text-muted shrink-0" />}
            <span>{item.label}</span>
          </button>
        ))}
      </div>
    </div>
  )
}

interface InlineInputProps {
  placeholder: string
  value: string
  onChange: (v: string) => void
  onSubmit: (e: FormEvent<HTMLFormElement>) => void
  onBlur: () => void
}

/* ── Inline input for new file / rename ── */
function InlineInput({ placeholder, value, onChange, onSubmit, onBlur }: InlineInputProps) {
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

interface TreeNodeProps {
  entry: TreeEntry
  depth: number
  rootId: string
  onSelect: (entry: TreeEntry, opts?: { preview?: boolean }) => void
  onToggle: (path: string) => void
  expanded: Set<string>
  authFetch: AuthFetch
  onRefresh: () => void
  selectedPath?: string | null
  gitMap: GitMap
  presenceMap: Record<string, PresenceMember[]>
  clipboard: ClipboardEntry | null
  setClipboard: (c: ClipboardEntry | null) => void
  onPaste: (dir: string) => void
  onDelete: (path: string) => void
  onRename: (path: string) => void
  onFocusDir: (path: string) => void
}

/* ── Tree node ── */
function TreeNode({
  entry, depth, rootId, onSelect, onToggle, expanded, authFetch,
  onRefresh, selectedPath, gitMap, presenceMap, clipboard, setClipboard,
  onPaste, onDelete, onRename, onFocusDir,
}: TreeNodeProps) {
  const [children, setChildren] = useState<TreeEntry[] | null>(null)
  const [ctx, setCtx] = useState<{ x: number; y: number } | null>(null)
  const isOpen     = expanded.has(entry.path)
  const isSelected = selectedPath === fileKey(rootId, entry.path)
  const gitStatus  = gitMap?.[entry.path]
  const viewers    = presenceMap?.[entry.path]
  const nameColor  = gitStatus ? GIT_COLOR[gitStatus] : 'text-text-primary'
  const badge      = gitStatus ? GIT_BADGE[gitStatus] : null

  const loadChildren = useCallback(async () => {
    if (!entry.isDir) return
    try {
      const res = await authFetch(`/api/files?path=${encodeURIComponent(entry.path)}`)
      const data: unknown = await res.json()
      setChildren(Array.isArray(data) ? data as TreeEntry[] : [])
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
        onContextMenu={(e: MouseEvent<HTMLDivElement>) => { e.preventDefault(); setCtx({ x: e.clientX, y: e.clientY }) }}
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
              key={child.path} entry={child} depth={depth + 1} rootId={rootId}
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

interface ConfirmDialogProps {
  message: string
  onConfirm: () => void
  onCancel: () => void
}

/* ── Confirmation dialog ── */
function ConfirmDialog({ message, onConfirm, onCancel }: ConfirmDialogProps) {
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

interface RootSectionProps {
  ws: WorkspaceSummary
  authFetch: AuthFetch
  onFileSelect: (entry: FileEntry, opts?: { preview?: boolean }) => void
  selectedPath?: string | null
  roster?: PresenceMember[]
  register: (wsId: string, actions: ExplorerActions | null) => void
  collapsed: boolean
  onToggleCollapse: () => void
  onFocus: () => void
  onClose: () => void
  canClose: boolean
  showHeader: boolean
}

/* ── One workspace root: a collapsible section with its own scoped file tree. ──
   `ws` is { id, name, root }. All file/git calls are rewritten to this root's
   /api/workspaces/<id>/... endpoints, so several roots coexist independently. */
function RootSection({ ws, authFetch, onFileSelect, selectedPath, roster, register, collapsed, onToggleCollapse, onFocus, onClose, canClose, showHeader }: RootSectionProps) {
  // wsFetch mirrors authFetch but pins every legacy /api/<service> path to THIS
  // workspace, so the tree, git status, and mutations act on this root alone.
  const wsFetch = useMemo(() => makeWsFetch(authFetch, ws.id), [authFetch, ws.id])

  const presenceMap = useMemo(() => {
    const m: Record<string, PresenceMember[]> = {}
    for (const mem of roster || []) {
      if (!mem.file) continue
      ;(m[mem.file] ||= []).push(mem)
    }
    return m
  }, [roster])

  const [files, _setFiles] = useState<TreeEntry[]>([])
  const setFiles = (v: unknown) => _setFiles(Array.isArray(v) ? v as TreeEntry[] : [])
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [showNew, setShowNew] = useState<'file' | 'folder' | null>(null)
  const [newName, setNewName] = useState('')
  const [clipboard, setClipboard] = useState<ClipboardEntry | null>(null)
  const [gitMap, setGitMap] = useState<GitMap>({})
  const [renaming, setRenaming] = useState<string | null>(null)
  const [renameName, setRenameName] = useState('')
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)
  const [focusedDir, setFocusedDir] = useState('')

  const loadRoot = useCallback(async () => {
    try {
      const res = await wsFetch('/api/files?path=')
      const data: unknown = await res.json()
      if (Array.isArray(data)) setFiles(data)
    } catch { /* ignore */ }
  }, [wsFetch])

  const loadGitStatus = useCallback(async () => {
    try {
      const res = await wsFetch('/api/git/status')
      const data: { files?: { path: string; status: GitStatus }[] } = await res.json()
      const map: GitMap = {}
      for (const f of (data.files || [])) { map[f.path] = f.status }
      setGitMap(map)
    } catch { /* ignore */ }
  }, [wsFetch])

  useEffect(() => {
    setFiles([])
    setExpanded(new Set())
    loadRoot()
    loadGitStatus()
    const interval = setInterval(loadGitStatus, 8000)
    return () => clearInterval(interval)
  }, [loadRoot, loadGitStatus])

  // Register per-section actions so the parent can route global new-file/refresh
  // triggers (command palette / SSE watcher) to the focused root.
  useEffect(() => {
    register(ws.id, {
      refresh: () => { loadRoot(); loadGitStatus() },
      newFile: () => { onFocus(); setShowNew('file') },
      newFolder: () => { onFocus(); setShowNew('folder') },
    })
    return () => register(ws.id, null)
  }, [register, ws.id, loadRoot, loadGitStatus, onFocus])

  const toggleExpand = (path: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path); else next.add(path)
      return next
    })
  }

  const handleCreate = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!newName.trim()) return
    const base = focusedDir ? `${focusedDir}/` : ''
    await wsFetch('/api/files/create', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: base + newName, isDir: showNew === 'folder' }),
    })
    setShowNew(null); setNewName(''); loadRoot()
  }

  const handlePaste = async (targetDir: string) => {
    if (!clipboard) return
    const name = clipboard.path.split('/').pop()
    const dest = targetDir ? `${targetDir}/${name}` : name
    try {
      await wsFetch('/api/files/copy', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ src: clipboard.path, dst: dest }),
      })
      loadRoot()
    } catch { /* ignore */ }
  }

  const handleDelete = (path: string) => setConfirmDelete(path)

  const confirmAndDelete = async () => {
    if (!confirmDelete) return
    const path = confirmDelete
    setConfirmDelete(null)
    await wsFetch(`/api/files/delete?path=${encodeURIComponent(path)}`, { method: 'DELETE' })
    loadRoot()
  }

  const handleRename = (path: string) => {
    setRenaming(path)
    setRenameName(path.split('/').pop() || '')
  }

  const submitRename = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!renameName.trim() || !renaming) return
    const dir = renaming.includes('/') ? renaming.slice(0, renaming.lastIndexOf('/')) : ''
    const newPath = dir ? `${dir}/${renameName}` : renameName
    await wsFetch('/api/files/rename', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ oldPath: renaming, newPath }),
    })
    setRenaming(null); setRenameName(''); loadRoot()
  }

  useEffect(() => {
    const handler = (e: globalThis.KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'v' && clipboard) handlePaste(focusedDir)
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [clipboard, focusedDir]) // eslint-disable-line react-hooks/exhaustive-deps

  const selectFromRoot = useCallback((entry: TreeEntry, opts?: { preview?: boolean }) => {
    onFileSelect({ ...entry, workspaceId: ws.id, rel: entry.path }, opts)
  }, [onFileSelect, ws.id])

  const isCollapsed = showHeader && collapsed

  return (
    <div className="border-b border-border/40 last:border-b-0" onMouseDownCapture={onFocus}>
      {/* Root header — only shown when multiple roots are open (VS Code parity). */}
      {showHeader && (
        <div className="group flex items-center h-[26px] px-2 sticky top-0 z-10 bg-bg-secondary hover:bg-bg-hover/60 cursor-pointer select-none"
          onClick={onToggleCollapse}>
          <span className="w-4 h-4 flex items-center justify-center shrink-0">
            {collapsed ? <ChevronRight className="w-3 h-3 text-text-muted" /> : <ChevronDown className="w-3 h-3 text-text-muted" />}
          </span>
          <span className="text-[11px] font-bold uppercase tracking-wide text-text-secondary truncate flex-1" title={ws.root}>{ws.name}</span>
          <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
            <IconBtn icon={FilePlus} title="New File" onClick={(e) => { e.stopPropagation(); onFocus(); setShowNew('file') }} />
            <IconBtn icon={FolderPlus} title="New Folder" onClick={(e) => { e.stopPropagation(); onFocus(); setShowNew('folder') }} />
            <IconBtn icon={RefreshCw} title="Refresh" onClick={(e) => { e.stopPropagation(); loadRoot(); loadGitStatus() }} />
            {canClose && <IconBtn icon={XIcon} title="Remove folder from workspace" onClick={(e) => { e.stopPropagation(); onClose() }} />}
          </div>
        </div>
      )}

      {!isCollapsed && (
        <>
          {showNew && (
            <InlineInput
              placeholder={showNew === 'file' ? 'filename.ext' : 'folder-name'}
              value={newName}
              onChange={setNewName}
              onSubmit={handleCreate}
              onBlur={() => { setShowNew(null); setNewName('') }}
            />
          )}
          {renaming && (
            <InlineInput
              placeholder="new name"
              value={renameName}
              onChange={setRenameName}
              onSubmit={submitRename}
              onBlur={() => setRenaming(null)}
            />
          )}
          <div className="py-0.5">
            {files.map((entry) => (
              <TreeNode
                key={entry.path} entry={entry} depth={0} rootId={ws.id}
                onSelect={selectFromRoot} onToggle={toggleExpand} expanded={expanded}
                authFetch={wsFetch} onRefresh={loadRoot} selectedPath={selectedPath}
                gitMap={gitMap} presenceMap={presenceMap} clipboard={clipboard} setClipboard={setClipboard}
                onPaste={handlePaste} onDelete={handleDelete} onRename={handleRename}
                onFocusDir={setFocusedDir}
              />
            ))}
            {files.length === 0 && (
              <div className="px-3 py-1.5 text-[11px] text-text-muted italic">empty</div>
            )}
          </div>
        </>
      )}

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

export interface FileExplorerProps {
  authFetch: AuthFetch
  workspaces?: WorkspaceSummary[]
  onFileSelect: (entry: FileEntry, opts?: { preview?: boolean }) => void
  selectedPath?: string | null
  onRegisterActions?: (actions: ExplorerActions) => void
  onAddFolder?: () => void
  onCloseWorkspace?: (id: string) => void
  roster?: PresenceMember[]
}

/* ── Main explorer: renders every open workspace as a root section. ── */
export default function FileExplorer({ authFetch, workspaces = [], onFileSelect, selectedPath, onRegisterActions, onAddFolder, onCloseWorkspace, roster = [] }: FileExplorerProps) {
  const [focusedRoot, setFocusedRoot] = useState<string | null>(null)
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set())
  const sectionActions = useRef<Record<string, ExplorerActions>>({})

  const register = useCallback((wsId: string, actions: ExplorerActions | null) => {
    if (actions) sectionActions.current[wsId] = actions
    else delete sectionActions.current[wsId]
  }, [])

  const roots = workspaces
  const activeRoot = (focusedRoot && roots.some((r) => r.id === focusedRoot)) ? focusedRoot : roots[0]?.id

  // Route the command-palette / SSE global triggers to the focused root (new
  // file/folder) or fan out to every root (refresh).
  useEffect(() => {
    onRegisterActions?.({
      refresh: () => Object.values(sectionActions.current).forEach((a) => a?.refresh?.()),
      newFile: () => (activeRoot ? sectionActions.current[activeRoot] : undefined)?.newFile?.(),
      newFolder: () => (activeRoot ? sectionActions.current[activeRoot] : undefined)?.newFolder?.(),
    })
  }, [onRegisterActions, activeRoot])

  const toggleCollapse = (id: string) => setCollapsed((prev) => {
    const next = new Set(prev)
    if (next.has(id)) next.delete(id); else next.add(id)
    return next
  })

  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden">
      {/* Header — single root shows the folder name; multi-root shows "Explorer". */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-border shrink-0">
        <span className="text-[10px] font-bold uppercase tracking-widest text-text-muted truncate">
          {roots.length === 1 ? roots[0].name : 'Explorer'}
        </span>
        <div className="flex items-center gap-0.5">
          {/* New file/folder route to the focused root (used when its own header is hidden). */}
          <IconBtn icon={FilePlus} title="New File" onClick={() => (activeRoot ? sectionActions.current[activeRoot] : undefined)?.newFile?.()} />
          <IconBtn icon={FolderPlus} title="New Folder" onClick={() => (activeRoot ? sectionActions.current[activeRoot] : undefined)?.newFolder?.()} />
          {onAddFolder && <IconBtn icon={AddRoot} title="Add Folder to Workspace" onClick={onAddFolder} />}
          <IconBtn icon={RefreshCw} title="Refresh All" onClick={() => Object.values(sectionActions.current).forEach((a) => a?.refresh?.())} />
        </div>
      </div>

      {/* Roots */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden select-none">
        {roots.length === 0 && (
          <div className="px-3 py-4 text-[12px] text-text-muted">No folders open.</div>
        )}
        {roots.map((ws) => (
          <RootSection
            key={ws.id} ws={ws} authFetch={authFetch}
            onFileSelect={onFileSelect} selectedPath={selectedPath} roster={roster}
            register={register}
            showHeader={roots.length > 1}
            collapsed={collapsed.has(ws.id)}
            onToggleCollapse={() => toggleCollapse(ws.id)}
            onFocus={() => setFocusedRoot(ws.id)}
            canClose={roots.length > 1 && !!onCloseWorkspace}
            onClose={() => onCloseWorkspace?.(ws.id)}
          />
        ))}
      </div>
    </div>
  )
}

interface IconBtnProps {
  icon: ComponentType<{ className?: string }>
  title: string
  onClick: (e: MouseEvent<HTMLButtonElement>) => void
}

function IconBtn({ icon: Icon, title, onClick }: IconBtnProps) {
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
