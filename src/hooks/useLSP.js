// useLSP — manages per-file LSP extensions for the CodeMirror editor.
//
// Responsibilities:
//  1. On mount, fetch GET /api/lsp/available to learn which language servers
//     are installed.
//  2. When the active file changes, derive the language from its extension and
//     (if a server is available) build a `languageServer` extension pointed at
//     the wss/ws LSP WebSocket endpoint.
//  3. Expose the extension (or null) plus the available-servers map so Settings
//     can display a hint when no servers are installed.
//
// The hook intentionally does not throw if the server is unavailable — it
// degrades gracefully to null (no LSP).

import { useEffect, useMemo, useState } from 'react'
import { languageServer } from 'codemirror-languageserver'

// Map file extensions → LSP language name (must match backend knownServers).
const EXT_TO_LANG = {
  go: 'go',
  js: 'javascript', jsx: 'javascript', cjs: 'javascript', mjs: 'javascript',
  ts: 'typescript', tsx: 'typescript',
  py: 'python', pyw: 'python',
  rs: 'rust',
}

function extFromPath(path) {
  if (!path) return null
  const dot = path.lastIndexOf('.')
  if (dot === -1) return null
  return path.slice(dot + 1).toLowerCase()
}

function langFromFile(file) {
  if (!file?.path) return null
  return EXT_TO_LANG[extFromPath(file.path)] || null
}

function buildWsUrl(lang, token, workspaceId) {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const path = workspaceId ? `/api/workspaces/${encodeURIComponent(workspaceId)}/lsp` : '/api/lsp'
  const base = `${proto}//${window.location.host}${path}?lang=${encodeURIComponent(lang)}`
  // Token passed as ?token= because WebSocket subprotocol approach mirrors
  // the terminal but codemirror-languageserver manages the WS internally.
  // The auth middleware accepts ?token= on WS upgrade requests.
  return `${base}&token=${encodeURIComponent(token)}`
}

export function useLSP({ file, token, authFetch, lspEnabled, workspaceId }) {
  const [available, setAvailable] = useState(null) // null = not yet fetched

  // Fetch available servers once on mount.
  useEffect(() => {
    if (!authFetch) return
    authFetch('/api/lsp/available')
      .then(r => r.json())
      .then(data => setAvailable(data.available || {}))
      .catch(() => setAvailable({}))
  }, [authFetch])

  // Derive the LSP extension synchronously from the current state.
  // useMemo avoids creating a new extension object on every render.
  // We key on the file path string specifically so that a mere content change
  // (same path, different text) does not tear down and rebuild the WS connection.
  const filePath = file?.path ?? null
  const extension = useMemo(() => {
    if (!lspEnabled || !filePath || available === null) return null

    const lang = langFromFile({ path: filePath })
    if (!lang || !available[lang]) return null

    const wsUrl = buildWsUrl(lang, token, workspaceId)
    const documentUri = `file://${filePath}`

    try {
      return languageServer({
        serverUri: wsUrl,
        rootUri: null,
        documentUri,
        languageId: lang,
        workspaceFolders: null,
      })
    } catch (err) {
      console.warn('[lsp] failed to create extension:', err)
      return null
    }
  }, [filePath, lspEnabled, available, token, workspaceId])

  return { extension, available }
}

export { langFromFile, EXT_TO_LANG }
