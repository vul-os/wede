import { useState, useEffect, useCallback } from 'react'
import { workspacesUrl } from '../api'
import { setActiveWorkspaceId as syncActiveWorkspaceId } from '../lib/activeWorkspace'

// useWorkspaces tracks the set of open projects ("workspaces") and the active one.
//
// On login it fetches GET /api/workspaces and selects the default workspace (the one the
// boot workspace was adopted into) so the solo-user experience is unchanged.
// createWorkspace opens a new project; setActiveWorkspaceId switches the focused workspace.
export function useWorkspaces(token, authFetch) {
  const [workspaces, setWorkspaces] = useState([])
  const [activeWorkspaceId, setActiveWorkspaceId] = useState(null)

  // Keep the module-level holder (read by authFetch) in sync with the focused
  // workspace so legacy /api/<service> calls are rewritten to the active one.
  // Synced during render — not in an effect — so authFetch sees the new id
  // BEFORE descendant effects (e.g. the file explorer's first fetch) run on the
  // same commit. React runs effects child-first, so a parent effect would land
  // too late and the explorer would fetch the now-removed unscoped /api/files.
  syncActiveWorkspaceId(activeWorkspaceId)

  const refresh = useCallback(async () => {
    if (!token) return
    try {
      const res = await authFetch(workspacesUrl)
      const data = await res.json()
      const list = data.workspaces || []
      setWorkspaces(list)
      setActiveWorkspaceId((prev) => {
        if (prev && list.some((r) => r.id === prev)) return prev
        const def = list.find((r) => r.name === 'default') || list[0]
        return def ? def.id : null
      })
    } catch { /* ignore network/parse errors; caller UI degrades gracefully */ }
  }, [token, authFetch])

  const createWorkspace = useCallback(async (name, path) => {
    const res = await authFetch(workspacesUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, path }),
    })
    const workspace = await res.json()
    if (!res.ok) throw new Error(workspace.error || 'failed to create workspace')
    await refresh()
    return workspace
  }, [authFetch, refresh])

  // closeWorkspace removes a workspace root (editor+ only, server-side). The
  // caller is responsible for closing any tabs belonging to it first.
  const closeWorkspace = useCallback(async (id) => {
    const res = await authFetch(`${workspacesUrl}/${id}`, { method: 'DELETE' })
    if (!res.ok && res.status !== 404) {
      const data = await res.json().catch(() => ({}))
      throw new Error(data.error || 'failed to close workspace')
    }
    await refresh()
  }, [authFetch, refresh])

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (token) refresh()
  }, [token, refresh])
  /* eslint-enable react-hooks/set-state-in-effect */

  return { workspaces, activeWorkspaceId, setActiveWorkspaceId, createWorkspace, closeWorkspace, refresh }
}
