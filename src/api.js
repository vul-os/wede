// Central API helpers for the workspace-scoped backend.
//
// The backend exposes both legacy single-workspace routes (e.g. /api/files) that
// operate on the default workspace, and workspace-scoped routes under /api/workspaces/{id}/...
// As the frontend migrates, prefer workspaceUrl(workspaceId, suffix) to address a specific
// workspace; the legacy paths remain valid for the default workspace until migration done.

export const API = '/api'

// workspaceUrl builds a workspace-scoped API path.
//   workspaceUrl('abc', '/files')          -> /api/workspaces/abc/files
//   workspaceUrl('abc', '/git/status')     -> /api/workspaces/abc/git/status
export function workspaceUrl(workspaceId, suffix = '') {
  return `${API}/workspaces/${workspaceId}${suffix}`
}

// workspacesUrl is the workspace collection endpoint.
export const workspacesUrl = `${API}/workspaces`
