// Derive a pure per-document extension. The mounted EditorView owns its socket;
// rendering this hook (even a discarded render) does not connect to the server.
import { useEffect, useMemo, useState, useSyncExternalStore } from 'react'
import { lspDocumentUris, lspExtension, type LspStatus } from '../lib/lsp'
import type { DocumentSaves } from '../lib/documentSaves'
import type { AuthFetch, Tab, WorkspaceSummary } from '../types'

// Map file extensions → LSP language name (must match backend knownServers).
const EXT_TO_LANG: Record<string, string> = {
  go: 'go',
  js: 'javascript', jsx: 'javascript', cjs: 'javascript', mjs: 'javascript',
  ts: 'typescript', tsx: 'typescript',
  py: 'python', pyw: 'python',
  rs: 'rust',
}

function extFromPath(path: string | null | undefined): string | null {
  if (!path) return null
  const dot = path.lastIndexOf('.')
  if (dot <= path.lastIndexOf('/')) return null
  return path.slice(dot + 1).toLowerCase()
}

function langFromFile(file: Pick<Tab, 'path'> | null | undefined): string | null {
  const ext = extFromPath(file?.path)
  return (ext && EXT_TO_LANG[ext]) || null
}

interface LspAvailableResponse {
  available?: Record<string, boolean>
  extensions?: Record<string, string>
}

function createStatusStore() {
  let status: LspStatus | null = null
  const listeners = new Set<() => void>()
  return {
    getSnapshot: () => status,
    subscribe(this: void, listener: () => void) {
      listeners.add(listener)
      return () => { listeners.delete(listener) }
    },
    set(this: void, next: LspStatus | null) {
      status = next
      for (const listener of listeners) listener()
    },
  }
}
const inactiveStatus = createStatusStore()

export interface UseLSPParams {
  file: Tab | null | undefined
  token: string | null | undefined
  authFetch: AuthFetch | null | undefined
  lspEnabled: boolean
  workspaces: readonly WorkspaceSummary[] | undefined
  saves?: DocumentSaves
}

export function useLSP({ file, token, authFetch, lspEnabled, workspaces, saves }: UseLSPParams) {
  const [available, setAvailable] = useState<Record<string, boolean> | null>(null)
  const [extMap, setExtMap] = useState<Record<string, string>>(EXT_TO_LANG)

  useEffect(() => {
    if (!authFetch) return
    let cancelled = false
    const controller = new AbortController()
    void authFetch('/api/lsp/available', { signal: controller.signal })
      .then(r => r.json() as Promise<LspAvailableResponse>)
      .then(data => {
        if (cancelled) return
        setAvailable(data.available || {})
        setExtMap({ ...EXT_TO_LANG, ...data.extensions })
      })
      .catch(() => { if (!cancelled) setAvailable({}) })
    return () => {
      cancelled = true
      controller.abort()
    }
  }, [authFetch])

  // Resolve all three from the tab, never from the asynchronously updated global
  // active workspace. A closed/unknown workspace cannot create a connection.
  const workspaceId = file?.workspaceId
  const root = workspaces?.find(w => w.id === workspaceId)?.root
  const relativePath = file && !file.type && !file.fileType ? file.rel : null
  const ext = extFromPath(relativePath)
  const languageId = (ext && extMap[ext]) || null
  const serverAvailable = !!(languageId && available?.[languageId])
  const session = useMemo(() => {
    if (!lspEnabled || !token || !workspaceId || !root || !relativePath || !languageId || !serverAvailable) return null
    const uris = lspDocumentUris(root, relativePath)
    if (!uris) return null
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const serverUri = `${proto}//${window.location.host}/api/workspaces/${encodeURIComponent(workspaceId)}/lsp?lang=${encodeURIComponent(languageId)}&token=${encodeURIComponent(token)}`
    const status = createStatusStore()
    return { status, extension: lspExtension({
      ...uris, languageId, serverUri,
      persistence: saves ? { events: saves, workspaceId, path: relativePath } : undefined,
      onStatus: status.set,
    }) }
  }, [relativePath, workspaceId, root, lspEnabled, token, languageId, serverAvailable, saves])

  // Each configuration owns its store; a late callback from a prior workspace
  // cannot overwrite the active document's status or force a reconnect.
  const store = session?.status ?? inactiveStatus
  const status = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot)
  return { extension: session?.extension ?? null, available, status }
}

export { langFromFile, EXT_TO_LANG }
