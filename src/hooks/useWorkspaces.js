import { useState, useEffect, useCallback } from 'react'
import { workspacesUrl } from '../api'

// useWorkspaces tracks the set of open projects ("workspaces") and the active one.
//
// On login it fetches GET /api/workspaces and selects the default workspace (the one the
// boot workspace was adopted into) so the solo-user experience is unchanged.
// createWorkspace opens a new project; setActiveWorkspaceId switches the focused workspace.
export function useWorkspaces(token, authFetch) {
  const [workspaces, setWorkspaces] = useState([])
  const [activeWorkspaceId, setActiveWorkspaceId] = useState(null)

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

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (token) refresh()
  }, [token, refresh])
  /* eslint-enable react-hooks/set-state-in-effect */

  return { workspaces, activeWorkspaceId, setActiveWorkspaceId, createWorkspace, refresh }
}
