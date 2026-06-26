import { useState, useCallback } from 'react'

const API = '/api'

export function useAuth() {
  const [token, setToken] = useState(() => localStorage.getItem('wede_token'))
  const [username, setUsername] = useState(() => localStorage.getItem('wede_username') || '')
  const [error, setError] = useState(null)
  const [locked, setLocked] = useState(false)
  const [remaining, setRemaining] = useState(3)

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
        return true
      }
      setError('Unknown error')
      return false
    } catch {
      setError('Cannot connect to server')
      return false
    }
  }, [])

  const logout = useCallback(async () => {
    const t = localStorage.getItem('wede_token')
    localStorage.removeItem('wede_token')
    setToken(null)
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
    const res = await fetch(url, { ...options, headers })
    if (res.status === 401) {
      logout()
      throw new Error('unauthorized')
    }
    return res
  }, [token, logout])

  return { token, username, login, logout, error, locked, remaining, authFetch }
}
