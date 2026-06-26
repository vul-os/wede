// Central API helpers for the room-scoped backend.
//
// The backend exposes both legacy single-workspace routes (e.g. /api/files) that
// operate on the default room, and room-scoped routes under /api/rooms/{id}/...
// As the frontend migrates, prefer roomUrl(roomId, suffix) to address a specific
// room; the legacy paths remain valid for the default room until migration done.

export const API = '/api'

// roomUrl builds a room-scoped API path.
//   roomUrl('abc', '/files')          -> /api/rooms/abc/files
//   roomUrl('abc', '/git/status')     -> /api/rooms/abc/git/status
export function roomUrl(roomId, suffix = '') {
  return `${API}/rooms/${roomId}${suffix}`
}

// roomsUrl is the room collection endpoint.
export const roomsUrl = `${API}/rooms`
