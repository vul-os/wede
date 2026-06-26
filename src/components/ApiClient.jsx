// ApiClient — wede's built-in Postman-style HTTP client.
//
// Requests + folders live under <wede>/requests/ (one JSON file per request),
// environments under <wede>/environments/. The send proxy runs server-side, so
// there are no browser CORS limits. {{variable}} substitution is applied here
// from the active environment before sending.
//
// Props: workspaceId, authFetch, readOnly (viewers can browse + send but not save).

import { useState, useEffect, useCallback } from 'react'
import {
  Send, Plus, Trash2, FolderPlus, ChevronRight, ChevronDown, Save, Globe2, Loader2,
} from 'lucide-react'

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
const METHOD_COLOR = {
  GET: 'text-green', POST: 'text-yellow', PUT: 'text-accent', PATCH: 'text-accent',
  DELETE: 'text-red', HEAD: 'text-text-muted', OPTIONS: 'text-text-muted',
}

const blankRequest = () => ({
  name: 'New Request', method: 'GET', url: '',
  params: [], headers: [], auth: { type: 'none' },
  body: { type: 'none', content: '', form: [] },
})

// The server returns a saved request as a JSON object (RawMessage); tolerate a
// string too in case of older/hand-written files.
function parseReq(raw) {
  if (!raw) return {}
  if (typeof raw === 'object') return raw
  try { return JSON.parse(raw) } catch { return {} }
}

function subst(str, vars) {
  return (str || '').replace(/\{\{([^}]+)\}\}/g, (_, k) => {
    const key = k.trim()
    return key in vars ? vars[key] : `{{${key}}}`
  })
}

// Resolve a saved request + active env into the wire payload for /send.
function buildSend(req, vars) {
  let url = subst(req.url, vars)
  const qp = (req.params || []).filter((p) => p.enabled && p.key)
    .map((p) => `${encodeURIComponent(subst(p.key, vars))}=${encodeURIComponent(subst(p.value, vars))}`)
  if (qp.length) url += (url.includes('?') ? '&' : '?') + qp.join('&')

  const headers = {}
  ;(req.headers || []).filter((h) => h.enabled && h.key).forEach((h) => {
    headers[subst(h.key, vars)] = subst(h.value, vars)
  })
  const a = req.auth || {}
  if (a.type === 'bearer' && a.token) headers['Authorization'] = 'Bearer ' + subst(a.token, vars)
  else if (a.type === 'basic') headers['Authorization'] = 'Basic ' + btoa(`${subst(a.username || '', vars)}:${subst(a.password || '', vars)}`)
  else if (a.type === 'apikey' && a.key) headers[subst(a.key, vars)] = subst(a.value || '', vars)

  let body = ''
  const b = req.body || {}
  if (b.type === 'json' || b.type === 'raw') {
    body = subst(b.content || '', vars)
    if (b.type === 'json' && !headers['Content-Type']) headers['Content-Type'] = 'application/json'
  } else if (b.type === 'form') {
    body = (b.form || []).filter((f) => f.enabled && f.key)
      .map((f) => `${encodeURIComponent(subst(f.key, vars))}=${encodeURIComponent(subst(f.value, vars))}`).join('&')
    if (!headers['Content-Type']) headers['Content-Type'] = 'application/x-www-form-urlencoded'
  }
  return { method: req.method, url, headers, body }
}

