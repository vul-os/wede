import { workspaceUrl } from '../api'
import type { AuthFetch } from '../types'

type Listener = { workspaceId: string; path: string; saved: (content: string) => void }

// Per-IDE channel, not a global event bus. Capture listeners when a write starts:
// a late response must not notify a replacement editor, even for the same file.
export function createDocumentSaves() {
  const listeners = new Set<Listener>()
  return {
    subscribe(workspaceId: string, path: string, saved: Listener['saved']) {
      const listener = { workspaceId, path, saved }
      listeners.add(listener)
      return () => { listeners.delete(listener) }
    },
    capture(workspaceId: string, path: string) {
      const recipients = [...listeners].filter(l => l.workspaceId === workspaceId && l.path === path)
      return (content: string) => {
        for (const listener of recipients) {
          if (listeners.has(listener)) listener.saved(content)
        }
      }
    },
  }
}

export type DocumentSaves = ReturnType<typeof createDocumentSaves>

export async function writeDocument(authFetch: AuthFetch, saves: DocumentSaves, workspaceId: string | null | undefined, path: string | undefined, content: string) {
  if (!workspaceId || !path) throw new Error('Missing document identity')
  const acknowledge = saves.capture(workspaceId, path)
  const response = await authFetch(workspaceUrl(encodeURIComponent(workspaceId), '/files/write'), {
    method: 'PUT', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, content }),
  })
  if (!response.ok) throw new Error(`Save failed (${response.status})`)
  const result: unknown = await response.json()
  if (!result || typeof result !== 'object' || !('status' in result) || result.status !== 'ok') {
    throw new Error('Missing disk-write acknowledgement')
  }
  acknowledge(content)
}
