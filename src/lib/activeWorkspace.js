// Module-level holder for the focused workspace id. useWorkspaces keeps it in
// sync; authFetch reads it to rewrite legacy /api/<service> paths to the
// workspace-scoped /api/workspaces/<id>/<service> form, so switching workspaces
// repoints all file/git/search/etc. requests with no per-component changes.
let activeWorkspaceId = null

export function setActiveWorkspaceId(id) { activeWorkspaceId = id }
export function getActiveWorkspaceId() { return activeWorkspaceId }

// Services that are workspace-scoped on the server. Matched as /api/<service>(/|?|end).
const SCOPED = /^\/api\/(files|git|search|watch|lsp|terminal)(\/|\?|$)/

// scopedUrl rewrites a legacy /api/<service>... path to the active workspace's
// scoped path. Already-scoped, auth, folder, tunnel, etc. paths pass through.
export function scopedUrl(url) {
  if (typeof url !== 'string' || !activeWorkspaceId) return url
  if (!SCOPED.test(url)) return url
  return url.replace(/^\/api/, `/api/workspaces/${encodeURIComponent(activeWorkspaceId)}`)
}
