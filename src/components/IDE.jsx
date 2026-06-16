import { useState, useCallback, useRef, useEffect } from 'react'
import {
  Files, GitBranch, TerminalSquare, LogOut, Save, FolderOpen,
  Globe, Settings as SettingsIcon, Moon, Sun, ChevronLeft, Search as SearchIcon,
} from 'lucide-react'
import { useMobile } from '../hooks/useMobile'
import { useTheme } from '../hooks/useTheme'
import Logo from './Logo'
import FileExplorer from './FileExplorer'
import Editor from './Editor'
import EditorTabs from './EditorTabs'
import TerminalPanel from './TerminalPanel'
import GitPanel from './GitPanel'
import FolderPicker from './FolderPicker'
import Browser from './Browser'
import Settings from './Settings'
import SearchPanel from './SearchPanel'
import MobileNav from './MobileNav'
import CommandPalette from './CommandPalette'
import { ImagePreview, BinaryNotice } from './ImagePreview'
import { useLSP } from '../hooks/useLSP'

let browserIdCounter = 0

export default function IDE({ token, authFetch, onLogout, workspace, recents, onWorkspaceChange }) {
  const isMobile = useMobile()
  const { isDark, toggle: toggleTheme } = useTheme()

  const [tabs, setTabs] = useState(() => {
    try {
      const saved = localStorage.getItem('wede_tabs')
      return saved ? JSON.parse(saved) : []
    } catch { return [] }
  })
  const [activeTab, setActiveTab] = useState(() => localStorage.getItem('wede_activeTab') || null)

  const [showSidebar, setShowSidebar] = useState(true)
  const [sidebarTab, setSidebarTab] = useState('files')

  const [showTerminal, setShowTerminal] = useState(true)
  const [showSettings, setShowSettings] = useState(false)

  const [mobilePanel, setMobilePanel] = useState('files')
  const [mobileMenu, setMobileMenu] = useState(false)
  const [termFullscreen, setTermFullscreen] = useState(false)

  const [showFolderPicker, setShowFolderPicker] = useState(false)
  const [saving, setSaving] = useState(false)
  const [autoSaveStatus, setAutoSaveStatus] = useState('') // 'saving'|'saved'|''
  const [showCommandPalette, setShowCommandPalette] = useState(false)

  // Editor settings — persisted to localStorage
  const [editorSettings, setEditorSettings] = useState(() => {
    const saved = localStorage.getItem('wede_editor_settings')
    if (!saved) return { fontSize: 13, tabWidth: 2, wordWrap: false, autoSave: true, minimap: false, lsp: true, formatOnSave: false }
    try {
      const s = JSON.parse(saved)
      // Back-fill new keys for users upgrading from older localStorage state.
      if (s.minimap === undefined) s.minimap = false
      if (s.lsp === undefined) s.lsp = true
      if (s.formatOnSave === undefined) s.formatOnSave = false
      return s
    } catch {
      return { fontSize: 13, tabWidth: 2, wordWrap: false, autoSave: true, minimap: false, lsp: true, formatOnSave: false }
    }
  })

  const handleEditorSettingsChange = useCallback((s) => {
    setEditorSettings(s)
    try { localStorage.setItem('wede_editor_settings', JSON.stringify(s)) } catch (_ignored) { void _ignored }
  }, [])


  // Expose FileExplorer's refresh + new-file/folder triggers for the command palette and SSE watcher.
  const explorerActionsRef = useRef(null)
  // Stable ref used by the SSE watcher to trigger explorer reload without re-subscribing.
  const sseExplorerRefreshRef = useRef(null)
  const handleRegisterExplorerActions = useCallback((actions) => {
    explorerActionsRef.current = actions
    sseExplorerRefreshRef.current = actions?.refresh
  }, [])

  // Expose Editor actions (goToLine) registered by the active Editor instance.
  const editorActionsRef = useRef(null)
  const handleRegisterEditorActions = useCallback((actions) => {
    editorActionsRef.current = actions
  }, [])

  const [sidebarWidth, setSidebarWidth] = useState(260)
  const [terminalHeight, setTerminalHeight] = useState(250)
  const [settingsWidth, setSettingsWidth] = useState(320)
  const [terminalKey, setTerminalKey] = useState(0)

  // Status bar info
  const [gitBranch, setGitBranch] = useState('')
  const [gitChanges, setGitChanges] = useState(0)
  const [cursor, setCursor] = useState({ line: 1, col: 1 })

  const resizingRef = useRef(null)
  const folderName = workspace?.split('/').pop() || 'wede'

  // Persist open tabs to localStorage
  useEffect(() => {
    try {
      const toSave = tabs.map(t => ({ path: t.path, name: t.name, type: t.type, url: t.url }))
      localStorage.setItem('wede_tabs', JSON.stringify(toSave))
    } catch { /* ignore */ }
  }, [tabs])

  useEffect(() => {
    if (activeTab) localStorage.setItem('wede_activeTab', activeTab)
    else localStorage.removeItem('wede_activeTab')
  }, [activeTab])

  // Re-fetch content for restored tabs on mount
  useEffect(() => {
    if (tabs.length === 0) return
    const needsContent = tabs.filter(t => t.type !== 'browser' && t.content === undefined)
    if (needsContent.length === 0) return
    Promise.all(needsContent.map(async (t) => {
      try {
        const res = await authFetch(`/api/files/read?path=${encodeURIComponent(t.path)}`)
        const data = await res.json()
        if (data.fileType === 'image') return { path: t.path, content: '', fileType: 'image', dataUrl: data.dataUrl }
        if (data.fileType === 'binary') return { path: t.path, content: '', fileType: 'binary', size: data.size }
        return { path: t.path, content: data.content }
      } catch { return { path: t.path, content: '' } }
    })).then(results => {
      setTabs(prev => prev.map(t => {
        const r = results.find(r => r.path === t.path)
        if (r) return { ...t, content: r.content, originalContent: r.content, modified: false, fileType: r.fileType, dataUrl: r.dataUrl, size: r.size }
        return t
      }))
    })
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Fetch git branch + change count for status bar
  useEffect(() => {
    let active = true
    const fetchGit = async () => {
      try {
        const res = await authFetch('/api/git/status')
        const data = await res.json()
        if (active) {
          setGitBranch(data.branch || '')
          setGitChanges(data.files?.length || 0)
        }
      } catch { /* ignore */ }
    }
    fetchGit()
    // Fallback poll at 30 s — the SSE watcher below provides faster updates.
    const interval = setInterval(fetchGit, 30000)
    return () => { active = false; clearInterval(interval) }
  }, [authFetch, workspace])

  // ── File watching SSE — workspace change events ──
  // When the server detects a file change it sends {"type":"change"} which
  // triggers a git-status refresh and signals the explorer to reload.
  useEffect(() => {
    if (!workspace) return
    let es = null
    let active = true

    const connect = () => {
      if (!active) return
      // SSE can't use custom headers, so pass the token as a query param.
      // The auth middleware already supports ?token= for WS/SSE routes.
      const url = `/api/watch?token=${encodeURIComponent(token)}`
      es = new EventSource(url)
      es.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data)
          if (msg.type === 'change') {
            // Refresh git status badge.
            authFetch('/api/git/status').then(r => r.json()).then(data => {
              setGitBranch(data.branch || '')
              setGitChanges(data.files?.length || 0)
            }).catch(() => {})
            // Signal explorer to reload.
            sseExplorerRefreshRef.current?.()
          }
        } catch { /* ignore */ }
      }
      es.onerror = () => {
        es?.close()
        if (active) setTimeout(connect, 5000) // reconnect after 5 s
      }
    }

    connect()
    return () => {
      active = false
      es?.close()
    }
  }, [workspace, token]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Auto-save ──
  // After a configurable debounce (1.5 s by default) auto-save dirty tabs.
  const autoSaveTimerRef = useRef(null)
  const triggerAutoSave = useCallback((path, content) => {
    if (!editorSettings.autoSave) return
    clearTimeout(autoSaveTimerRef.current)
    autoSaveTimerRef.current = setTimeout(async () => {
      setAutoSaveStatus('saving')
      try {
        await authFetch('/api/files/write', {
          method: 'PUT', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path, content }),
        })
        setTabs((prev) => prev.map((t) =>
          t.path === path ? { ...t, originalContent: t.content, modified: false } : t
        ))
        setAutoSaveStatus('saved')
        setTimeout(() => setAutoSaveStatus(''), 2000)
      } catch {
        setAutoSaveStatus('')
      }
    }, 1500)
  }, [authFetch, editorSettings.autoSave])

  // ── Close tab ── (defined here so it is reachable by the keyboard-shortcut
  // useEffect below — Vite/Rolldown's const hoisting can trigger a TDZ when
  // a const binding is referenced before its declaration in the same scope.)
  const closeTab = useCallback((path) => {
    setTabs((prev) => {
      const next = prev.filter((t) => t.path !== path)
      if (activeTab === path) {
        const idx = prev.findIndex((t) => t.path === path)
        setActiveTab(next[Math.min(idx, next.length - 1)]?.path || null)
      }
      return next
    })
  }, [activeTab])

  // ── Global keyboard shortcuts ──
  useEffect(() => {
    const handler = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === 'P') {
        e.preventDefault()
        setShowCommandPalette((v) => !v)
      }
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === 'F') {
        e.preventDefault()
        // Open search sidebar.
        setSidebarTab('search')
        setShowSidebar(true)
      }
      // Ctrl+G — go to line (only when an editor tab is active; let the editor
      // keymap handle it when the editor has focus; this catches the case where
      // focus is outside the editor, e.g. in the sidebar).
      if (e.ctrlKey && !e.metaKey && !e.shiftKey && e.key === 'g') {
        if (activeTab) {
          e.preventDefault()
          editorActionsRef.current?.goToLine()
        }
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [activeTab])

  // Ctrl/Cmd+W — close active tab
  useEffect(() => {
    const handler = (e) => {
      if ((e.metaKey || e.ctrlKey) && !e.shiftKey && e.key === 'w') {
        // Only intercept if there is an active tab (let browser handle otherwise)
        if (activeTab) {
          e.preventDefault()
          closeTab(activeTab)
        }
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [activeTab, closeTab])

  // ── Save All modified tabs ──
  const saveAll = useCallback(async () => {
    const modifiedTabs = tabs.filter((t) => t.modified && t.type !== 'browser')
    await Promise.all(modifiedTabs.map(async (tab) => {
      try {
        await authFetch('/api/files/write', {
          method: 'PUT', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path: tab.path, content: tab.content }),
        })
        setTabs((prev) => prev.map((t) =>
          t.path === tab.path ? { ...t, originalContent: t.content, modified: false } : t
        ))
      } catch { /* ignore */ }
    }))
  }, [tabs, authFetch])

  // ── Git stage/unstage all via command palette ──
  const gitStageAll = useCallback(async () => {
    await authFetch('/api/git/stage', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: '.' }),
    })
  }, [authFetch])

  const gitUnstageAll = useCallback(async () => {
    await authFetch('/api/git/unstage', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: '.' }),
    })
  }, [authFetch])

  // ── Open a browser tab ──
  const openBrowser = useCallback((url = 'https://wede.vulos.org') => {
    const existing = tabs.find((t) => t.type === 'browser')
    if (existing) {
      setTabs((prev) => prev.map((t) =>
        t.path === existing.path ? { ...t, url, name: urlToName(url) } : t
      ))
      setActiveTab(existing.path)
      if (isMobile) setMobilePanel('code')
      return
    }
    const id = `browser:${++browserIdCounter}`
    setTabs((prev) => [...prev, {
      path: id, name: urlToName(url), type: 'browser', url,
      content: '', originalContent: '', modified: false,
    }])
    setActiveTab(id)
    if (isMobile) setMobilePanel('code')
  }, [tabs, isMobile])

  // Capture link clicks → open in preview browser
  useEffect(() => {
    const handler = (e) => {
      const a = e.target.closest('a[href]')
      if (!a) return
      const href = a.getAttribute('href')
      if (!href) return
      if (href.startsWith('http://') || href.startsWith('https://')) {
        e.preventDefault()
        e.stopPropagation()
        openBrowser(href)
      }
    }
    document.addEventListener('click', handler, true)
    document.addEventListener('auxclick', handler, true)
    return () => {
      document.removeEventListener('click', handler, true)
      document.removeEventListener('auxclick', handler, true)
    }
  }, [openBrowser])

  const toggleSidebarTab = (tab) => {
    if (sidebarTab === tab && showSidebar) setShowSidebar(false)
    else { setSidebarTab(tab); setShowSidebar(true) }
  }

  const handleMouseDown = (type) => (e) => {
    e.preventDefault()
    resizingRef.current = { type, startX: e.clientX, startY: e.clientY }
    const handleMouseMove = (e) => {
      if (!resizingRef.current) return
      const { type, startX, startY } = resizingRef.current
      if (type === 'sidebar') {
        setSidebarWidth((w) => Math.max(180, Math.min(500, w + (e.clientX - startX))))
        resizingRef.current.startX = e.clientX
      } else if (type === 'terminal') {
        setTerminalHeight((h) => Math.max(100, Math.min(600, h + (startY - e.clientY))))
        resizingRef.current.startY = e.clientY
      } else if (type === 'settings') {
        setSettingsWidth((w) => Math.max(200, Math.min(500, w + (startX - e.clientX))))
        resizingRef.current.startX = e.clientX
      }
    }
    const handleMouseUp = () => {
      resizingRef.current = null
      window.removeEventListener('mousemove', handleMouseMove)
      window.removeEventListener('mouseup', handleMouseUp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }
    document.body.style.cursor = type === 'terminal' ? 'row-resize' : 'col-resize'
    document.body.style.userSelect = 'none'
    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('mouseup', handleMouseUp)
  }

  const handleWorkspaceOpen = (path) => {
    setTabs([])
    setActiveTab(null)
    setTerminalKey((k) => k + 1)
    setShowFolderPicker(false)
    onWorkspaceChange(path)
  }

  const openFile = useCallback(async (entry) => {
    if (entry.isDir) return
    const existing = tabs.find((t) => t.path === entry.path)
    if (existing) { setActiveTab(entry.path); return }
    try {
      const res = await authFetch(`/api/files/read?path=${encodeURIComponent(entry.path)}`)
      const data = await res.json()
      const tab = {
        path: entry.path, name: entry.name,
        content: data.content || '', originalContent: data.content || '', modified: false,
      }
      if (data.fileType === 'image') {
        tab.fileType = 'image'
        tab.dataUrl = data.dataUrl
      } else if (data.fileType === 'binary') {
        tab.fileType = 'binary'
        tab.size = data.size
      }
      setTabs((prev) => [...prev, tab])
      setActiveTab(entry.path)
      if (isMobile) setMobilePanel('code')
    } catch { /* ignore */ }
  }, [tabs, authFetch, isMobile])

  const updateContent = useCallback((path, newContent) => {
    setTabs((prev) => prev.map((t) => {
      if (t.path !== path) return t
      const modified = newContent !== t.originalContent
      if (modified) triggerAutoSave(path, newContent)
      return { ...t, content: newContent, modified }
    }))
  }, [triggerAutoSave])

  const FORMAT_EXTS = new Set(['go', 'js', 'jsx', 'ts', 'tsx', 'css', 'json', 'html', 'md', 'py'])

  const formatCurrentFile = useCallback(async () => {
    const tab = tabs.find((t) => t.path === activeTab)
    if (!tab || tab.type === 'browser') return
    const ext = tab.name.split('.').pop().toLowerCase()
    if (!FORMAT_EXTS.has(ext)) return
    try {
      const res = await authFetch('/api/files/format', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: tab.path, content: tab.content }),
      })
      if (!res.ok) return
      const data = await res.json()
      if (data.formatted && data.content !== tab.content) {
        setTabs((prev) => prev.map((t) =>
          t.path === tab.path ? { ...t, content: data.content, modified: data.content !== t.originalContent } : t
        ))
      }
    } catch { /* ignore — format failure should not block save */ }
  }, [tabs, activeTab, authFetch]) // eslint-disable-line react-hooks/exhaustive-deps

  const saveFile = useCallback(async () => {
    const tab = tabs.find((t) => t.path === activeTab)
    if (!tab?.modified || tab.type === 'browser') return
    // Format on save — run before writing so the formatted content is persisted.
    if (editorSettings.formatOnSave) {
      const ext = tab.name.split('.').pop().toLowerCase()
      if (FORMAT_EXTS.has(ext)) {
        try {
          const res = await authFetch('/api/files/format', {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: tab.path, content: tab.content }),
          })
          if (res.ok) {
            const data = await res.json()
            if (data.formatted && data.content !== tab.content) {
              setTabs((prev) => prev.map((t) =>
                t.path === tab.path ? { ...t, content: data.content, modified: data.content !== t.originalContent } : t
              ))
              // Re-read the (now updated) tab for the write below.
              // We use a local variable so we don't need to wait for state flush.
              setSaving(true)
              try {
                await authFetch('/api/files/write', {
                  method: 'PUT', headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify({ path: tab.path, content: data.content }),
                })
                setTabs((prev) => prev.map((t) =>
                  t.path === tab.path ? { ...t, originalContent: data.content, modified: false } : t
                ))
              } catch { /* ignore */ }
              setSaving(false)
              return
            }
          }
        } catch { /* ignore — fall through to normal save */ }
      }
    }
    setSaving(true)
    try {
      await authFetch('/api/files/write', {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: tab.path, content: tab.content }),
      })
      setTabs((prev) => prev.map((t) =>
        t.path === activeTab ? { ...t, originalContent: t.content, modified: false } : t
      ))
    } catch { /* ignore */ }
    setSaving(false)
  }, [tabs, activeTab, authFetch, editorSettings.formatOnSave]) // eslint-disable-line react-hooks/exhaustive-deps

  const currentTab = tabs.find((t) => t.path === activeTab)
  const hasModified = tabs.some((t) => t.modified)

  // LSP — provides diagnostics + hover for supported languages.
  // Degrades gracefully when language server binaries are not installed.
  const { extension: lspExtension, available: lspAvailable } = useLSP({
    file: currentTab,
    token,
    authFetch,
    lspEnabled: editorSettings.lsp ?? true,
  })

  const renderTabContent = () => {
    if (!currentTab) {
      return <Editor file={null} content={null} onChange={() => {}} onSave={() => {}} settings={editorSettings} />
    }
    if (currentTab.type === 'browser') {
      return (
        <Browser
          url={currentTab.url}
          onUrlChange={(newUrl) => {
            setTabs((prev) => prev.map((t) =>
              t.path === currentTab.path ? { ...t, url: newUrl, name: urlToName(newUrl) } : t
            ))
          }}
        />
      )
    }
    if (currentTab.fileType === 'image') {
      return <ImagePreview dataUrl={currentTab.dataUrl} filename={currentTab.name} />
    }
    if (currentTab.fileType === 'binary') {
      return <BinaryNotice filename={currentTab.name} size={currentTab.size} />
    }
    return (
      <Editor
        file={currentTab}
        content={currentTab.content}
        onChange={(c) => activeTab && updateContent(activeTab, c)}
        onSave={saveFile}
        onCursorChange={setCursor}
        settings={editorSettings}
        lspExtension={lspExtension}
        onRegisterActions={handleRegisterEditorActions}
      />
    )
  }

  // ── Mobile menu overlay ──
  const MobileMenuOverlay = () => (
    <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm" onClick={() => setMobileMenu(false)}>
      <div className="absolute bottom-16 left-0 right-0 bg-bg-elevated border-t border-border rounded-t-2xl p-4 animate-slide-up"
        onClick={(e) => e.stopPropagation()}>
        <div className="w-8 h-1 bg-bg-active rounded-full mx-auto mb-4" />
        <div className="grid grid-cols-3 gap-2.5">
          {[
            { icon: Globe, label: 'Browser', action: () => { openBrowser(); setMobileMenu(false) } },
            { icon: SettingsIcon, label: 'Settings', action: () => { setMobilePanel('settings'); setMobileMenu(false) } },
            { icon: FolderOpen, label: 'Open Folder', action: () => { setShowFolderPicker(true); setMobileMenu(false) } },
            { icon: isDark ? Sun : Moon, label: isDark ? 'Light' : 'Dark', action: () => { toggleTheme(); setMobileMenu(false) } },
            { icon: LogOut, label: 'Logout', action: () => { onLogout(); setMobileMenu(false) } },
          // eslint-disable-next-line no-unused-vars
          ].map(({ icon: Icon, label, action }) => (
            <button key={label} onClick={action}
              className="flex flex-col items-center gap-2 p-3 rounded-xl bg-bg-secondary border border-border text-text-secondary hover:text-accent hover:border-accent/30 transition-colors">
              <Icon className="w-5 h-5" />
              <span className="text-[11px] font-medium">{label}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  )

  if (showFolderPicker) {
    return (
      <div className="h-screen flex flex-col bg-bg-primary">
        <div className="flex items-center px-4 py-2.5 bg-bg-tertiary border-b border-border">
          <button onClick={() => setShowFolderPicker(false)}
            className="flex items-center gap-1.5 text-xs text-text-muted hover:text-text-primary transition-colors">
            <ChevronLeft className="w-4 h-4" /> Back
          </button>
        </div>
        <div className="flex-1 overflow-y-auto flex items-start justify-center p-6">
          <div className="w-full max-w-xl bg-bg-primary border border-border rounded-xl p-6 shadow-xl shadow-shadow">
            <h2 className="text-base font-semibold text-text-primary mb-4">Open Folder</h2>
            <FolderPicker authFetch={authFetch} recents={recents} onOpen={handleWorkspaceOpen} inline />
          </div>
        </div>
      </div>
    )
  }

  // ══════════════════════════
  // ── MOBILE ──
  // ══════════════════════════
  if (isMobile) {
    return (
      <div className="h-[100dvh] flex flex-col bg-bg-primary">
        {/* Mobile top bar */}
        <div className="flex items-center justify-between px-3 py-2 bg-bg-tertiary border-b border-border shrink-0">
          <div className="flex items-center gap-2 min-w-0">
            <Logo size={18} showName nameClass="text-sm font-semibold text-text-primary" />
            <button onClick={() => setShowFolderPicker(true)}
              className="flex items-center gap-1 text-xs text-text-secondary truncate max-w-32 hover:text-text-primary transition-colors">
              <FolderOpen className="w-3 h-3 text-yellow shrink-0" />
              <span className="truncate">{folderName}</span>
            </button>
          </div>
          <div className="flex items-center gap-1">
            {currentTab?.modified && currentTab.type !== 'browser' && (
              <button onClick={saveFile} disabled={saving}
                className="flex items-center gap-1 px-2.5 py-1 text-xs bg-accent/15 text-accent rounded-lg font-medium hover:bg-accent/25 transition-colors">
                <Save className="w-3 h-3" />{saving ? '...' : 'Save'}
              </button>
            )}
          </div>
        </div>

        <div className="flex-1 min-h-0 relative">
          {mobilePanel === 'files' && (
            <div className="h-full animate-fade-in">
              <FileExplorer authFetch={authFetch} onFileSelect={openFile} selectedPath={activeTab} workspace={workspace} onRegisterActions={handleRegisterExplorerActions} />
            </div>
          )}
          {mobilePanel === 'code' && (
            <div className="h-full flex flex-col animate-fade-in">
              <EditorTabs tabs={tabs} activeTab={activeTab} onSelect={setActiveTab} onClose={closeTab} />
              <div className="flex-1 min-h-0">{renderTabContent()}</div>
            </div>
          )}
          <div className={termFullscreen ? 'fixed inset-0 z-50' : 'absolute inset-0 z-10'} style={{ display: mobilePanel === 'terminal' ? 'block' : 'none' }}>
            <TerminalPanel key={terminalKey} token={token} authFetch={authFetch} visible={mobilePanel === 'terminal'}
              isFullscreen={termFullscreen} onToggleFullscreen={() => setTermFullscreen(!termFullscreen)} isMobile />
          </div>
          {mobilePanel === 'git' && (
            <div className="h-full animate-fade-in"><GitPanel authFetch={authFetch} visible isMobile /></div>
          )}
          {mobilePanel === 'settings' && (
            <div className="h-full animate-fade-in">
              <Settings
                visible
                onOpenFolder={() => setShowFolderPicker(true)}
                workspace={workspace}
                editorSettings={editorSettings}
                onEditorSettingsChange={handleEditorSettingsChange}
                lspAvailable={lspAvailable}
              />
            </div>
          )}
        </div>

        <MobileNav active={mobilePanel}
          onSelect={(id) => { if (id === 'menu') { setMobileMenu(true); return }; setMobilePanel(id) }}
          hasModified={hasModified} />
        {mobileMenu && <MobileMenuOverlay />}
        <CommandPalette
          visible={showCommandPalette}
          onClose={() => setShowCommandPalette(false)}
          onSaveFile={saveFile}
          onSaveAll={saveAll}
          onNewFile={() => { setMobilePanel('files'); explorerActionsRef.current?.newFile() }}
          onNewFolder={() => { setMobilePanel('files'); explorerActionsRef.current?.newFolder() }}
          onOpenFolder={() => setShowFolderPicker(true)}
          onToggleTerminal={() => setMobilePanel('terminal')}
          onOpenSettings={() => setMobilePanel('settings')}
          onFocusExplorer={() => setMobilePanel('files')}
          onFocusGit={() => setMobilePanel('git')}
          onOpenBrowser={() => openBrowser()}
          onCloseTab={() => activeTab && closeTab(activeTab)}
          onRefreshExplorer={() => explorerActionsRef.current?.refresh()}
          onGitStageAll={gitStageAll}
          onGitUnstageAll={gitUnstageAll}
          onToggleTheme={toggleTheme}
          onLogout={onLogout}
          onGoToLine={() => editorActionsRef.current?.goToLine()}
          onFormatFile={formatCurrentFile}
          isDark={isDark}
          hasActiveTab={!!activeTab}
          hasModified={hasModified}
        />
      </div>
    )
  }

  // ══════════════════════════
  // ── DESKTOP ──
  // ══════════════════════════
  return (
    <div className="h-screen flex flex-col bg-bg-base">
      {/* ── Top bar ── */}
      <div className="flex items-center justify-between px-2 h-10 bg-bg-tertiary border-b border-border shrink-0 select-none">
        {/* Left: logo + workspace */}
        <div className="flex items-center gap-1.5">
          <div className="flex items-center gap-2 pr-2 mr-0.5 border-r border-border h-6">
            <Logo size={18} showName nameClass="text-[13px] font-semibold text-text-primary tracking-tight" />
          </div>

          <button onClick={() => setShowFolderPicker(true)}
            className="flex items-center gap-1.5 px-2 py-1 rounded-md text-[12px] text-text-secondary hover:text-text-primary hover:bg-bg-hover transition-colors"
            title="Open Folder (change workspace)">
            <FolderOpen className="w-3.5 h-3.5 text-yellow" />
            <span className="max-w-36 truncate font-medium">{folderName}</span>
          </button>

          <div className="w-px h-4 bg-border mx-0.5" />

          {/* Panel toggles */}
          <button onClick={() => setShowTerminal(!showTerminal)}
            className={`flex items-center gap-1.5 px-2 py-1 rounded-md text-[12px] transition-colors ${
              showTerminal
                ? 'bg-accent/10 text-accent'
                : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'
            }`}
            title="Toggle Terminal">
            <TerminalSquare className="w-3.5 h-3.5" />
          </button>
          <button onClick={() => openBrowser()}
            className="flex items-center px-2 py-1 rounded-md text-[12px] text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors"
            title="Open Browser Preview">
            <Globe className="w-3.5 h-3.5" />
          </button>
          <button onClick={() => setShowSettings(!showSettings)}
            className={`flex items-center px-2 py-1 rounded-md text-[12px] transition-colors ${
              showSettings
                ? 'bg-accent/10 text-accent'
                : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'
            }`}
            title="Settings">
            <SettingsIcon className="w-3.5 h-3.5" />
          </button>
        </div>

        {/* Right: save + auto-save status + theme + logout */}
        <div className="flex items-center gap-1">
          {/* Auto-save status indicator */}
          {autoSaveStatus && (
            <span className="text-[11px] text-text-muted px-2 animate-fade-in">
              {autoSaveStatus === 'saving' ? 'saving…' : 'saved'}
            </span>
          )}
          {currentTab?.modified && currentTab.type !== 'browser' && (
            <button onClick={saveFile} disabled={saving}
              className="flex items-center gap-1.5 px-3 py-1 text-[12px] bg-accent text-white rounded-md hover:bg-accent-hover disabled:opacity-40 disabled:cursor-not-allowed transition-all font-medium shadow-sm shadow-accent/20">
              <Save className="w-3 h-3" />
              {saving ? 'Saving…' : 'Save'}
            </button>
          )}
          <button onClick={toggleTheme}
            className="p-1.5 rounded-md text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors"
            title={isDark ? 'Light mode' : 'Dark mode'}>
            {isDark ? <Sun className="w-3.5 h-3.5" /> : <Moon className="w-3.5 h-3.5" />}
          </button>
          <button onClick={onLogout}
            className="p-1.5 rounded-md text-text-muted hover:text-red hover:bg-bg-hover transition-colors" title="Logout">
            <LogOut className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* ── Main area ── */}
      <div className="flex flex-1 min-h-0">
        {/* Activity bar — narrow icon rail */}
        <div className="flex flex-col items-center pt-2 pb-2 gap-0.5 bg-bg-tertiary border-r border-border w-10 shrink-0">
          <ActivityBtn
            icon={Files}
            title="Explorer"
            active={sidebarTab === 'files' && showSidebar}
            onClick={() => toggleSidebarTab('files')}
          />
          <ActivityBtn
            icon={SearchIcon}
            title="Search (Ctrl+Shift+F)"
            active={sidebarTab === 'search' && showSidebar}
            onClick={() => toggleSidebarTab('search')}
          />
          <ActivityBtn
            icon={GitBranch}
            title="Source Control"
            active={sidebarTab === 'git' && showSidebar}
            badge={gitChanges > 0 ? gitChanges : 0}
            onClick={() => toggleSidebarTab('git')}
          />
        </div>

        {/* Sidebar */}
        {showSidebar && (
          <>
            <div style={{ width: sidebarWidth }} className="shrink-0 flex flex-col border-r border-border overflow-hidden bg-bg-secondary">
              {sidebarTab === 'files' && <FileExplorer authFetch={authFetch} onFileSelect={openFile} selectedPath={activeTab} workspace={workspace} onRegisterActions={handleRegisterExplorerActions} />}
              {sidebarTab === 'search' && <SearchPanel authFetch={authFetch} onOpenFile={(entry, line) => {
                openFile(entry).then?.(() => {
                  // targetLine is used by Editor to scroll to the match.
                  setTabs((prev) => prev.map((t) =>
                    t.path === entry.path ? { ...t, targetLine: line } : t
                  ))
                })
                // openFile is not async in the traditional sense, so set targetLine after a tick.
                setTimeout(() => {
                  setTabs((prev) => prev.map((t) =>
                    t.path === entry.path ? { ...t, targetLine: line } : t
                  ))
                }, 50)
              }} />}
              {sidebarTab === 'git' && <GitPanel authFetch={authFetch} visible />}
            </div>
            {/* Drag handle */}
            <div className="resize-handle-h shrink-0" onMouseDown={handleMouseDown('sidebar')} />
          </>
        )}

        {/* Center: editor + terminal */}
        <div className="flex-1 flex flex-col min-w-0 bg-bg-primary">
          <div className="flex-1 flex flex-col min-h-0">
            <EditorTabs tabs={tabs} activeTab={activeTab} onSelect={setActiveTab} onClose={closeTab} />
            <div className="flex-1 min-h-0">{renderTabContent()}</div>
          </div>

          {showTerminal && (
            <>
              <div className="resize-handle-v shrink-0" onMouseDown={handleMouseDown('terminal')} />
              <div style={{ height: terminalHeight }} className="shrink-0">
                <TerminalPanel key={terminalKey} token={token} authFetch={authFetch} visible={showTerminal} />
              </div>
            </>
          )}
        </div>

        {/* Right panel: settings */}
        {showSettings && (
          <>
            <div className="resize-handle-h shrink-0" onMouseDown={handleMouseDown('settings')} />
            <div style={{ width: settingsWidth }} className="shrink-0 border-l border-border bg-bg-secondary">
              <Settings
                visible
                onOpenFolder={() => setShowFolderPicker(true)}
                workspace={workspace}
                editorSettings={editorSettings}
                onEditorSettingsChange={handleEditorSettingsChange}
                lspAvailable={lspAvailable}
              />
            </div>
          </>
        )}
      </div>

      {/* ── Command palette ── */}
      <CommandPalette
        visible={showCommandPalette}
        onClose={() => setShowCommandPalette(false)}
        onSaveFile={saveFile}
        onSaveAll={saveAll}
        onNewFile={() => { toggleSidebarTab('files'); explorerActionsRef.current?.newFile() }}
        onNewFolder={() => { toggleSidebarTab('files'); explorerActionsRef.current?.newFolder() }}
        onOpenFolder={() => setShowFolderPicker(true)}
        onToggleTerminal={() => setShowTerminal((v) => !v)}
        onOpenSettings={() => setShowSettings((v) => !v)}
        onFocusExplorer={() => toggleSidebarTab('files')}
        onFocusGit={() => toggleSidebarTab('git')}
        onOpenBrowser={() => openBrowser()}
        onCloseTab={() => activeTab && closeTab(activeTab)}
        onRefreshExplorer={() => explorerActionsRef.current?.refresh()}
        onGitStageAll={gitStageAll}
        onGitUnstageAll={gitUnstageAll}
        onToggleTheme={toggleTheme}
        onLogout={onLogout}
        onGoToLine={() => editorActionsRef.current?.goToLine()}
        onFormatFile={formatCurrentFile}
        isDark={isDark}
        hasActiveTab={!!activeTab}
        hasModified={hasModified}
      />

      {/* ── Status bar ── */}
      <div className="flex items-center justify-between px-1 h-6 bg-status-bar border-t border-border text-status-text text-[11px] font-medium shrink-0 select-none">
        <div className="flex items-center">
          {gitBranch && (
            <button onClick={() => toggleSidebarTab('git')}
              className="flex items-center gap-1 px-2 h-full hover:bg-bg-hover transition-colors rounded-sm">
              <GitBranch className="w-3 h-3" />
              <span className="font-mono">{gitBranch}</span>
              {gitChanges > 0 && (
                <span className="ml-0.5 px-1 py-px rounded text-[10px] bg-yellow/15 text-yellow font-semibold">
                  {gitChanges}
                </span>
              )}
            </button>
          )}
          {currentTab?.modified && currentTab.type !== 'browser' && (
            <span className="px-2 text-yellow/80 font-medium">●</span>
          )}
        </div>
        <div className="flex items-center">
          {currentTab && currentTab.type !== 'browser' && (
            <>
              <span className="px-2 hover:bg-bg-hover rounded-sm cursor-default">Ln {cursor.line}, Col {cursor.col}</span>
              <span className="px-2 hover:bg-bg-hover rounded-sm cursor-default opacity-80">{extToLang(currentTab.name)}</span>
            </>
          )}
          {currentTab?.type === 'browser' && (
            <span className="px-2 opacity-70">Browser Preview</span>
          )}
          <span className="px-2 opacity-50">UTF-8</span>
        </div>
      </div>
    </div>
  )
}

/* Activity bar button */
// eslint-disable-next-line no-unused-vars
function ActivityBtn({ icon: Icon, title, active, badge, onClick }) {
  return (
    <button
      onClick={onClick}
      title={title}
      className={`relative w-8 h-8 flex items-center justify-center rounded-md transition-colors ${
        active
          ? 'text-accent bg-accent/10'
          : 'text-text-muted hover:text-text-secondary hover:bg-bg-hover'
      }`}
    >
      {active && (
        <span className="absolute left-0 top-1.5 bottom-1.5 w-0.5 rounded-r bg-accent" />
      )}
      <Icon className="w-4 h-4" />
      {badge > 0 && (
        <span className="absolute -top-0.5 -right-0.5 min-w-[14px] h-3.5 flex items-center justify-center rounded-full text-[9px] font-bold bg-accent text-white px-0.5 leading-none">
          {badge > 99 ? '99+' : badge}
        </span>
      )}
    </button>
  )
}

const EXT_LANGS = {
  js: 'JavaScript', jsx: 'JavaScript JSX', ts: 'TypeScript', tsx: 'TypeScript JSX',
  go: 'Go', py: 'Python', rs: 'Rust', rb: 'Ruby', java: 'Java', php: 'PHP',
  c: 'C', cpp: 'C++', h: 'C Header', cs: 'C#',
  html: 'HTML', htm: 'HTML', css: 'CSS', scss: 'SCSS', less: 'Less',
  json: 'JSON', xml: 'XML', svg: 'SVG', yaml: 'YAML', yml: 'YAML', toml: 'TOML',
  md: 'Markdown', sql: 'SQL', sh: 'Shell', bash: 'Bash', zsh: 'Shell',
  dockerfile: 'Dockerfile', makefile: 'Makefile',
  txt: 'Plain Text', env: 'Environment', gitignore: 'Git Ignore',
  mod: 'Go Module', sum: 'Go Checksum', lock: 'Lock File',
}

function extToLang(filename) {
  if (!filename) return 'Plain Text'
  const name = filename.toLowerCase()
  if (name === 'dockerfile') return 'Dockerfile'
  if (name === 'makefile') return 'Makefile'
  const ext = name.split('.').pop()
  return EXT_LANGS[ext] || 'Plain Text'
}

function urlToName(url) {
  try {
    const u = new URL(url)
    return u.hostname + (u.port ? ':' + u.port : '')
  } catch {
    return url.replace(/^https?:\/\//, '').split('/')[0] || 'Browser'
  }
}