// ── Key/value rows editor (params, headers, form fields) ──────────────────────
function KVEditor({ rows, onChange, readOnly, placeholderKey = 'key', placeholderVal = 'value' }) {
  const set = (i, patch) => onChange(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const add = () => onChange([...rows, { key: '', value: '', enabled: true }])
  const del = (i) => onChange(rows.filter((_, j) => j !== i))
  return (
    <div className="space-y-1">
      {rows.map((r, i) => (
        <div key={i} className="flex items-center gap-1.5">
          <input type="checkbox" checked={r.enabled !== false} disabled={readOnly}
            onChange={(e) => set(i, { enabled: e.target.checked })} className="shrink-0" />
          <input value={r.key} disabled={readOnly} placeholder={placeholderKey}
            onChange={(e) => set(i, { key: e.target.value })}
            className="flex-1 bg-bg-input border border-border rounded px-2 py-1 text-[11px] font-mono text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60" />
          <input value={r.value} disabled={readOnly} placeholder={placeholderVal}
            onChange={(e) => set(i, { value: e.target.value })}
            className="flex-1 bg-bg-input border border-border rounded px-2 py-1 text-[11px] font-mono text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60" />
          {!readOnly && (
            <button onClick={() => del(i)} className="p-1 text-text-muted hover:text-red shrink-0"><Trash2 className="w-3.5 h-3.5" /></button>
          )}
        </div>
      ))}
      {!readOnly && (
        <button onClick={add} className="flex items-center gap-1 text-[11px] text-accent hover:text-accent-hover mt-1">
          <Plus className="w-3 h-3" /> Add
        </button>
      )}
    </div>
  )
}

// ── Collections tree ──────────────────────────────────────────────────────────
function TreeNode({ node, depth, onOpen, onDelete, readOnly, activePath }) {
  const [open, setOpen] = useState(true)
  if (node.type === 'folder') {
    return (
      <div>
        <div className="flex items-center gap-1 px-1 py-1 hover:bg-bg-hover rounded cursor-pointer group"
          style={{ paddingLeft: depth * 12 + 4 }} onClick={() => setOpen(!open)}>
          {open ? <ChevronDown className="w-3 h-3 text-text-muted" /> : <ChevronRight className="w-3 h-3 text-text-muted" />}
          <span className="text-[12px] text-text-secondary truncate flex-1">{node.name}</span>
          {!readOnly && <button onClick={(e) => { e.stopPropagation(); onDelete(node) }} className="opacity-0 group-hover:opacity-100 text-text-muted hover:text-red"><Trash2 className="w-3 h-3" /></button>}
        </div>
        {open && node.children?.map((c) => (
          <TreeNode key={c.path} node={c} depth={depth + 1} onOpen={onOpen} onDelete={onDelete} readOnly={readOnly} activePath={activePath} />
        ))}
      </div>
    )
  }
  const m = parseReq(node.request).method || 'GET'
  return (
    <div className={`flex items-center gap-1.5 px-1 py-1 rounded cursor-pointer group ${activePath === node.path ? 'bg-bg-active' : 'hover:bg-bg-hover'}`}
      style={{ paddingLeft: depth * 12 + 16 }} onClick={() => onOpen(node)}>
      <span className={`text-[9px] font-bold w-9 shrink-0 ${METHOD_COLOR[m] || 'text-text-muted'}`}>{m}</span>
      <span className="text-[12px] text-text-primary truncate flex-1">{node.name}</span>
      {!readOnly && <button onClick={(e) => { e.stopPropagation(); onDelete(node) }} className="opacity-0 group-hover:opacity-100 text-text-muted hover:text-red"><Trash2 className="w-3 h-3" /></button>}
    </div>
  )
}

export default function ApiClient({ workspaceId, authFetch, readOnly = false }) {
  const [tree, setTree] = useState([])
  const [environments, setEnvironments] = useState([])
  const [activeEnv, setActiveEnv] = useState('')
  const [req, setReq] = useState(blankRequest())
  const [savePath, setSavePath] = useState(null) // path of the loaded request (for save/overwrite)
  const [reqTab, setReqTab] = useState('params')
  const [resp, setResp] = useState(null)
  const [respTab, setRespTab] = useState('body')
  const [sending, setSending] = useState(false)
  const [err, setErr] = useState(null)

  const base = workspaceId ? `/api/workspaces/${encodeURIComponent(workspaceId)}/apiclient` : null

  const load = useCallback(async () => {
    if (!base) return
    try {
      const res = await authFetch(base)
      const data = await res.json()
      setTree(data.tree || [])
      setEnvironments(data.environments || [])
    } catch { /* keep prior */ }
  }, [base, authFetch])

  useEffect(() => { load() }, [load])

  const vars = (environments.find((e) => e.name === activeEnv)?.variables) || {}

  const send = async () => {
    if (!base || !req.url.trim()) return
    setSending(true); setErr(null); setResp(null)
    try {
      const payload = buildSend(req, vars)
      const res = await authFetch(`${base}/send`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
      })
      const data = await res.json()
      if (data.error) setErr(data.error)
      setResp(data)
      setRespTab('body')
    } catch { setErr('Request failed') } finally { setSending(false) }
  }

  const saveRequest = async () => {
    if (readOnly || !base) return
    let path = savePath
    if (!path) {
      const name = prompt('Save request as (e.g. tasks/list):', req.name.toLowerCase().replace(/\s+/g, '-'))
      if (!name) return
      path = name
      setSavePath(path)
    }
    await authFetch(`${base}/item`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type: 'request', path, request: { ...req, name: path.split('/').pop() } }),
    })
    load()
  }

  const newFolder = async () => {
    if (readOnly || !base) return
    const name = prompt('New folder/collection name:')
    if (!name) return
    await authFetch(`${base}/item`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ type: 'folder', path: name }),
    })
    load()
  }

  const openRequest = (node) => {
    setReq({ ...blankRequest(), ...parseReq(node.request) })
    setSavePath(node.path.replace(/\.json$/, ''))
    setResp(null); setErr(null)
  }

  const deleteItem = async (node) => {
    if (readOnly || !base) return
    if (!confirm(`Delete ${node.name}?`)) return
    await authFetch(`${base}/item?path=${encodeURIComponent(node.path.replace(/\.json$/, ''))}&type=${node.type}`, { method: 'DELETE' })
    if (savePath && node.path.replace(/\.json$/, '') === savePath) { setSavePath(null) }
    load()
  }

  const patch = (p) => setReq((r) => ({ ...r, ...p }))
  const setBody = (p) => setReq((r) => ({ ...r, body: { ...r.body, ...p } }))
  const setAuth = (p) => setReq((r) => ({ ...r, auth: { ...r.auth, ...p } }))

  const statusColor = resp?.status >= 200 && resp?.status < 300 ? 'text-green'
    : resp?.status >= 400 ? 'text-red' : 'text-yellow'

  const prettyBody = (() => {
    if (!resp?.body) return ''
    try { return JSON.stringify(JSON.parse(resp.body), null, 2) } catch { return resp.body }
  })()

  return (
    <div className="h-full flex bg-bg-secondary overflow-hidden text-text-primary">
      {/* Collections sidebar */}
      <div className="w-56 shrink-0 border-r border-border flex flex-col">
        <div className="flex items-center justify-between px-2.5 py-2 border-b border-border">
          <span className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">Collections</span>
          {!readOnly && (
            <div className="flex items-center gap-0.5">
              <button title="New request" onClick={() => { setReq(blankRequest()); setSavePath(null); setResp(null) }} className="p-1 text-text-muted hover:text-text-primary hover:bg-bg-hover rounded"><Plus className="w-3.5 h-3.5" /></button>
              <button title="New folder" onClick={newFolder} className="p-1 text-text-muted hover:text-text-primary hover:bg-bg-hover rounded"><FolderPlus className="w-3.5 h-3.5" /></button>
            </div>
          )}
        </div>
        <div className="flex-1 overflow-y-auto p-1">
          {tree.length === 0 ? (
            <p className="text-[11px] text-text-muted p-3 text-center">No saved requests yet. Build one and hit Save.</p>
          ) : tree.map((n) => (
            <TreeNode key={n.path} node={n} depth={0} onOpen={openRequest} onDelete={deleteItem} readOnly={readOnly} activePath={savePath ? savePath + '.json' : null} />
          ))}
        </div>
        {/* Environment selector */}
        <div className="border-t border-border px-2.5 py-2 flex items-center gap-1.5">
          <Globe2 className="w-3.5 h-3.5 text-text-muted shrink-0" />
          <select value={activeEnv} onChange={(e) => setActiveEnv(e.target.value)}
            className="flex-1 bg-bg-input border border-border rounded px-1.5 py-1 text-[11px] text-text-primary focus:outline-none focus:border-accent/60">
            <option value="">No environment</option>
            {environments.map((e) => <option key={e.name} value={e.name}>{e.name}</option>)}
          </select>
        </div>
      </div>

      {/* Request + response */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* URL bar */}
        <div className="flex items-center gap-1.5 px-2.5 py-2 border-b border-border shrink-0">
          <select value={req.method} disabled={readOnly} onChange={(e) => patch({ method: e.target.value })}
            className={`bg-bg-input border border-border rounded px-1.5 py-1.5 text-[11px] font-bold focus:outline-none focus:border-accent/60 ${METHOD_COLOR[req.method] || ''}`}>
            {METHODS.map((m) => <option key={m} value={m} className="text-text-primary">{m}</option>)}
          </select>
          <input value={req.url} disabled={readOnly} onChange={(e) => patch({ url: e.target.value })}
            onKeyDown={(e) => e.key === 'Enter' && send()}
            placeholder="https://api.example.com/path  or  {{base}}/tasks"
            className="flex-1 bg-bg-input border border-border rounded px-2.5 py-1.5 text-[12px] font-mono text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 min-w-0" />
          <button onClick={send} disabled={sending || !req.url.trim()}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-accent text-white rounded text-[12px] font-semibold hover:bg-accent-hover disabled:opacity-40 shrink-0">
            {sending ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Send className="w-3.5 h-3.5" />} Send
          </button>
          {!readOnly && (
            <button onClick={saveRequest} title="Save request" className="p-1.5 text-text-muted hover:text-text-primary hover:bg-bg-hover rounded shrink-0"><Save className="w-4 h-4" /></button>
          )}
        </div>

        {/* Request config tabs */}
        <div className="flex items-center gap-3 px-3 border-b border-border text-[11px] shrink-0">
          {['params', 'headers', 'auth', 'body'].map((t) => (
            <button key={t} onClick={() => setReqTab(t)}
              className={`py-2 capitalize border-b-2 -mb-px ${reqTab === t ? 'border-accent text-text-primary' : 'border-transparent text-text-muted hover:text-text-secondary'}`}>
              {t}{t === 'params' && req.params.length ? ` (${req.params.length})` : ''}{t === 'headers' && req.headers.length ? ` (${req.headers.length})` : ''}
            </button>
          ))}
        </div>
        <div className="px-3 py-2.5 border-b border-border max-h-44 overflow-y-auto shrink-0">
          {reqTab === 'params' && <KVEditor rows={req.params} onChange={(params) => patch({ params })} readOnly={readOnly} />}
          {reqTab === 'headers' && <KVEditor rows={req.headers} onChange={(headers) => patch({ headers })} readOnly={readOnly} />}
          {reqTab === 'auth' && (
            <div className="space-y-2">
              <select value={req.auth.type} disabled={readOnly} onChange={(e) => setAuth({ type: e.target.value })}
                className="bg-bg-input border border-border rounded px-2 py-1 text-[11px] focus:outline-none focus:border-accent/60">
                <option value="none">No Auth</option><option value="bearer">Bearer Token</option>
                <option value="basic">Basic Auth</option><option value="apikey">API Key (header)</option>
              </select>
              {req.auth.type === 'bearer' && <input value={req.auth.token || ''} disabled={readOnly} onChange={(e) => setAuth({ token: e.target.value })} placeholder="token  ({{var}} ok)" className="w-full bg-bg-input border border-border rounded px-2 py-1 text-[11px] font-mono focus:outline-none focus:border-accent/60" />}
              {req.auth.type === 'basic' && (
                <div className="flex gap-1.5">
                  <input value={req.auth.username || ''} disabled={readOnly} onChange={(e) => setAuth({ username: e.target.value })} placeholder="username" className="flex-1 bg-bg-input border border-border rounded px-2 py-1 text-[11px] font-mono focus:outline-none focus:border-accent/60" />
                  <input value={req.auth.password || ''} disabled={readOnly} onChange={(e) => setAuth({ password: e.target.value })} placeholder="password" className="flex-1 bg-bg-input border border-border rounded px-2 py-1 text-[11px] font-mono focus:outline-none focus:border-accent/60" />
                </div>
              )}
              {req.auth.type === 'apikey' && (
                <div className="flex gap-1.5">
                  <input value={req.auth.key || ''} disabled={readOnly} onChange={(e) => setAuth({ key: e.target.value })} placeholder="header name" className="flex-1 bg-bg-input border border-border rounded px-2 py-1 text-[11px] font-mono focus:outline-none focus:border-accent/60" />
                  <input value={req.auth.value || ''} disabled={readOnly} onChange={(e) => setAuth({ value: e.target.value })} placeholder="value" className="flex-1 bg-bg-input border border-border rounded px-2 py-1 text-[11px] font-mono focus:outline-none focus:border-accent/60" />
                </div>
              )}
            </div>
          )}
          {reqTab === 'body' && (
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-[11px]">
                {['none', 'json', 'form', 'raw'].map((t) => (
                  <label key={t} className="flex items-center gap-1 cursor-pointer">
                    <input type="radio" checked={req.body.type === t} disabled={readOnly} onChange={() => setBody({ type: t })} />
                    <span className="uppercase">{t}</span>
                  </label>
                ))}
              </div>
              {(req.body.type === 'json' || req.body.type === 'raw') && (
                <textarea value={req.body.content} disabled={readOnly} onChange={(e) => setBody({ content: e.target.value })}
                  placeholder={req.body.type === 'json' ? '{\n  "key": "value"\n}' : 'raw body'} rows={5}
                  className="w-full bg-bg-input border border-border rounded px-2 py-1.5 text-[11px] font-mono text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 resize-y" />
              )}
              {req.body.type === 'form' && <KVEditor rows={req.body.form} onChange={(form) => setBody({ form })} readOnly={readOnly} />}
            </div>
          )}
        </div>

        {/* Response */}
        <div className="flex-1 flex flex-col min-h-0">
          <div className="flex items-center gap-3 px-3 py-1.5 border-b border-border text-[11px] shrink-0">
            {resp ? (
              <>
                <span className={`font-bold ${statusColor}`}>{resp.status || '—'} {resp.statusText?.replace(/^\d+\s*/, '')}</span>
                <span className="text-text-muted">{resp.timeMs} ms</span>
                <span className="text-text-muted">{resp.size != null ? `${resp.size} B` : ''}</span>
                <div className="flex-1" />
                {['body', 'headers'].map((t) => (
                  <button key={t} onClick={() => setRespTab(t)} className={`capitalize ${respTab === t ? 'text-text-primary font-semibold' : 'text-text-muted'}`}>{t}</button>
                ))}
              </>
            ) : <span className="text-text-muted">Response</span>}
          </div>
          <div className="flex-1 overflow-auto p-3">
            {err && <div className="text-[12px] text-red mb-2">⚠ {err}</div>}
            {!resp && !err && <p className="text-[12px] text-text-muted">Send a request to see the response.</p>}
            {resp && respTab === 'body' && (
              <pre className="text-[11px] font-mono text-text-primary whitespace-pre-wrap break-words leading-relaxed">{prettyBody}</pre>
            )}
            {resp && respTab === 'headers' && (
              <div className="space-y-0.5">
                {Object.entries(resp.headers || {}).map(([k, v]) => (
                  <div key={k} className="text-[11px] font-mono"><span className="text-accent">{k}</span><span className="text-text-muted">: </span><span className="text-text-secondary break-all">{v}</span></div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
