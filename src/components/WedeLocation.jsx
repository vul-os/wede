import { useState, useEffect, useCallback } from 'react'
import { FolderCog, Check, Loader2 } from 'lucide-react'

// WedeLocation — owner-only control to choose which workspace folder hosts the
// .wede directory (chat history, saved API requests). Changing it moves .wede.
export default function WedeLocation({ workspaceId, authFetch }) {
  const [state, setState] = useState(null) // { host, dir, folders }
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState(null)
  const [saved, setSaved] = useState(false)

  const base = workspaceId ? `/api/workspaces/${encodeURIComponent(workspaceId)}/wede-location` : null

  const load = useCallback(async () => {
    if (!base) return
    try {
      const r = await authFetch(base)
      setState(await r.json())
    } catch { /* leave hidden */ }
  }, [base, authFetch])

  useEffect(() => { load() }, [load])

  const setHost = async (host) => {
    if (!base) return
    setBusy(true); setErr(null); setSaved(false)
    try {
      const r = await authFetch(base, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ host }),
      })
      const data = await r.json()
      if (!r.ok) setErr(data.error || 'Failed to move')
      else { setState(data); setSaved(true); setTimeout(() => setSaved(false), 1500) }
    } catch { setErr('Request failed') } finally { setBusy(false) }
  }

  if (!state) return null

  return (
    <div>
      <h3 className="text-xs font-semibold uppercase tracking-wider text-text-muted mb-3 flex items-center gap-1.5">
        <FolderCog className="w-3.5 h-3.5" /> Workspace data (.wede)
      </h3>
      <p className="text-[11px] text-text-muted leading-relaxed mb-2">
        Chat history and saved API requests live in <code className="text-text-secondary">.wede/</code>.
        Pick which project folder hosts it so it commits with that repo — wede moves the contents when you change this.
      </p>
      <div className="flex items-center gap-2">
        <select
          value={state.host || ''}
          onChange={(e) => setHost(e.target.value)}
          disabled={busy}
          className="flex-1 bg-bg-input border border-border rounded-md px-2.5 py-1.5 text-[12px] text-text-primary focus:outline-none focus:border-accent/60 disabled:opacity-50"
        >
          <option value="">/ (workspace root)</option>
          {(state.folders || []).map((f) => <option key={f} value={f}>{f}/</option>)}
        </select>
        {busy && <Loader2 className="w-4 h-4 text-text-muted animate-spin shrink-0" />}
        {saved && <Check className="w-4 h-4 text-green shrink-0" />}
      </div>
      <p className="text-[10px] text-text-muted mt-1.5 font-mono truncate" title={state.dir}>{state.dir}</p>
      {err && <div className="text-[11px] text-red mt-1">{err}</div>}
    </div>
  )
}
