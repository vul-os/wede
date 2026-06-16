import { Moon, Sun, FolderOpen, Info, Minus, Plus } from 'lucide-react'
import { useTheme } from '../hooks/useTheme'
import Logo from './Logo'

export default function Settings({ visible, onOpenFolder, workspace, editorSettings, onEditorSettingsChange, lspAvailable }) {
  const { setTheme, isDark } = useTheme()

  if (!visible) return null

  const s = editorSettings || {}
  const fontSize  = s.fontSize  ?? 13
  const tabWidth  = s.tabWidth  ?? 2
  const wordWrap     = s.wordWrap     ?? false
  const autoSave     = s.autoSave     ?? true
  const minimap      = s.minimap      ?? false
  const lspOn        = s.lsp          ?? true
  const formatOnSave = s.formatOnSave ?? false

  // Derive LSP status hint for the user.
  // lspAvailable: null = not yet fetched, {} = fetched (possibly empty), { go: '...' } = servers found
  const lspHint = (() => {
    if (lspAvailable === null) return null
    const names = Object.keys(lspAvailable)
    if (names.length === 0) return 'No language servers found on PATH. Install gopls, typescript-language-server, or pylsp to enable diagnostics and hover.'
    return `Active for: ${names.join(', ')}`
  })()

  const update = (patch) => onEditorSettingsChange?.({ ...s, ...patch })

  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-y-auto">
      <div className="px-4 py-3 border-b border-border">
        <h2 className="text-sm font-semibold text-text-primary">Settings</h2>
      </div>

      <div className="p-4 space-y-6">
        {/* Theme */}
        <div>
          <h3 className="text-xs font-semibold uppercase tracking-wider text-text-muted mb-3">Appearance</h3>
          <div className="grid grid-cols-2 gap-3">
            <button
              onClick={() => setTheme('dark')}
              className={`rounded-xl border-2 p-3 transition-all ${
                isDark
                  ? 'border-accent bg-accent/5'
                  : 'border-border hover:border-border-active'
              }`}
            >
              <div className="rounded-lg bg-[#111827] border border-[#1e293b] p-2.5 mb-2">
                <div className="flex gap-1 mb-1.5">
                  <div className="w-1.5 h-1.5 rounded-full bg-[#fb7185]" />
                  <div className="w-1.5 h-1.5 rounded-full bg-[#fbbf24]" />
                  <div className="w-1.5 h-1.5 rounded-full bg-[#34d399]" />
                </div>
                <div className="space-y-1">
                  <div className="h-1 w-3/4 rounded bg-[#60a5fa]/30" />
                  <div className="h-1 w-1/2 rounded bg-[#a78bfa]/30" />
                  <div className="h-1 w-5/6 rounded bg-[#34d399]/20" />
                </div>
              </div>
              <div className="flex items-center justify-center gap-1.5 text-xs">
                <Moon className="w-3.5 h-3.5" />
                <span className="font-medium">Midnight</span>
              </div>
            </button>

            <button
              onClick={() => setTheme('light')}
              className={`rounded-xl border-2 p-3 transition-all ${
                !isDark
                  ? 'border-accent bg-accent/5'
                  : 'border-border hover:border-border-active'
              }`}
            >
              <div className="rounded-lg bg-white border border-[#e2e8f0] p-2.5 mb-2">
                <div className="flex gap-1 mb-1.5">
                  <div className="w-1.5 h-1.5 rounded-full bg-[#dc2626]" />
                  <div className="w-1.5 h-1.5 rounded-full bg-[#d97706]" />
                  <div className="w-1.5 h-1.5 rounded-full bg-[#16a34a]" />
                </div>
                <div className="space-y-1">
                  <div className="h-1 w-3/4 rounded bg-[#3b82f6]/30" />
                  <div className="h-1 w-1/2 rounded bg-[#7c3aed]/30" />
                  <div className="h-1 w-5/6 rounded bg-[#16a34a]/20" />
                </div>
              </div>
              <div className="flex items-center justify-center gap-1.5 text-xs text-[#475569]">
                <Sun className="w-3.5 h-3.5" />
                <span className="font-medium">Daylight</span>
              </div>
            </button>
          </div>
        </div>

        {/* Editor settings */}
        <div>
          <h3 className="text-xs font-semibold uppercase tracking-wider text-text-muted mb-3">Editor</h3>
          <div className="space-y-2">
            {/* Font size */}
            <div className="flex items-center justify-between bg-bg-primary border border-border rounded-lg px-3 py-2.5">
              <span className="text-xs text-text-secondary">Font size</span>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => update({ fontSize: Math.max(10, fontSize - 1) })}
                  className="w-5 h-5 flex items-center justify-center rounded text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors"
                >
                  <Minus className="w-3 h-3" />
                </button>
                <span className="text-xs font-mono text-text-primary w-6 text-center">{fontSize}</span>
                <button
                  onClick={() => update({ fontSize: Math.min(24, fontSize + 1) })}
                  className="w-5 h-5 flex items-center justify-center rounded text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors"
                >
                  <Plus className="w-3 h-3" />
                </button>
              </div>
            </div>

            {/* Tab width */}
            <div className="flex items-center justify-between bg-bg-primary border border-border rounded-lg px-3 py-2.5">
              <span className="text-xs text-text-secondary">Tab width</span>
              <div className="flex items-center gap-1">
                {[2, 4, 8].map((n) => (
                  <button
                    key={n}
                    onClick={() => update({ tabWidth: n })}
                    className={`text-[11px] font-mono px-2 py-0.5 rounded transition-colors ${
                      tabWidth === n
                        ? 'bg-accent/15 text-accent border border-accent/30'
                        : 'text-text-muted hover:text-text-primary hover:bg-bg-hover border border-transparent'
                    }`}
                  >
                    {n}
                  </button>
                ))}
              </div>
            </div>

            {/* Word wrap */}
            <label className="flex items-center justify-between bg-bg-primary border border-border rounded-lg px-3 py-2.5 cursor-pointer group">
              <span className="text-xs text-text-secondary group-hover:text-text-primary transition-colors">Word wrap</span>
              <Toggle checked={wordWrap} onChange={(v) => update({ wordWrap: v })} />
            </label>

            {/* Auto-save */}
            <label className="flex items-center justify-between bg-bg-primary border border-border rounded-lg px-3 py-2.5 cursor-pointer group">
              <div>
                <span className="text-xs text-text-secondary group-hover:text-text-primary transition-colors">Auto-save</span>
                <span className="block text-[10px] text-text-muted">1.5 s after last edit</span>
              </div>
              <Toggle checked={autoSave} onChange={(v) => update({ autoSave: v })} />
            </label>

            {/* Minimap */}
            <label className="flex items-center justify-between bg-bg-primary border border-border rounded-lg px-3 py-2.5 cursor-pointer group">
              <span className="text-xs text-text-secondary group-hover:text-text-primary transition-colors">Minimap</span>
              <Toggle checked={minimap} onChange={(v) => update({ minimap: v })} />
            </label>

            {/* Format on save */}
            <label className="flex items-center justify-between bg-bg-primary border border-border rounded-lg px-3 py-2.5 cursor-pointer group">
              <div>
                <span className="text-xs text-text-secondary group-hover:text-text-primary transition-colors">Format on save</span>
                <span className="block text-[10px] text-text-muted">gofmt / prettier / black</span>
              </div>
              <Toggle checked={formatOnSave} onChange={(v) => update({ formatOnSave: v })} />
            </label>

            {/* LSP */}
            <label className="flex items-center justify-between bg-bg-primary border border-border rounded-lg px-3 py-2.5 cursor-pointer group">
              <div>
                <span className="text-xs text-text-secondary group-hover:text-text-primary transition-colors">Language server (LSP)</span>
                <span className="block text-[10px] text-text-muted">Diagnostics &amp; hover</span>
              </div>
              <Toggle checked={lspOn} onChange={(v) => update({ lsp: v })} />
            </label>

            {/* LSP availability hint */}
            {lspHint && (
              <div className="flex items-start gap-2 px-3 py-2 rounded-lg bg-bg-primary border border-border">
                <Info className="w-3.5 h-3.5 text-text-muted shrink-0 mt-px" />
                <span className="text-[10px] text-text-muted leading-relaxed">{lspHint}</span>
              </div>
            )}
          </div>
        </div>

        {/* Workspace */}
        <div>
          <h3 className="text-xs font-semibold uppercase tracking-wider text-text-muted mb-3">Workspace</h3>
          <div className="bg-bg-primary border border-border rounded-lg p-3">
            <div className="flex items-center gap-2 mb-2">
              <FolderOpen className="w-4 h-4 text-yellow" />
              <span className="text-xs text-text-secondary font-mono truncate flex-1">
                {workspace || 'No folder open'}
              </span>
            </div>
            <button
              onClick={onOpenFolder}
              className="w-full text-xs py-2 rounded-lg border border-border text-text-secondary hover:text-text-primary hover:bg-bg-hover transition-colors"
            >
              Change Folder
            </button>
          </div>
        </div>

        {/* About */}
        <div>
          <h3 className="text-xs font-semibold uppercase tracking-wider text-text-muted mb-3">About</h3>
          <div className="bg-bg-primary border border-border rounded-lg p-3">
            <div className="flex items-center gap-2 mb-2">
              <div className="w-8 h-8 rounded-lg bg-bg-hover flex items-center justify-center overflow-hidden">
                <Logo size={24} />
              </div>
              <div>
                <div className="text-sm font-semibold text-text-primary">wede</div>
                <div className="text-[11px] text-text-muted">Web Development Environment</div>
              </div>
            </div>
            <div className="text-[11px] text-text-muted space-y-0.5 mt-2">
              <p>A lightweight, self-hosted web IDE.</p>
              <p>Single binary. No cloud required.</p>
            </div>
            <div className="flex items-center gap-1.5 mt-3 pt-2 border-t border-border">
              <svg className="w-3.5 h-3.5 text-text-muted shrink-0" viewBox="0 0 16 16" fill="currentColor">
                <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z" />
              </svg>
              <span className="text-[10px] text-text-muted">
                Vibed by <a href="https://github.com/imranparuk" target="_blank" rel="noopener noreferrer" className="text-accent hover:underline">Imran</a> on behalf of <a href="https://vulos.org" target="_blank" rel="noopener noreferrer" className="text-accent hover:underline">Vulos</a>
              </span>
            </div>
          </div>
        </div>

        {/* Keyboard shortcuts */}
        <div>
          <h3 className="text-xs font-semibold uppercase tracking-wider text-text-muted mb-3">Shortcuts</h3>
          <div className="space-y-1.5 text-xs">
            {[
              ['Command palette', 'Ctrl/Cmd + Shift + P'],
              ['Save file', 'Ctrl/Cmd + S'],
              ['Go to line', 'Ctrl + G'],
              ['Search in file', 'Ctrl/Cmd + F'],
              ['Project search', 'Ctrl/Cmd + Shift + F'],
              ['Close tab', 'Ctrl/Cmd + W'],
              ['Multi-cursor', 'Alt + Click'],
              ['Column select', 'Alt + Drag'],
            ].map(([action, keys]) => (
              <div key={action} className="flex items-center justify-between py-1.5 px-3 bg-bg-primary rounded-lg border border-border">
                <span className="text-text-secondary">{action}</span>
                <kbd className="text-[10px] font-mono text-text-muted bg-bg-hover px-1.5 py-0.5 rounded border border-border">
                  {keys}
                </kbd>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function Toggle({ checked, onChange }) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={`relative w-8 h-4.5 rounded-full transition-colors shrink-0 ${checked ? 'bg-accent' : 'bg-bg-active border border-border'}`}
      style={{ height: '18px', width: '32px' }}
    >
      <span
        className={`absolute top-0.5 w-3.5 h-3.5 rounded-full bg-white shadow transition-transform ${checked ? 'translate-x-3.5' : 'translate-x-0.5'}`}
        style={{ height: '14px', width: '14px' }}
      />
    </button>
  )
}
