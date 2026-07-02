import { useState, useEffect, useCallback } from 'react'
import { useAuth } from './hooks/useAuth'
import { useTheme } from './hooks/useTheme'
import { useWorkspaces } from './hooks/useWorkspaces'
import Logo from './components/Logo'
import ThemePicker from './components/ThemePicker'
import Login from './components/Login'
import FolderPicker from './components/FolderPicker'
import IDE from './components/IDE'

function App() {
  const { token, username, role, login, logout, redeem, error, locked, remaining, authFetch, updateUsername } = useAuth()
  const { theme, setTheme } = useTheme()
  const workspacesApi = useWorkspaces(token, authFetch)
  const [workspace, setWorkspace] = useState(null)
  const [loading, setLoading] = useState(true)

  const fetchWorkspace = useCallback(async () => {
    if (!token) return
    try {
      const res = await authFetch('/api/folder')
      const data = await res.json()
      setWorkspace(data)
    } catch { /* ignore */ }
    setLoading(false)
  }, [token, authFetch])

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (token) fetchWorkspace()
    else setLoading(false)
  }, [token, fetchWorkspace])
  /* eslint-enable react-hooks/set-state-in-effect */

  // Handle ?invite=TOKEN in the URL: redeem the token then strip the param so
  // the invite link isn't reusable by copy-pasting from the address bar.
   
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const invite = params.get('invite')
    if (!invite) return
    history.replaceState({}, '', window.location.pathname)
    redeem(invite).catch(() => {})
  }, [redeem])
   

  // First visit - pick theme
  if (!theme) {
    return <ThemePicker onSelect={setTheme} />
  }

  if (!token) {
    return <Login onLogin={login} error={error} locked={locked} remaining={remaining} />
  }

  // Wait for the active workspace to resolve before mounting the IDE, so every
  // file/git/search request is workspace-scoped from the first render.
  if (loading || !workspacesApi.activeWorkspaceId) {
    return (
      <div className="min-h-screen bg-bg-tertiary flex items-center justify-center">
        <div className="animate-fade-in text-center">
          <div className="w-10 h-10 rounded-xl bg-bg-primary border border-border flex items-center justify-center mx-auto mb-3 overflow-hidden">
            <Logo size={28} />
          </div>
          <div className="text-text-muted text-sm">Loading...</div>
        </div>
      </div>
    )
  }

  if (!workspace?.hasWorkspace) {
    return (
      <FolderPicker
        authFetch={authFetch}
        recents={workspace?.recents || []}
        onOpen={(path) => setWorkspace({ ...workspace, current: path, hasWorkspace: true })}
      />
    )
  }

  // The active root path tracks the focused workspace (which follows the active
  // editor tab in multi-root), so the top bar / status bar / DAP reflect it.
  const activeWs = workspacesApi.workspaces.find((w) => w.id === workspacesApi.activeWorkspaceId)
  const activeRoot = activeWs?.root || workspace.current

  return (
    <IDE
      token={token}
      authFetch={authFetch}
      onLogout={logout}
      workspace={activeRoot}
      recents={workspace?.recents || []}
      onWorkspaceChange={(path) => setWorkspace({ ...workspace, current: path, hasWorkspace: true })}
      workspaceId={workspacesApi.activeWorkspaceId}
      workspacesApi={workspacesApi}
      username={username}
      onUsernameChange={updateUsername}
      role={role}
    />
  )
}

export default App
