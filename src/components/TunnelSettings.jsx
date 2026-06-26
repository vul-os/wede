import { useState, useEffect, useCallback, useRef } from 'react'
import { Globe, Copy, Check, Play, Square, AlertTriangle, ExternalLink } from 'lucide-react'

// TunnelSettings — owner-only UI to expose wede publicly via an frp tunnel.
// wede detects frpc, the owner supplies their frps relay details, and wede runs
// the tunnel + shows the live public URL.
export default function TunnelSettings({ authFetch }) {
  const [state, setState] = useState(null)
  const [form, setForm] = useState({
    serverAddr: '', serverPort: 7000, token: '', mode: 'http', domain: '', remotePort: 0, https: false,
  })
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState(null)
  const [copied, setCopied] = useState(false)
  const pollRef = useRef(null)

  const load = useCallback(async () => {
    try {
      const res = await authFetch('/api/tunnel')
      const data = await res.json()
      setState(data)
      if (data.config) {
        // adopt stored config (token is redacted server-side; keep any typed token)
        setForm((f) => ({
          serverAddr: data.config.serverAddr || '',
          serverPort: data.config.serverPort || 7000,
          token: f.token,
          mode: data.config.mode || 'http',
          domain: data.config.domain || '',
          remotePort: data.config.remotePort || 0,
          https: !!data.config.https,
        }))
      }
    } catch { /* ignore — tunnel UI degrades to hidden */ }
  }, [authFetch])

   
  useEffect(() => { load() }, [load])

  useEffect(() => {
    const s = state?.status
    if (s === 'starting' || s === 'connected') {
      pollRef.current = setInterval(load, 3000)
      return () => clearInterval(pollRef.current)
    }
    return undefined
  }, [state?.status, load])
   

  const start = async () => {
    setBusy(true); setErr(null)
    try {
      const r1 = await authFetch('/api/tunnel/config', {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...form, serverPort: Number(form.serverPort), remotePort: Number(form.remotePort) }),
      })
      if (!r1.ok) { setErr((await r1.json()).error || 'Invalid config'); setBusy(false); return }
      const r2 = await authFetch('/api/tunnel/start', { method: 'POST' })
      const data = await r2.json()
      if (!r2.ok) setErr(data.error || 'Failed to start')
      else setState(data)
    } catch { setErr('Request failed') } finally { setBusy(false) }
  }

  const stop = async () => {
    setBusy(true); setErr(null)
    try { const r = await authFetch('/api/tunnel/stop', { method: 'POST' }); setState(await r.json()) }
    catch { /* ignore */ } finally { setBusy(false) }
  }

  const copyUrl = () => {
    if (!state?.publicUrl) return
    navigator.clipboard?.writeText(state.publicUrl)
    setCopied(true); setTimeout(() => setCopied(false), 1500)
  }

  if (!state) return null

  const running = state.status === 'connected' || state.status === 'starting'
  const statusColor = {
    connected: 'text-green', starting: 'text-yellow', error: 'text-red', stopped: 'text-text-muted',
  }[state.status] || 'text-text-muted'

  const field = 'w-full bg-bg-input border border-border rounded-md px-2.5 py-1.5 text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60'

  return (
    <div>
      <h3 className="text-xs font-semibold uppercase tracking-wider text-text-muted mb-3 flex items-center gap-1.5">
        <Globe className="w-3.5 h-3.5" /> Public access (frp tunnel)
      </h3>

      {!state.detected ? (
        <div className="flex items-start gap-2 px-3 py-2.5 rounded-lg bg-bg-primary border border-border">
          <AlertTriangle className="w-3.5 h-3.5 text-yellow shrink-0 mt-px" />
          <span className="text-[11px] text-text-muted leading-relaxed">
            <code className="text-text-secondary">frpc</code> not found on PATH. Install{' '}
            <a href="https://github.com/fatedier/frp" target="_blank" rel="noreferrer" className="text-accent underline">frp</a>{' '}
            to expose wede over the internet via your own relay (VPS). See docs/GETTING-STARTED.
          </span>
        </div>
      ) : (
        <div className="space-y-2.5">
          {/* Status + public URL */}
          <div className="flex items-center justify-between px-3 py-2 rounded-lg bg-bg-primary border border-border">
            <span className="text-[11px] text-text-secondary">
              Status: <span className={`font-semibold ${statusColor}`}>{state.status}</span>
            </span>
            {running && state.publicUrl && (
              <div className="flex items-center gap-1.5 min-w-0">
                <a href={state.publicUrl} target="_blank" rel="noreferrer" className="text-[11px] text-accent font-mono truncate flex items-center gap-1">
                  {state.publicUrl} <ExternalLink className="w-3 h-3 shrink-0" />
                </a>
                <button onClick={copyUrl} title="Copy" className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-bg-hover">
                  {copied ? <Check className="w-3.5 h-3.5 text-green" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
              </div>
            )}
          </div>

          {/* Relay config */}
          <div className="grid grid-cols-2 gap-2">
            <input className={field} placeholder="frps server address (VPS IP/host)" value={form.serverAddr}
              onChange={(e) => setForm({ ...form, serverAddr: e.target.value })} />
            <input className={field} type="number" placeholder="server port (7000)" value={form.serverPort}
              onChange={(e) => setForm({ ...form, serverPort: e.target.value })} />
          </div>
          <input className={field} type="password" placeholder="auth token (shared with frps)" value={form.token}
            onChange={(e) => setForm({ ...form, token: e.target.value })} />
          <div className="flex items-center gap-2">
            <select className={`${field} w-auto`} value={form.mode} onChange={(e) => setForm({ ...form, mode: e.target.value })}>
              <option value="http">http (domain)</option>
              <option value="tcp">tcp (port)</option>
            </select>
            {form.mode === 'http' ? (
              <input className={field} placeholder="domain (wede.example.com)" value={form.domain}
                onChange={(e) => setForm({ ...form, domain: e.target.value })} />
            ) : (
              <input className={field} type="number" placeholder="remote port (9090)" value={form.remotePort}
                onChange={(e) => setForm({ ...form, remotePort: e.target.value })} />
            )}
            <label className="flex items-center gap-1 text-[11px] text-text-muted shrink-0">
              <input type="checkbox" checked={form.https} onChange={(e) => setForm({ ...form, https: e.target.checked })} /> https
            </label>
          </div>

          {err && <div className="text-[11px] text-red px-0.5">{err}</div>}

          <div className="flex items-center gap-2">
            {running ? (
              <button onClick={stop} disabled={busy}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-[12px] bg-red/10 text-red hover:bg-red/20 transition-colors disabled:opacity-50">
                <Square className="w-3.5 h-3.5" /> Stop tunnel
              </button>
            ) : (
              <button onClick={start} disabled={busy}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-[12px] bg-accent/10 text-accent hover:bg-accent/20 transition-colors disabled:opacity-50">
                <Play className="w-3.5 h-3.5" /> {busy ? 'Starting…' : 'Start tunnel'}
              </button>
            )}
          </div>

          {/* Log tail */}
          {state.log && state.log.length > 0 && (
            <pre className="max-h-28 overflow-auto bg-bg-primary border border-border rounded-md p-2 text-[10px] leading-relaxed text-text-muted font-mono whitespace-pre-wrap">
              {state.log.slice(-12).join('\n')}
            </pre>
          )}
        </div>
      )}
    </div>
  )
}
