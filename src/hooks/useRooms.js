import { useState, useEffect, useCallback } from 'react'
import { roomsUrl } from '../api'

// useRooms tracks the set of open projects ("rooms") and the active one.
//
// On login it fetches GET /api/rooms and selects the default room (the one the
// boot workspace was adopted into) so the solo-user experience is unchanged.
// createRoom opens a new project; setActiveRoomId switches the focused room.
export function useRooms(token, authFetch) {
  const [rooms, setRooms] = useState([])
  const [activeRoomId, setActiveRoomId] = useState(null)

  const refresh = useCallback(async () => {
    if (!token) return
    try {
      const res = await authFetch(roomsUrl)
      const data = await res.json()
      const list = data.rooms || []
      setRooms(list)
      setActiveRoomId((prev) => {
        if (prev && list.some((r) => r.id === prev)) return prev
        const def = list.find((r) => r.name === 'default') || list[0]
        return def ? def.id : null
      })
    } catch { /* ignore network/parse errors; caller UI degrades gracefully */ }
  }, [token, authFetch])

  const createRoom = useCallback(async (name, path) => {
    const res = await authFetch(roomsUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, path }),
    })
    const room = await res.json()
    if (!res.ok) throw new Error(room.error || 'failed to create room')
    await refresh()
    return room
  }, [authFetch, refresh])

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (token) refresh()
  }, [token, refresh])
  /* eslint-enable react-hooks/set-state-in-effect */

  return { rooms, activeRoomId, setActiveRoomId, createRoom, refresh }
}
