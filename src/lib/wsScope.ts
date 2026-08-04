// scopeToWorkspace rewrites a legacy /api/<service>... path to a SPECIFIC
// workspace's scoped path (/api/workspaces/<id>/<service>...). Unlike the global
// scopedUrl (which targets the active workspace), this pins a request to one
// named root — the primitive that lets several roots' trees, git panels, and
// searches coexist. Non-/api URLs and missing ids pass through unchanged.
import { workspaceUrl } from '../api'
import type { AuthFetch } from '../types'

export function scopeToWorkspace<T>(url: T, workspaceId: string | null | undefined): T {
  if (!workspaceId || typeof url !== 'string' || !url.startsWith('/api/')) return url
  return url.replace(/^\/api/, workspaceUrl(workspaceId)) as unknown as T
}

// makeWsFetch returns an authFetch-shaped function pinned to one workspace, so a
// component can reuse code written against the legacy /api/<service> paths.
export function makeWsFetch(authFetch: AuthFetch, workspaceId: string): AuthFetch {
  return (url, opts) => authFetch(scopeToWorkspace(url, workspaceId), opts)
}
