// useApiClient — shared state for the built-in API client, so the collections
// (rendered in the IDE sidebar like the file explorer) and the request editor
// (rendered in the editor area) stay in sync. The IDE creates one instance and
// passes it to both.

import { useState, useEffect, useCallback } from 'react'
import { parseReq, type ApiRequest, type ApiTreeNode, type ApiEnvironment, type ApiVars } from '../lib/apiRequest'
import type { AuthFetch } from '../types'

export const blankRequest = (): ApiRequest => ({
  name: 'New Request', method: 'GET', url: '',
  params: [], headers: [], auth: { type: 'none' },
  body: { type: 'none', content: '', form: [] },
})

interface ApiClientLoadResponse {
  tree?: ApiTreeNode[]
  environments?: ApiEnvironment[]
}

export function useApiClient(workspaceId: string | null | undefined, authFetch: AuthFetch) {
  const [tree, setTree] = useState<ApiTreeNode[]>([])
  const [environments, setEnvironments] = useState<ApiEnvironment[]>([])
  const [activeEnv, setActiveEnv] = useState('')
  const [req, setReq] = useState<ApiRequest>(blankRequest())
  const [savePath, setSavePath] = useState<string | null>(null) // path of the loaded request (no .json)

  const base = workspaceId ? `/api/workspaces/${encodeURIComponent(workspaceId)}/apiclient` : null

  const load = useCallback(async () => {
    if (!base) return
    try {
      const r = await authFetch(base)
      const d: ApiClientLoadResponse = await r.json()
      setTree(d.tree || [])
      setEnvironments(d.environments || [])
    } catch { /* keep prior */ }
  }, [base, authFetch])

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => { void load() }, [load])
  /* eslint-enable react-hooks/set-state-in-effect */

  const vars: ApiVars = (environments.find((e) => e.name === activeEnv)?.variables) || {}

  const openRequest = useCallback((node: ApiTreeNode) => {
    setReq({ ...blankRequest(), ...parseReq(node.request) })
    setSavePath(node.path.replace(/\.json$/, ''))
  }, [])

  const newRequest = useCallback(() => {
    setReq(blankRequest())
    setSavePath(null)
  }, [])

  // saveRequest persists the request to .wede/requests/, deriving the filename
  // from the name. It's also a true rename: if the name changes, the file is
  // moved to the new slug (keeping any folder), so the tree and file stay in sync.
  // saveRequest/newFolder/deleteItem are invoked as fire-and-forget callback
  // props (onClick={api.newFolder} etc. — see ApiCollections/ApiClient), so
  // nothing above them can catch a rejection. Unlike every other network call
  // in this codebase they previously had no try/catch of their own: a 401
  // (authFetch throws after logging out) or a network error while saving a
  // request, creating a folder, or deleting an item would reject silently —
  // the button just sits there with no feedback and, for a save, the edit is
  // lost with no indication it never landed.
  const saveRequest = useCallback(async () => {
    if (!base) return false
    const name = (req.name || '').trim() || 'Untitled Request'
    const slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-+|-+$)/g, '') || 'untitled'
    const dir = savePath && savePath.includes('/') ? savePath.slice(0, savePath.lastIndexOf('/') + 1) : ''
    const newPath = dir + slug
    try {
      await authFetch(`${base}/item`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: 'request', path: newPath, request: { ...req, name } }),
      })
      if (savePath && savePath !== newPath) {
        await authFetch(`${base}/item?path=${encodeURIComponent(savePath)}&type=request`, { method: 'DELETE' })
      }
    } catch {
      alert('Failed to save request — check your connection and try again.')
      return false
    }
    setSavePath(newPath)
    void load()
    return true
  }, [base, authFetch, req, savePath, load])

  const newFolder = useCallback(async () => {
    if (!base) return
    const name = prompt('New folder/collection name:')
    if (!name) return
    try {
      await authFetch(`${base}/item`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ type: 'folder', path: name }),
      })
    } catch {
      alert('Failed to create folder — check your connection and try again.')
      return
    }
    void load()
  }, [base, authFetch, load])

  const deleteItem = useCallback(async (node: ApiTreeNode) => {
    if (!base) return
    if (!confirm(`Delete ${node.name}?`)) return
    const rel = node.path.replace(/\.json$/, '')
    try {
      await authFetch(`${base}/item?path=${encodeURIComponent(rel)}&type=${node.type}`, { method: 'DELETE' })
    } catch {
      alert('Failed to delete — check your connection and try again.')
      return
    }
    if (savePath && rel === savePath) setSavePath(null)
    void load()
  }, [base, authFetch, savePath, load])

  return {
    base, tree, environments, activeEnv, setActiveEnv, vars,
    req, setReq, savePath, load, openRequest, newRequest, saveRequest, newFolder, deleteItem,
  }
}
