import { useState, useCallback, useEffect } from 'react'
import { scopedUrl } from '../lib/activeWorkspace'

const API = '/api'

// SECURITY NOTE — session token storage.
//
// The session token is currently kept in localStorage so it survives reloads and
// is readable by the WebSocket clients (terminal/collab/chat) that pass it as an
// "auth.<token>" subprotocol. The downside is that any successful XSS could read
// it. The primary stored-XSS vector (rendering collaborators' markdown) is now
// closed by sanitizing with DOMPurify in MarkdownPreview, so a token-stealing
// payload can no longer be planted via file content.
//
// The robust fix is to move the token to an HttpOnly, SameSite cookie so JS can
// never read it. That is a backend change (Set-Cookie on login/redeem, read the
// cookie in auth.Middleware, clear it on logout, and switch the WS handshake to
// cookie auth) and is tracked as follow-up work; it is intentionally not done in
// this change to avoid destabilizing the WS auth path. Until then, keep the XSS
// surface minimal (DOMPurify above) and treat the token as sensitive.
export function useAuth() {
  const [token, setToken] = useState(() => localStorage.getItem('wede_token'))
  const [username, setUsername] = useState(() => localStorage.getItem('wede_username') || '')
  const [role, setRole] = useState(() => localStorage.getItem('wede_role') || '')
  const [error, setError] = useState(null)
  const [locked, setLocked] = useState(false)
  const [remaining, setRemaining] = useState(3)

  // On mount: confirm role from /api/auth/check so a persisted viewer/editor
  // session gets the correct role even after a page reload.
   
  useEffect(() => {
    const t = localStorage.getItem('wede_token')
    if (!t) return
    fetch(`${API}/auth/check`, { headers: { Authorization: t } })
      .then(r => r.json())
      .then(data => {
        if (data.authenticated && data.role) {
          setRole(data.role)
          localStorage.setItem('wede_role', data.role)
        } else if (!data.authenticated) {
          // Session expired server-side; clear stored credentials.
          localStorage.removeItem('wede_token')
          localStorage.removeItem('wede_username')
          localStorage.removeItem('wede_role')
          setToken(null)
          setUsername('')
          setRole('')
        }
      })
      .catch(() => {})
  }, []) // intentionally run once on mount only
   

  const login = useCallback(async (password, name = '') => {
    setError(null)
    try {
      const res = await fetch(`${API}/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password, username: name }),
      })
      const data = await res.json()
      if (data.error === 'locked') {
        setLocked(true)
        setError(data.message)
        return false
      }
      if (data.error === 'wrong_password') {
        setRemaining(data.remaining)
        setError(`Wrong password. ${data.remaining} attempt${data.remaining !== 1 ? 's' : ''} remaining.`)
        return false
      }
      if (data.token) {
        localStorage.setItem('wede_token', data.token)
        setToken(data.token)
        const resolved = data.username || name || ''
        localStorage.setItem('wede_username', resolved)
        setUsername(resolved)
        const resolvedRole = data.role || 'owner'
        localStorage.setItem('wede_role', resolvedRole)
        setRole(resolvedRole)
        return true
      }
      setError('Unknown error')
      return false
    } catch {
      setError('Cannot connect to server')
      return false
    }
  }, [])

  // redeem exchanges a raw invite token for a new session and stores the result
  // exactly like login does.
  const redeem = useCallback(async (inviteToken) => {
    try {
      const res = await fetch(`${API}/auth/redeem`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: inviteToken }),
      })
      if (!res.ok) return false
      const data = await res.json()
      if (data.token) {
        localStorage.setItem('wede_token', data.token)
        setToken(data.token)
        const resolved = data.username || ''
        localStorage.setItem('wede_username', resolved)
        setUsername(resolved)
        const resolvedRole = data.role || 'viewer'
        localStorage.setItem('wede_role', resolvedRole)
        setRole(resolvedRole)
        return true
      }
      return false
    } catch {
      return false
    }
  }, [])

  const logout = useCallback(async () => {
    const t = localStorage.getItem('wede_token')
    localStorage.removeItem('wede_token')
    localStorage.removeItem('wede_username')
    localStorage.removeItem('wede_role')
    setToken(null)
    setRole('')
    if (t) {
      // Revoke the token server-side (fire-and-forget; ignore network errors)
      fetch(`${API}/auth/logout`, {
        method: 'DELETE',
        headers: { Authorization: t },
      }).catch(() => {})
    }
  }, [])

  const authFetch = useCallback(async (url, options = {}) => {
    const headers = { ...options.headers, Authorization: token }
    const res = await fetch(scopedUrl(url), { ...options, headers })
    if (res.status === 401) {
      logout()
      throw new Error('unauthorized')
    }
    return res
  }, [token, logout])

  const updateUsername = useCallback((name) => {
    const v = (name || '').trim()
    setUsername(v)
    try {
      if (v) localStorage.setItem('wede_username', v)
      else localStorage.removeItem('wede_username')
    } catch { /* ignore */ }
  }, [])

  return { token, username, role, login, logout, redeem, error, locked, remaining, authFetch, updateUsername }
}
