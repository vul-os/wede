// Shared domain types for the wede frontend, used across lib/hooks/components.
// Kept intentionally loose where the backend's JSON shapes are informally typed
// (this is a type-level migration only — no runtime validation was added).

// An open editor tab. Ordinary file tabs carry `rel` + `workspaceId` and a
// composite `path` (see lib/fileKey.ts); "special" tabs (browser/gitgraph/
// apiclient) use a `type`-prefixed pseudo-id as `path` and no `rel`/`workspaceId`.
export interface Tab {
  path: string
  rel?: string
  workspaceId?: string | null
  name?: string
  type?: 'browser' | 'gitgraph' | 'apiclient' | string
  url?: string
  content?: string
  originalContent?: string
  modified?: boolean
  manual?: boolean
  preview?: boolean
  fileType?: 'image' | 'binary' | string
  dataUrl?: string
  size?: number
  targetLine?: number
  [key: string]: unknown
}

// A file/folder entry as surfaced by the file tree, search results, or quick
// open — all three shapes are used interchangeably as openFile() targets.
export interface FileEntry {
  path: string
  name?: string
  rel?: string
  workspaceId?: string | null
  isDir?: boolean
  [key: string]: unknown
}

export interface WorkspaceSummary {
  id: string
  name: string
  path?: string
  root?: string
  [key: string]: unknown
}

// authFetch shape returned by useAuth — a fetch-alike that injects the auth
// header, rewrites legacy /api paths to the active workspace, and logs out on 401.
export type AuthFetch = (url: string, options?: RequestInit) => Promise<Response>

export interface TerminalSession {
  id: number
  name: string
  manual?: boolean
  initial?: string
}

// Imperative handle exposed by the <Terminal> component (via useImperativeHandle)
// so the mobile toolbar and floating-window chrome can send raw input.
export interface TerminalHandle {
  send: (data: string) => void
}

// A member of a workspace's live collaboration roster, as broadcast by the
// backend's /collab websocket ({type:'presence', members:[...]}).
export interface PresenceMember {
  id: string
  username?: string
  color?: string
  file?: string
  [key: string]: unknown
}

export interface MousePosition {
  x: number
  y: number
}

// A persisted/live chat message, as broadcast by the backend's /chat websocket
// ({type:'history', messages:[...]} on join, {type:'msg', message:{...}} live).
export interface ChatMessage {
  id?: string | number
  kind: 'user' | 'git' | string
  user?: string
  color?: string
  text?: string
  time?: string
  [key: string]: unknown
}

// Persisted editor preferences (src/hooks lives outside this, IDE.tsx owns the
// state) — read by Editor/Settings/DebugPanel.
export interface EditorSettings {
  fontSize: number
  tabWidth: number
  wordWrap: boolean
  autoSave: boolean
  minimap: boolean
  lsp: boolean
  formatOnSave: boolean
  collab?: boolean
  shareCursors?: boolean
  [key: string]: unknown
}

// A user-defined task from ~/.wede/tasks.json (or a trusted project's
// .wede/tasks.json), run in a new terminal by TasksPanel/IDE.
export interface Task {
  name?: string
  command: string
  cwd?: string
}

// Actions a mounted <FileExplorer> registers with the parent IDE so the
// command palette / SSE watcher can trigger them without prop drilling.
export interface ExplorerActions {
  refresh: () => void
  newFile: () => void
  newFolder: () => void
}

// Actions a mounted <Editor> registers with the parent IDE (currently just
// Ctrl+G "go to line").
export interface EditorActions {
  goToLine: () => void
}

// Which DAP-capable languages are available server-side, keyed by language id
// (from GET /api/dap/available), plus the extension→language map.
export interface DapAvailableInfo {
  available: Record<string, boolean>
  extensions: Record<string, string>
}

