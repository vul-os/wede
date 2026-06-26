import { useState, useEffect, useCallback } from 'react'
import { useAuth } from './hooks/useAuth'
import { useTheme } from './hooks/useTheme'
import { useRooms } from './hooks/useRooms'
import Logo from './components/Logo'
import ThemePicker from './components/ThemePicker'
import Login from './components/Login'
import FolderPicker from './components/FolderPicker'
import IDE from './components/IDE'

function App() {
  const { token, login, logout, error, locked, remaining, authFetch } = useAuth()
  const { theme, setTheme } = useTheme()
  const roomsApi = useRooms(token, authFetch)
  const [workspace, setWorkspace] = useState(null)
  const [loading, setLoading] = useState(true)

  const fetchWorkspace = useCallback(async () => {
    if (!token) return
    try {
      const res = await authFetch('/api/workspace')
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

  // First visit - pick theme
  if (!theme) {
    return <ThemePicker onSelect={setTheme} />
  }

  if (!token) {
    return <Login onLogin={login} error={error} locked={locked} remaining={remaining} />
  }

  if (loading) {
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

  return (
    <IDE
      token={token}
      authFetch={authFetch}
      onLogout={logout}
      workspace={workspace.current}
      recents={workspace?.recents || []}
      onWorkspaceChange={(path) => setWorkspace({ ...workspace, current: path, hasWorkspace: true })}
      roomId={roomsApi.activeRoomId}
      roomsApi={roomsApi}
    />
  )
}

export default App
