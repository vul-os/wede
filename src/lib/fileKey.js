// Composite identity for an open editor tab in a multi-root workspace.
//
// A file is identified by (workspaceId, relPath). We encode that pair into a
// single opaque string used as the tab's `path` so the many `tab.path === activeTab`
// comparisons throughout the IDE keep working unchanged, while `tab.rel` and
// `tab.workspaceId` carry the parts needed for the actual API calls.
//
// The separator is NUL — legal in neither a workspace id (hex) nor a POSIX path
// component (paths may contain spaces, so a space would be unsafe) — so it can
// never collide with real path content.
//
// Special tabs (browser:*, gitgraph:*, apiclient:*) keep their own pseudo-id as
// `path` with no separator; isFileKey() returns false for them.

export const KEY_SEP = String.fromCharCode(0)

export function fileKey(workspaceId, rel) {
  return `${workspaceId ?? ''}${KEY_SEP}${rel ?? ''}`
}

export function parseFileKey(key) {
  const i = typeof key === 'string' ? key.indexOf(KEY_SEP) : -1
  if (i === -1) return { workspaceId: null, rel: typeof key === 'string' ? key : '' }
  return { workspaceId: key.slice(0, i), rel: key.slice(i + 1) }
}

export function isFileKey(key) {
  return typeof key === 'string' && key.includes(KEY_SEP)
}

// Special tabs (browser/gitgraph/apiclient) use a `type:`-prefixed pseudo-id as
// their path and are never files.
const SPECIAL_TAB = /^(browser|gitgraph|apiclient):/
export function isSpecialTabId(id) {
  return typeof id === 'string' && SPECIAL_TAB.test(id)
}

// normalizeTabIdentity upgrades a persisted editor tab to the multi-root
// composite identity. Tabs saved before that change have a bare relative
// `path` and no `rel`/`workspaceId`; without repair, save/format/autosave would
// POST to /api/workspaces/undefined/... with an undefined path and silently
// fail. This backfills rel + workspaceId and rebuilds `path` as the composite
// key. Special tabs and already-composite tabs pass through unchanged.
export function normalizeTabIdentity(tab, activeWorkspaceId) {
  if (!tab || tab.type || isSpecialTabId(tab.path)) return tab
  const parsed = parseFileKey(tab.path)
  const rel = tab.rel ?? parsed.rel
  const workspaceId = tab.workspaceId || parsed.workspaceId || activeWorkspaceId
  if (!workspaceId) return tab // can't resolve a root yet — leave as-is
  const path = fileKey(workspaceId, rel)
  if (tab.path === path && tab.rel === rel && tab.workspaceId === workspaceId) return tab
  return { ...tab, rel, workspaceId, path }
}
