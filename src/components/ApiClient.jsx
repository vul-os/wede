// ApiClient — the request editor + response viewer for the built-in API client.
// Collections live in the sidebar (ApiCollections); shared state comes from the
// useApiClient hook passed in as `api`. Rendered as a full-width editor tab.

import { useState } from 'react'
import { Send, Plus, Trash2, Save, Loader2, Check, Copy } from 'lucide-react'
import { buildSend } from '../lib/apiRequest'

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
const METHOD_COLOR = {
  GET: 'text-green', POST: 'text-yellow', PUT: 'text-cyan', PATCH: 'text-mauve',
  DELETE: 'text-red', HEAD: 'text-text-muted', OPTIONS: 'text-text-muted',
}

// JsonHighlight — token-colours a pretty-printed JSON string (no dangerouslySetHTML).
function JsonHighlight({ text }) {
  const out = []
  const re = /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g
  let last = 0, m, i = 0
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) out.push(text.slice(last, m.index))
    if (m[1] && m[2]) { // "key":
      out.push(<span key={i++} className="text-accent">{m[1]}</span>)
      out.push(<span key={i++} className="text-text-muted">{m[2]}</span>)
    } else if (m[1]) {  // "string"
      out.push(<span key={i++} className="text-green">{m[1]}</span>)
    } else if (m[3]) {  // true/false/null
      out.push(<span key={i++} className="text-mauve">{m[3]}</span>)
    } else if (m[4]) {  // number
      out.push(<span key={i++} className="text-peach">{m[4]}</span>)
    }
    last = re.lastIndex
  }
  if (last < text.length) out.push(text.slice(last))
  return <pre className="text-[11.5px] font-mono leading-relaxed whitespace-pre-wrap break-words text-text-primary">{out}</pre>
}

