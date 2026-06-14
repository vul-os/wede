import { useState, useCallback, useRef, useEffect } from 'react'
import {
  Files, GitBranch, TerminalSquare, LogOut, Save, FolderOpen,
  Globe, Settings as SettingsIcon, Moon, Sun, ChevronLeft
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
import MobileNav from './MobileNav'

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
      // Save tab metadata (path, name, type) but not content — content is re-fetched
      const toSave = tabs.map(t => ({ path: t.path, name: t.name, type: t.type, url: t.url }))
      localStorage.setItem('wede_tabs', JSON.stringify(toSave))
    } catch {}
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
        return { path: t.path, content: data.content }
      } catch { return { path: t.path, content: '' } }
    })).then(results => {
      setTabs(prev => prev.map(t => {
        const r = results.find(r => r.path === t.path)
        if (r) return { ...t, content: r.content, originalContent: r.content, modified: false }
        return t
      }))
    })
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Fetch git branch + change count
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
      } catch {}
    }
    fetchGit()
    const interval = setInterval(fetchGit, 10000)
    return () => { active = false; clearInterval(interval) }
  }, [authFetch, workspace])

  // ── Open a browser tab ──
  const openBrowser = useCallback((url = 'https://wede.vulos.org') => {
    // If there's already a browser tab, navigate it
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

  // Capture link clicks and open in preview browser
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
    // Use capture phase to intercept before browser handles ctrl+click
    document.addEventListener('click', handler, true)
    // Also intercept auxclick (middle-click)
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
      setTabs((prev) => [...prev, {
        path: entry.path, name: entry.name,
        content: data.content, originalContent: data.content, modified: false,
      }])
      setActiveTab(entry.path)
      if (isMobile) setMobilePanel('code')
    } catch {}
  }, [tabs, authFetch, isMobile])

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

  const updateContent = useCallback((path, newContent) => {
    setTabs((prev) => prev.map((t) =>
      t.path === path ? { ...t, content: newContent, modified: newContent !== t.originalContent } : t
    ))
  }, [])

  const saveFile = useCallback(async () => {
    const tab = tabs.find((t) => t.path === activeTab)
    if (!tab?.modified || tab.type === 'browser') return
    setSaving(true)
    try {
      await authFetch('/api/files/write', {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: tab.path, content: tab.content }),
      })
      setTabs((prev) => prev.map((t) =>
        t.path === activeTab ? { ...t, originalContent: t.content, modified: false } : t
      ))
    } catch {}
    setSaving(false)
  }, [tabs, activeTab, authFetch])

  const currentTab = tabs.find((t) => t.path === activeTab)
  const hasModified = tabs.some((t) => t.modified)

  // ── Render the active tab content ──
  const renderTabContent = () => {
    if (!currentTab) {
      return <Editor file={null} content={null} onChange={() => {}} onSave={() => {}} />
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
    return (
      <Editor
        file={currentTab}
        content={currentTab.content}
        onChange={(c) => activeTab && updateContent(activeTab, c)}
        onSave={saveFile}
        onCursorChange={setCursor}
      />
    )
  }

  // ── Mobile Menu ──
  const MobileMenuOverlay = () => (
    <div className="fixed inset-0 z-50 bg-black/60" onClick={() => setMobileMenu(false)}>
      <div className="absolute bottom-16 left-0 right-0 bg-bg-primary border-t border-border rounded-t-2xl p-4 animate-slide-up"
        onClick={(e) => e.stopPropagation()}>
        <div className="w-8 h-1 bg-bg-active rounded-full mx-auto mb-4" />
        <div className="grid grid-cols-3 gap-3">
          {[
            { icon: Globe, label: 'Browser', action: () => { openBrowser(); setMobileMenu(false) } },
            { icon: SettingsIcon, label: 'Settings', action: () => { setMobilePanel('settings'); setMobileMenu(false) } },
            { icon: FolderOpen, label: 'Open Folder', action: () => { setShowFolderPicker(true); setMobileMenu(false) } },
            { icon: isDark ? Sun : Moon, label: isDark ? 'Light Mode' : 'Dark Mode', action: () => { toggleTheme(); setMobileMenu(false) } },
            { icon: LogOut, label: 'Logout', action: () => { onLogout(); setMobileMenu(false) } },
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
        <div className="flex items-center px-3 py-2 bg-bg-tertiary border-b border-border">
          <button onClick={() => setShowFolderPicker(false)}
            className="flex items-center gap-1 text-xs text-text-muted hover:text-text-primary">
            <ChevronLeft className="w-4 h-4" /> Back
          </button>
        </div>
        <div className="flex-1 overflow-y-auto flex items-start justify-center p-4">
          <div className="w-full max-w-xl bg-bg-primary border border-border rounded-xl p-6">
            <h2 className="text-lg font-semibold text-text-primary mb-4">Open Folder</h2>
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
        <div className="flex items-center justify-between px-3 py-2 bg-bg-tertiary border-b border-border shrink-0">
          <div className="flex items-center gap-2 min-w-0">
            <Logo size={18} showName nameClass="text-sm text-text-primary" />
            <button onClick={() => setShowFolderPicker(true)}
              className="flex items-center gap-1 text-xs text-text-secondary truncate max-w-32">
              <FolderOpen className="w-3 h-3 text-yellow shrink-0" />
              <span className="truncate">{folderName}</span>
            </button>
          </div>
          <div className="flex items-center gap-1">
            {currentTab?.modified && currentTab.type !== 'browser' && (
              <button onClick={saveFile} disabled={saving}
                className="flex items-center gap-1 px-2 py-1 text-xs bg-accent/20 text-accent rounded-lg">
                <Save className="w-3 h-3" />{saving ? '...' : 'Save'}
              </button>
            )}
          </div>
        </div>

        <div className="flex-1 min-h-0 relative">
          {mobilePanel === 'files' && (
            <div className="h-full animate-fade-in">
              <FileExplorer authFetch={authFetch} onFileSelect={openFile} selectedPath={activeTab} workspace={workspace} />
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
              <Settings visible onOpenFolder={() => setShowFolderPicker(true)} workspace={workspace} />
            </div>
          )}
        </div>

        <MobileNav active={mobilePanel}
          onSelect={(id) => { if (id === 'menu') { setMobileMenu(true); return }; setMobilePanel(id) }}
          hasModified={hasModified} />
        {mobileMenu && <MobileMenuOverlay />}
      </div>
    )
  }

  // ══════════════════════════
  // ── DESKTOP ──
  // ══════════════════════════
  return (
    <div className="h-screen flex flex-col bg-bg-primary">
      {/* Top bar */}
      <div className="flex items-center justify-between px-3 py-1.5 bg-bg-tertiary border-b border-border shrink-0">
        <div className="flex items-center gap-1">
          <Logo size={20} showName nameClass="text-sm text-text-primary mr-1" />
          <button onClick={() => setShowFolderPicker(true)}
            className="flex items-center gap-1.5 px-2 py-1 rounded-md text-xs text-text-secondary hover:text-text-primary hover:bg-bg-hover transition-colors border border-border"
            title="Open Folder">
            <FolderOpen className="w-3.5 h-3.5 text-yellow" />
            <span className="max-w-32 truncate">{folderName}</span>
          </button>

          <div className="w-px h-5 bg-border mx-1" />

          <button onClick={() => setShowTerminal(!showTerminal)}
            className={`p-1.5 rounded-md transition-colors ${showTerminal ? 'bg-bg-hover text-accent' : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'}`}
            title="Terminal">
            <TerminalSquare className="w-4 h-4" />
          </button>
          <button onClick={() => openBrowser()}
            className="p-1.5 rounded-md transition-colors text-text-muted hover:text-text-primary hover:bg-bg-hover"
            title="Open Browser Tab">
            <Globe className="w-4 h-4" />
          </button>
          <button onClick={() => setShowSettings(!showSettings)}
            className={`p-1.5 rounded-md transition-colors ${showSettings ? 'bg-bg-hover text-accent' : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'}`}
            title="Settings">
            <SettingsIcon className="w-4 h-4" />
          </button>
        </div>

        <div className="flex items-center gap-2">
          {currentTab?.modified && currentTab.type !== 'browser' && (
            <button onClick={saveFile} disabled={saving}
              className="flex items-center gap-1 px-2.5 py-1 text-xs bg-accent/15 text-accent rounded-md hover:bg-accent/25 transition-colors font-medium">
              <Save className="w-3 h-3" />{saving ? 'Saving...' : 'Save'}
            </button>
          )}
          <button onClick={toggleTheme}
            className="p-1.5 rounded-md text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors"
            title={isDark ? 'Light mode' : 'Dark mode'}>
            {isDark ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
          </button>
          <button onClick={onLogout}
            className="p-1.5 rounded-md text-text-muted hover:text-red hover:bg-bg-hover transition-colors" title="Logout">
            <LogOut className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Main area */}
      <div className="flex flex-1 min-h-0">
        {/* Activity bar */}
        <div className="flex flex-col items-center py-1.5 gap-1 bg-bg-tertiary border-r border-border w-10 shrink-0">
          <button onClick={() => toggleSidebarTab('files')}
            className={`p-1.5 rounded-md transition-colors ${sidebarTab === 'files' && showSidebar ? 'text-accent' : 'text-text-muted hover:text-text-primary'}`}
            title="Explorer">
            <Files className="w-5 h-5" />
          </button>
          <button onClick={() => toggleSidebarTab('git')}
            className={`p-1.5 rounded-md transition-colors ${sidebarTab === 'git' && showSidebar ? 'text-accent' : 'text-text-muted hover:text-text-primary'}`}
            title="Source Control">
            <GitBranch className="w-5 h-5" />
          </button>
        </div>

        {/* Sidebar */}
        {showSidebar && (
          <>
            <div style={{ width: sidebarWidth }} className="shrink-0 border-r border-border">
              {sidebarTab === 'files' && <FileExplorer authFetch={authFetch} onFileSelect={openFile} selectedPath={activeTab} workspace={workspace} />}
              {sidebarTab === 'git' && <GitPanel authFetch={authFetch} visible />}
            </div>
            <div className="w-1 cursor-col-resize hover:bg-accent/30 active:bg-accent/50 transition-colors"
              onMouseDown={handleMouseDown('sidebar')} />
          </>
        )}

        {/* Center: tabs + editor/browser + terminal */}
        <div className="flex-1 flex flex-col min-w-0">
          <div className="flex-1 flex flex-col min-h-0">
            <EditorTabs tabs={tabs} activeTab={activeTab} onSelect={setActiveTab} onClose={closeTab} />
            <div className="flex-1 min-h-0">{renderTabContent()}</div>
          </div>

          {showTerminal && (
            <>
              <div className="h-1 cursor-row-resize hover:bg-accent/30 active:bg-accent/50 transition-colors border-t border-border"
                onMouseDown={handleMouseDown('terminal')} />
              <div style={{ height: terminalHeight }} className="shrink-0">
                <TerminalPanel key={terminalKey} token={token} authFetch={authFetch} visible={showTerminal} />
              </div>
            </>
          )}
        </div>

        {/* Right panel: settings only */}
        {showSettings && (
          <>
            <div className="w-1 cursor-col-resize hover:bg-accent/30 active:bg-accent/50 transition-colors"
              onMouseDown={handleMouseDown('settings')} />
            <div style={{ width: settingsWidth }} className="shrink-0 border-l border-border">
              <Settings visible onOpenFolder={() => setShowFolderPicker(true)} workspace={workspace} />
            </div>
          </>
        )}
      </div>

      {/* Status bar */}
      <div className="flex items-center justify-between px-2 py-0.5 bg-status-bar text-status-text text-[11px] font-medium shrink-0">
        <div className="flex items-center gap-0.5">
          {gitBranch && (
            <button onClick={() => { toggleSidebarTab('git') }}
              className="flex items-center gap-1 px-1.5 py-0.5 rounded hover:bg-white/10 transition-colors">
              <GitBranch className="w-3 h-3" />
              <span>{gitBranch}</span>
              {gitChanges > 0 && <span className="opacity-75">*{gitChanges}</span>}
            </button>
          )}
          {currentTab?.modified && currentTab.type !== 'browser' && (
            <span className="px-1.5 opacity-75">Modified</span>
          )}
        </div>
        <div className="flex items-center gap-0.5">
          {currentTab && currentTab.type !== 'browser' && (
            <>
              <span className="px-1.5 py-0.5">Ln {cursor.line}, Col {cursor.col}</span>
              <span className="px-1.5 py-0.5 opacity-75">{extToLang(currentTab.name)}</span>
            </>
          )}
          {currentTab?.type === 'browser' && (
            <span className="px-1.5 py-0.5 opacity-75">Browser</span>
          )}
          <span className="px-1.5 py-0.5 opacity-60">UTF-8</span>
        </div>
      </div>
    </div>
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
