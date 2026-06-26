// useYDoc — opens a Yjs document synced to the server's CRDT doc for one file.
//
// Connects a y-websocket WebsocketProvider to the ygo doc endpoint
// /api/workspaces/{id}/doc/{workspace} where {workspace} is base64url(relative path) (matching
// the Go backend's decodeRoom). The provider speaks the y-protocols sync +
// awareness wire format that ygo's provider/websocket implements. Exposes the
// shared Y.Text 'content' plus the provider/awareness so the editor can bind a
// yCollab extension and render remote cursors.
//
// Defensive by design: missing inputs or any construction error leave collab
// inactive (all nulls); the editor falls back to its normal single-user mode.

import { useEffect, useState } from 'react'
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'

// b64urlPath encodes a (possibly UTF-8) path as base64url without padding —
// byte-for-byte compatible with Go's base64.RawURLEncoding used by decodeRoom.
export function b64urlPath(path) {
  const b64 = btoa(unescape(encodeURIComponent(path)))
  return b64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function docBaseUrl(workspaceId) {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const port = window.location.port
  const host = (port === '5173' || port === '5174')
    ? window.location.hostname + ':9090'
    : window.location.host
  return `${proto}//${host}/api/workspaces/${encodeURIComponent(workspaceId)}/doc`
}

export function useYDoc({ workspaceId, path, token, username, color }) {
  const [state, setState] = useState({ ytext: null, provider: null, awareness: null })

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (!workspaceId || !path || !token) {
      setState({ ytext: null, provider: null, awareness: null })
      return undefined
    }

    let ydoc
    let provider
    try {
      ydoc = new Y.Doc()
      // y-websocket builds the socket URL as `${base}/${workspace}?${params}`, which
      // matches our route /api/workspaces/{id}/doc/{workspace...} + ?token= (the auth
      // middleware authenticates WS upgrades via ?token=).
      provider = new WebsocketProvider(docBaseUrl(workspaceId), b64urlPath(path), ydoc, {
        params: { token },
        connect: true,
      })
      provider.awareness.setLocalStateField('user', {
        name: username || 'anon',
        color: color || '#888',
      })
      const ytext = ydoc.getText('content')
      setState({ ytext, provider, awareness: provider.awareness })
    } catch {
      // Construction failed → collab inactive; editor stays single-user.
      try { if (provider) provider.destroy() } catch { /* ignore */ }
      try { if (ydoc) ydoc.destroy() } catch { /* ignore */ }
      setState({ ytext: null, provider: null, awareness: null })
      return undefined
    }

    return () => {
      try { provider.destroy() } catch { /* ignore */ }
      try { ydoc.destroy() } catch { /* ignore */ }
    }
  }, [workspaceId, path, token, username, color])
  /* eslint-enable react-hooks/set-state-in-effect */

  return state
}