// Key/value rows editor (params, headers, form fields) — with column headers.
function KVEditor({ rows, onChange, readOnly }) {
  const set = (i, patch) => onChange(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const add = () => onChange([...rows, { key: '', value: '', enabled: true }])
  const del = (i) => onChange(rows.filter((_, j) => j !== i))
  const cell = 'flex-1 bg-transparent px-2 py-1.5 text-[11.5px] font-mono text-text-primary placeholder:text-text-muted/60 focus:outline-none focus:bg-bg-input rounded min-w-0'
  return (
    <div className="border border-border rounded-lg overflow-hidden">
      <div className="flex items-center gap-1.5 px-2 py-1 bg-bg-tertiary/60 border-b border-border text-[9px] font-bold uppercase tracking-wider text-text-muted">
        <span className="w-4 shrink-0" />
        <span className="flex-1 px-1">Key</span>
        <span className="flex-1 px-1">Value</span>
        <span className="w-6 shrink-0" />
      </div>
      {rows.length === 0 && (
        <div className="px-3 py-2 text-[11px] text-text-muted">{readOnly ? 'None' : 'No entries — add one below.'}</div>
      )}
      {rows.map((r, i) => (
        <div key={i} className="flex items-center gap-1.5 px-2 border-b border-border/40 last:border-0 hover:bg-bg-hover/40 group">
          <input type="checkbox" checked={r.enabled !== false} disabled={readOnly}
            onChange={(e) => set(i, { enabled: e.target.checked })} className="w-4 shrink-0 accent-accent" />
          <input value={r.key} disabled={readOnly} placeholder="key" onChange={(e) => set(i, { key: e.target.value })} className={cell} />
          <input value={r.value} disabled={readOnly} placeholder="value" onChange={(e) => set(i, { value: e.target.value })} className={cell} />
          {!readOnly
            ? <button onClick={() => del(i)} className="w-6 h-6 flex items-center justify-center text-text-muted/0 group-hover:text-text-muted hover:!text-red shrink-0"><Trash2 className="w-3.5 h-3.5" /></button>
            : <span className="w-6 shrink-0" />}
        </div>
      ))}
      {!readOnly && (
        <button onClick={add} className="flex items-center gap-1 text-[11px] text-accent hover:text-accent-hover px-2 py-1.5 w-full">
          <Plus className="w-3 h-3" /> Add
        </button>
      )}
    </div>
  )
}

const inp = 'w-full bg-bg-input border border-border rounded-md px-2.5 py-1.5 text-[11.5px] font-mono text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60'

export default function ApiClient({ api, authFetch, readOnly = false }) {
  const { req, setReq, vars, base, saveRequest } = api
  const [reqTab, setReqTab] = useState('params')
  const [resp, setResp] = useState(null)
  const [respTab, setRespTab] = useState('body')
  const [sending, setSending] = useState(false)
  const [err, setErr] = useState(null)
  const [saved, setSaved] = useState(false)
  const [copied, setCopied] = useState(false)

  const doSave = async () => {
    if (await saveRequest()) { setSaved(true); setTimeout(() => setSaved(false), 1500) }
  }

  const patch = (p) => setReq((r) => ({ ...r, ...p }))
  const setBody = (p) => setReq((r) => ({ ...r, body: { ...r.body, ...p } }))
  const setAuth = (p) => setReq((r) => ({ ...r, auth: { ...r.auth, ...p } }))

  const send = async () => {
    if (!base || !req.url.trim()) return
    setSending(true); setErr(null); setResp(null)
    try {
      const res = await authFetch(`${base}/send`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(buildSend(req, vars)),
      })
      const data = await res.json()
      if (data.error) setErr(data.error)
      setResp(data)
      setRespTab('body')
    } catch { setErr('Request failed') } finally { setSending(false) }
  }

  const ok = resp?.status >= 200 && resp?.status < 300
  const statusPill = ok ? 'bg-green/12 text-green' : resp?.status >= 400 ? 'bg-red/12 text-red' : 'bg-yellow/12 text-yellow'

  const { pretty, isJson } = (() => {
    if (!resp?.body) return { pretty: '', isJson: false }
    try { return { pretty: JSON.stringify(JSON.parse(resp.body), null, 2), isJson: true } }
    catch { return { pretty: resp.body, isJson: false } }
  })()

  const copyBody = () => { navigator.clipboard?.writeText(pretty); setCopied(true); setTimeout(() => setCopied(false), 1500) }

  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden text-text-primary min-w-0">
      {/* Request name — click to rename */}
      <div className="px-3 pt-2.5 pb-1 shrink-0 flex items-center gap-2">
        <input value={req.name} disabled={readOnly} onChange={(e) => patch({ name: e.target.value })}
          placeholder="Untitled request — click to name it" title="Click to rename"
          className="flex-1 min-w-0 text-[14px] font-semibold text-text-primary placeholder:text-text-muted placeholder:font-normal rounded px-1.5 py-0.5 -ml-1.5 border border-transparent hover:border-border focus:border-accent/60 focus:bg-bg-input focus:outline-none transition-colors" />
        {api.savePath && <span className="text-[10px] text-text-muted font-mono shrink-0">{api.savePath}.json</span>}
      </div>

      {/* Postman-style URL bar: method + url unified, prominent Send */}
      <div className="flex items-center gap-2 px-3 pb-2.5 pt-1 shrink-0">
        <div className="flex-1 flex items-center bg-bg-input border border-border rounded-lg overflow-hidden focus-within:border-accent/60 transition-colors min-w-0">
          <select value={req.method} disabled={readOnly} onChange={(e) => patch({ method: e.target.value })}
            className={`bg-transparent pl-3 pr-2 py-2 text-[12px] font-bold focus:outline-none border-r border-border cursor-pointer ${METHOD_COLOR[req.method] || ''}`}>
            {METHODS.map((m) => <option key={m} value={m} className="text-text-primary bg-bg-primary">{m}</option>)}
          </select>
          <input value={req.url} disabled={readOnly} onChange={(e) => patch({ url: e.target.value })}
            onKeyDown={(e) => e.key === 'Enter' && send()}
            placeholder="https://api.example.com/path   ·   {{base}}/tasks"
            className="flex-1 bg-transparent px-3 py-2 text-[12px] font-mono text-text-primary placeholder:text-text-muted focus:outline-none min-w-0" />
        </div>
        <button onClick={send} disabled={sending || !req.url.trim()}
          className="flex items-center gap-1.5 px-5 py-2 bg-accent text-white rounded-lg text-[12px] font-semibold hover:bg-accent-hover disabled:opacity-40 shrink-0 shadow-sm shadow-accent/20 transition-colors">
          {sending ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Send className="w-3.5 h-3.5" />} Send
        </button>
        {!readOnly && (
          <button onClick={doSave} title="Save to .wede/requests/"
            className="flex items-center gap-1.5 px-2.5 py-2 text-[12px] text-text-muted hover:text-text-primary border border-border rounded-lg hover:bg-bg-hover shrink-0 transition-colors">
            {saved ? <Check className="w-4 h-4 text-green" /> : <Save className="w-4 h-4" />}
            <span className="hidden sm:inline">{saved ? 'Saved' : 'Save'}</span>
          </button>
        )}
      </div>

      {/* Request config tabs */}
      <div className="flex items-center gap-4 px-3 border-b border-border text-[11.5px] shrink-0">
        {['params', 'headers', 'auth', 'body'].map((t) => {
          const count = t === 'params' ? req.params.filter((p) => p.key).length
            : t === 'headers' ? req.headers.filter((h) => h.key).length : 0
          return (
            <button key={t} onClick={() => setReqTab(t)}
              className={`flex items-center gap-1 py-2.5 capitalize border-b-2 -mb-px transition-colors ${reqTab === t ? 'border-accent text-text-primary font-medium' : 'border-transparent text-text-muted hover:text-text-secondary'}`}>
              {t}{count > 0 && <span className="text-[9px] font-bold px-1 py-px rounded-full bg-accent/15 text-accent">{count}</span>}
            </button>
          )
        })}
      </div>
      <div className="px-3 py-3 border-b border-border max-h-52 overflow-y-auto shrink-0">
        {reqTab === 'params' && <KVEditor rows={req.params} onChange={(params) => patch({ params })} readOnly={readOnly} />}
        {reqTab === 'headers' && <KVEditor rows={req.headers} onChange={(headers) => patch({ headers })} readOnly={readOnly} />}
        {reqTab === 'auth' && (
          <div className="space-y-2 max-w-lg">
            <select value={req.auth.type} disabled={readOnly} onChange={(e) => setAuth({ type: e.target.value })}
              className="bg-bg-input border border-border rounded-md px-2.5 py-1.5 text-[11.5px] focus:outline-none focus:border-accent/60">
              <option value="none">No Auth</option><option value="bearer">Bearer Token</option>
              <option value="basic">Basic Auth</option><option value="apikey">API Key (header)</option>
            </select>
            {req.auth.type === 'bearer' && <input value={req.auth.token || ''} disabled={readOnly} onChange={(e) => setAuth({ token: e.target.value })} placeholder="token   ({{var}} ok)" className={inp} />}
            {req.auth.type === 'basic' && (
              <div className="flex gap-2">
                <input value={req.auth.username || ''} disabled={readOnly} onChange={(e) => setAuth({ username: e.target.value })} placeholder="username" className={inp} />
                <input value={req.auth.password || ''} disabled={readOnly} onChange={(e) => setAuth({ password: e.target.value })} placeholder="password" className={inp} />
              </div>
            )}
            {req.auth.type === 'apikey' && (
              <div className="flex gap-2">
                <input value={req.auth.key || ''} disabled={readOnly} onChange={(e) => setAuth({ key: e.target.value })} placeholder="header name" className={inp} />
                <input value={req.auth.value || ''} disabled={readOnly} onChange={(e) => setAuth({ value: e.target.value })} placeholder="value" className={inp} />
              </div>
            )}
          </div>
        )}
        {reqTab === 'body' && (
          <div className="space-y-2.5">
            <div className="inline-flex rounded-md border border-border overflow-hidden">
              {['none', 'json', 'form', 'raw'].map((t) => (
                <button key={t} onClick={() => !readOnly && setBody({ type: t })}
                  className={`px-2.5 py-1 text-[10px] font-semibold uppercase transition-colors ${req.body.type === t ? 'bg-accent text-white' : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'}`}>
                  {t}
                </button>
              ))}
            </div>
            {(req.body.type === 'json' || req.body.type === 'raw') && (
              <textarea value={req.body.content} disabled={readOnly} onChange={(e) => setBody({ content: e.target.value })}
                placeholder={req.body.type === 'json' ? '{\n  "key": "value"\n}' : 'raw body'} rows={6} spellCheck={false}
                className="w-full bg-bg-input border border-border rounded-lg px-3 py-2 text-[11.5px] font-mono text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 resize-y leading-relaxed" />
            )}
            {req.body.type === 'form' && <KVEditor rows={req.body.form} onChange={(form) => setBody({ form })} readOnly={readOnly} />}
          </div>
        )}
      </div>

      {/* Response */}
      <div className="flex-1 flex flex-col min-h-0">
        <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border text-[11px] shrink-0">
          <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Response</span>
          {resp && (
            <>
              <span className={`px-1.5 py-0.5 rounded font-bold ${statusPill}`}>{resp.status || '—'} {(resp.statusText || '').replace(/^\d+\s*/, '')}</span>
              <span className="text-text-muted">⏱ {resp.timeMs} ms</span>
              {resp.size != null && <span className="text-text-muted">⬇ {resp.size < 1024 ? `${resp.size} B` : `${(resp.size / 1024).toFixed(1)} KB`}</span>}
              <div className="flex-1" />
              <button onClick={copyBody} title="Copy body" className="flex items-center gap-1 px-1.5 py-0.5 rounded text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors">
                {copied ? <Check className="w-3 h-3 text-green" /> : <Copy className="w-3 h-3" />}
              </button>
              {['body', 'headers'].map((t) => (
                <button key={t} onClick={() => setRespTab(t)} className={`capitalize px-1 ${respTab === t ? 'text-text-primary font-semibold' : 'text-text-muted hover:text-text-secondary'}`}>
                  {t}{t === 'headers' && resp.headers ? ` (${Object.keys(resp.headers).length})` : ''}
                </button>
              ))}
            </>
          )}
        </div>
        <div className="flex-1 overflow-auto">
          {err && <div className="m-3 px-3 py-2 text-[12px] text-red bg-red/8 border border-red/20 rounded-lg">⚠ {err}</div>}
          {!resp && !err && (
            <div className="h-full flex flex-col items-center justify-center text-center px-6 select-none">
              <div className="w-12 h-12 rounded-2xl bg-bg-hover flex items-center justify-center mb-3">
                <Send className="w-5 h-5 text-text-muted opacity-50" />
              </div>
              <p className="text-[12px] text-text-secondary font-medium">Enter a URL and hit Send</p>
              <p className="text-[11px] text-text-muted mt-1">The response shows here, JSON pretty-printed</p>
            </div>
          )}
          {resp && respTab === 'body' && (
            <div className="p-3">
              {isJson ? <JsonHighlight text={pretty} /> : <pre className="text-[11.5px] font-mono text-text-primary whitespace-pre-wrap break-words leading-relaxed">{pretty}</pre>}
            </div>
          )}
          {resp && respTab === 'headers' && (
            <div className="p-3 space-y-1">
              {Object.entries(resp.headers || {}).map(([k, v]) => (
                <div key={k} className="text-[11.5px] font-mono flex gap-2"><span className="text-accent shrink-0">{k}</span><span className="text-text-secondary break-all">{v}</span></div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
