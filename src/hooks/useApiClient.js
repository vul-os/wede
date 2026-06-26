// useApiClient — shared state for the built-in API client, so the collections
// (rendered in the IDE sidebar like the file explorer) and the request editor
// (rendered in the editor area) stay in sync. The IDE creates one instance and
// passes it to both.

import { useState, useEffect, useCallback } from 'react'
import { parseReq } from '../lib/apiRequest'

export const blankRequest = () => ({
  name: 'New Request', method: 'GET', url: '',
  params: [], headers: [], auth: { type: 'none' },
  body: { type: 'none', content: '', form: [] },
})

export function useApiClient(workspaceId, authFetch) {
  const [tree, setTree] = useState([])
  const [environments, setEnvironments] = useState([])
  const [activeEnv, setActiveEnv] = useState('')
  const [req, setReq] = useState(blankRequest())
  const [savePath, setSavePath] = useState(null) // path of the loaded request (no .json)

  const base = workspaceId ? `/api/workspaces/${encodeURIComponent(workspaceId)}/apiclient` : null

  const load = useCallback(async () => {
    if (!base) return
    try {
      const r = await authFetch(base)
      const d = await r.json()
      setTree(d.tree || [])
      setEnvironments(d.environments || [])
    } catch { /* keep prior */ }
  }, [base, authFetch])

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => { load() }, [load])
  /* eslint-enable react-hooks/set-state-in-effect */

  const vars = (environments.find((e) => e.name === activeEnv)?.variables) || {}

  const openRequest = useCallback((node) => {
    setReq({ ...blankRequest(), ...parseReq(node.request) })
    setSavePath(node.path.replace(/\.json$/, ''))
  }, [])

  const newRequest = useCallback(() => {
    setReq(blankRequest())
    setSavePath(null)
  }, [])

  const saveRequest = useCallback(async () => {
    if (!base) return
    const name = (req.name || '').trim() || 'Untitled Request'
    // New requests get a file path derived from the name (slugified, folders via
    // "/" preserved); already-saved requests keep their path so renaming the
    // display name doesn't move the file.
    let path = savePath
    if (!path) {
      path = name.toLowerCase().replace(/[^a-z0-9/]+/g, '-').replace(/(^-+|-+$)/g, '') || 'untitled'
      setSavePath(path)
    }
    await authFetch(`${base}/item`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type: 'request', path, request: { ...req, name } }),
    })
    load()
    return true
  }, [base, authFetch, req, savePath, load])

  const newFolder = useCallback(async () => {
    if (!base) return
    const name = prompt('New folder/collection name:')
    if (!name) return
    await authFetch(`${base}/item`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ type: 'folder', path: name }),
    })
    load()
  }, [base, authFetch, load])

  const deleteItem = useCallback(async (node) => {
    if (!base) return
    if (!confirm(`Delete ${node.name}?`)) return
    const rel = node.path.replace(/\.json$/, '')
    await authFetch(`${base}/item?path=${encodeURIComponent(rel)}&type=${node.type}`, { method: 'DELETE' })
    if (savePath && rel === savePath) setSavePath(null)
    load()
  }, [base, authFetch, savePath, load])

  return {
    base, tree, environments, activeEnv, setActiveEnv, vars,
    req, setReq, savePath, load, openRequest, newRequest, saveRequest, newFolder, deleteItem,
  }
}
