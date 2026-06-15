import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import {
  GitBranch, Plus, Minus, RefreshCw, Check,
  ChevronDown, ChevronRight, RotateCcw, Copy, GitCommit,
  GitMerge, Clock, User, Hash, Upload, Download, CloudDownload,
  AlertCircle, X
} from 'lucide-react'

/* ═══════════════════════════════════════════════════
   Commit right-click context menu
═══════════════════════════════════════════════════ */

function CommitMenu({ x, y, commit, onClose, onAction }) {
  const ref = useRef(null)

  useEffect(() => {
    const handler = (e) => { if (!ref.current?.contains(e.target)) onClose() }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [onClose])

  const clamped = {
    left: Math.min(x, window.innerWidth - 210),
    top: Math.min(y, window.innerHeight - 200),
  }

  const items = [
    { label: 'Checkout commit', icon: GitCommit, action: () => onAction('checkout', commit.hash) },
    { label: 'Copy full hash', icon: Hash, action: () => { navigator.clipboard.writeText(commit.hash); onClose() } },
    { label: 'Copy short hash', icon: Copy, action: () => { navigator.clipboard.writeText(commit.short); onClose() } },
    { label: 'Copy message', icon: Copy, action: () => { navigator.clipboard.writeText(commit.message); onClose() } },
  ]

  return (
    <div ref={ref} className="fixed z-50 animate-fade-in" style={clamped}>
      <div className="bg-bg-elevated border border-border rounded-lg shadow-xl shadow-shadow-lg py-1.5 min-w-[200px] overflow-hidden">
        {/* Commit preview */}
        <div className="px-3 pb-2 mb-1 border-b border-border">
          <div className="flex items-center gap-1.5 mb-0.5">
            <span className="font-mono text-[10px] text-accent bg-accent/10 px-1.5 py-0.5 rounded font-semibold">{commit.short}</span>
          </div>
          <p className="text-[11px] text-text-secondary truncate max-w-[180px]">{commit.message}</p>
        </div>
        {items.map((item, i) => (
          <button key={i} onClick={item.action}
            className="w-full flex items-center gap-2.5 px-3 py-1.5 text-[12px] text-text-secondary hover:bg-bg-hover hover:text-text-primary transition-colors text-left">
            <item.icon className="w-3.5 h-3.5 text-text-muted shrink-0" />
            {item.label}
          </button>
        ))}
      </div>
    </div>
  )
}

/* ═══════════════════════════════════════════════════
   Git graph — lane-based SVG visualization
═══════════════════════════════════════════════════ */

// Lane colours — a deliberate palette, not rainbow noise
const LANE_COLORS = [
  '#7c8cf8', // indigo (accent)
  '#4ade80', // green
  '#c084fc', // mauve
  '#fb923c', // peach
  '#22d3ee', // cyan
  '#fbbf24', // yellow
  '#f472b6', // pink
  '#f87171', // red
]

function buildGraph(entries) {
  if (!entries?.length) return []
  const lanes = []  // lane[i] = hash of commit expected next in that lane
  const rows = []

  for (const entry of entries) {
    // Find or assign a lane for this commit
    let lane = lanes.indexOf(entry.hash)
    if (lane === -1) {
      lane = lanes.indexOf(null)
      if (lane === -1) { lane = lanes.length; lanes.push(null) }
      lanes[lane] = entry.hash
    }

    const activeBefore = lanes.slice()
    const mergeLines = []

    for (let i = 0; i < entry.parents.length; i++) {
      const p = entry.parents[i]
      if (i === 0) {
        lanes[lane] = p || null
      } else {
        let pl = lanes.indexOf(p)
        if (pl === -1) {
          pl = lanes.indexOf(null)
          if (pl === -1) { pl = lanes.length; lanes.push(null) }
          lanes[pl] = p
        }
        mergeLines.push({ from: lane, to: pl })
      }
    }
    if (entry.parents.length === 0) lanes[lane] = null

    // Trim trailing nulls
    while (lanes.length > 0 && lanes[lanes.length - 1] === null) lanes.pop()

    rows.push({
      ...entry,
      lane,
      mergeLines,
      laneCount: Math.max(lanes.length, activeBefore.length, 1),
      activeLanes: activeBefore,
    })
  }
  return rows
}

const LANE_W = 16
const ROW_H  = 34
const DOT_R  = 4.5

function GraphRow({ row, nextRow, isLast, onContextMenu, isSelected }) {
  const laneCount = Math.max(row.laneCount, nextRow?.laneCount || 0, 1)
  const svgW = laneCount * LANE_W + 8
  const cx   = 4 + row.lane * LANE_W + LANE_W / 2
  const cy   = ROW_H / 2
  const color = LANE_COLORS[row.lane % LANE_COLORS.length]
  const isMerge = row.parents.length > 1

  // Draw lane pass-through lines
  const lines = []
  for (let i = 0; i < laneCount; i++) {
    const x = 4 + i * LANE_W + LANE_W / 2
    const lc = LANE_COLORS[i % LANE_COLORS.length]
    const active = row.activeLanes[i] != null

    if (i === row.lane) {
      // Top half always (something coming in)
      lines.push(<line key={`t${i}`} x1={x} y1={0} x2={x} y2={cy} stroke={lc} strokeWidth={1.5} opacity={0.5} />)
      // Bottom half only if there's a primary parent and not last
      if (row.parents.length > 0 && !isLast) {
        lines.push(<line key={`b${i}`} x1={x} y1={cy} x2={x} y2={ROW_H} stroke={lc} strokeWidth={1.5} opacity={0.5} />)
      }
    } else if (active) {
      lines.push(<line key={`p${i}`} x1={x} y1={0} x2={x} y2={ROW_H} stroke={lc} strokeWidth={1.5} opacity={0.35} />)
    }
  }

  // Merge curve paths
  const curves = row.mergeLines.map(({ from, to }, i) => {
    const fx = 4 + from * LANE_W + LANE_W / 2
    const tx = 4 + to   * LANE_W + LANE_W / 2
    return (
      <path key={`m${i}`}
        d={`M${fx},${cy} C${fx},${cy + 10} ${tx},${cy + 10} ${tx},${ROW_H}`}
        stroke={LANE_COLORS[to % LANE_COLORS.length]}
        strokeWidth={1.5} fill="none" opacity={0.45} />
    )
  })

  // Ref badges (HEAD, branch, origin/*)
  const refs = row.refs ? row.refs.split(', ').filter(Boolean) : []

  return (
    <div
      className={`flex items-stretch border-b border-border/30 transition-colors group cursor-pointer ${
        isSelected ? 'bg-accent/8' : 'hover:bg-bg-hover'
      }`}
      style={{ minHeight: ROW_H }}
      onContextMenu={(e) => { e.preventDefault(); onContextMenu(e, row) }}
    >
      {/* SVG lane graph */}
      <svg width={svgW} height={ROW_H} className="shrink-0 opacity-90" style={{ minWidth: svgW }}>
        {lines}
        {curves}
        {/* Dot background halo */}
        <circle cx={cx} cy={cy} r={DOT_R + 2.5} fill="var(--c-bg-primary)" />
        {isMerge ? (
          <>
            <circle cx={cx} cy={cy} r={DOT_R + 1} fill={color} opacity={0.25} />
            <circle cx={cx} cy={cy} r={DOT_R} fill={color} />
            <circle cx={cx} cy={cy} r={DOT_R - 2} fill="var(--c-bg-primary)" />
          </>
        ) : (
          <>
            <circle cx={cx} cy={cy} r={DOT_R + 1} fill={color} opacity={0.2} />
            <circle cx={cx} cy={cy} r={DOT_R} fill={color} />
          </>
        )}
      </svg>

      {/* Commit text info */}
      <div className="flex-1 min-w-0 flex flex-col justify-center py-1.5 pr-3 gap-0.5 overflow-hidden">
        {/* Subject line + refs */}
        <div className="flex items-center gap-1 min-w-0 overflow-hidden">
          {refs.map((ref) => (
            <span key={ref} className={`shrink-0 inline-flex items-center px-1.5 py-px rounded text-[9px] font-bold tracking-wide ${
              ref.includes('HEAD')
                ? 'bg-green/15 text-green border border-green/20'
                : ref.startsWith('origin/') || ref.startsWith('remotes/')
                  ? 'bg-peach/12 text-peach border border-peach/20'
                  : 'bg-accent/12 text-accent border border-accent/20'
            }`}>
              {ref.replace('refs/remotes/', '').replace('refs/heads/', '')}
            </span>
          ))}
          <span className="text-[12px] text-text-primary truncate font-medium leading-tight">{row.message}</span>
        </div>

        {/* Metadata row */}
        <div className="flex items-center gap-2 min-w-0 overflow-hidden">
          <span className="font-mono text-[10px] text-accent/70 shrink-0 tracking-wide">{row.short}</span>
          <span className="flex items-center gap-0.5 text-[10px] text-text-muted truncate">
            <User className="w-2.5 h-2.5 shrink-0" />
            <span className="truncate">{row.author}</span>
          </span>
          <span className="flex items-center gap-0.5 text-[10px] text-text-muted shrink-0 ml-auto opacity-0 group-hover:opacity-100 transition-opacity">
            <Clock className="w-2.5 h-2.5 shrink-0" />
            {row.date}
          </span>
        </div>
      </div>
    </div>
  )
}

function GitGraph({ entries, onCommitAction }) {
  const rows = useMemo(() => buildGraph(entries), [entries])
  const [menu, setMenu] = useState(null)
  const [selected, setSelected] = useState(null)

  const handleCtx = (e, row) => {
    e.preventDefault()
    setSelected(row.hash)
    setMenu({ x: e.clientX, y: e.clientY, commit: row })
  }

  const handleAction = async (action, hash) => {
    setMenu(null)
    onCommitAction?.(action, hash)
  }

  if (!rows.length) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-text-muted px-6">
        <div className="w-10 h-10 rounded-xl bg-bg-hover flex items-center justify-center mb-3">
          <GitCommit className="w-5 h-5 opacity-30" />
        </div>
        <span className="text-[12px] font-medium">No commits yet</span>
        <span className="text-[11px] mt-1 text-center">Make your first commit in the Changes tab</span>
      </div>
    )
  }

  return (
    <div className="overflow-y-auto overflow-x-hidden" onClick={() => setSelected(null)}>
      {rows.map((row, i) => (
        <GraphRow
          key={row.hash}
          row={row}
          nextRow={rows[i + 1]}
          isLast={i === rows.length - 1}
          onContextMenu={handleCtx}
          isSelected={selected === row.hash}
        />
      ))}
      {menu && (
        <CommitMenu
          x={menu.x} y={menu.y} commit={menu.commit}
          onClose={() => setMenu(null)}
          onAction={handleAction}
        />
      )}
    </div>
  )
}

/* ═══════════════════════════════════════════════════
   Status badge for file status
═══════════════════════════════════════════════════ */

const STATUS_META = {
  modified:  { label: 'M', color: 'text-yellow', ring: 'bg-yellow/10 border-yellow/25' },
  added:     { label: 'A', color: 'text-green',  ring: 'bg-green/10 border-green/25' },
  deleted:   { label: 'D', color: 'text-red',    ring: 'bg-red/10 border-red/25' },
  untracked: { label: 'U', color: 'text-green',  ring: 'bg-green/10 border-green/25' },
  copied:    { label: 'C', color: 'text-cyan',   ring: 'bg-cyan/10 border-cyan/25' },
  renamed:   { label: 'R', color: 'text-accent', ring: 'bg-accent/10 border-accent/25' },
}

function FileBadge({ status }) {
  const s = STATUS_META[status] || STATUS_META.modified
  return (
    <span className={`w-5 h-5 flex items-center justify-center rounded border text-[9px] font-bold shrink-0 ${s.color} ${s.ring}`}>
      {s.label}
    </span>
  )
}

/* ═══════════════════════════════════════════════════
   Changed-file row
═══════════════════════════════════════════════════ */

function FileRow({ file, action, onAction }) {
  const filename = file.path.split('/').pop()
  const dir = file.path.includes('/') ? file.path.slice(0, file.path.lastIndexOf('/')) : ''

  return (
    <div className="flex items-center px-3 py-1.5 hover:bg-bg-hover transition-colors group overflow-hidden cursor-default">
      <FileBadge status={file.status} />
      <div className="flex-1 min-w-0 ml-2.5 overflow-hidden">
        <div className="flex items-baseline gap-1.5 min-w-0 overflow-hidden">
          <span className="text-[12px] text-text-primary font-medium truncate leading-tight">{filename}</span>
          {dir && <span className="text-[10px] text-text-muted truncate shrink">{dir}</span>}
        </div>
      </div>
      <button
        onClick={() => onAction(file.path)}
        className="opacity-0 group-hover:opacity-100 ml-2 w-6 h-6 flex items-center justify-center rounded-md bg-bg-active hover:bg-border-active text-text-muted hover:text-text-primary transition-all shrink-0"
        title={action === 'stage' ? 'Stage file' : 'Unstage file'}
      >
        {action === 'stage' ? <Plus className="w-3 h-3" /> : <Minus className="w-3 h-3" />}
      </button>
    </div>
  )
}

/* ═══════════════════════════════════════════════════
   Collapsible section header
═══════════════════════════════════════════════════ */

function SectionHeader({ label, count, colorClass, defaultOpen = true, children, onStageAll, onUnstageAll }) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div>
      <div className={`flex items-center px-3 py-1.5 select-none ${open ? 'border-b border-border/40' : ''}`}>
        <button
          onClick={() => setOpen(!open)}
          className="flex items-center gap-1.5 flex-1 min-w-0"
        >
          {open
            ? <ChevronDown className="w-3 h-3 text-text-muted shrink-0" />
            : <ChevronRight className="w-3 h-3 text-text-muted shrink-0" />
          }
          <span className={`text-[10px] font-bold uppercase tracking-widest ${colorClass}`}>{label}</span>
          <span className={`ml-1 text-[10px] font-semibold ${colorClass} opacity-60`}>{count}</span>
        </button>
        {onStageAll && (
          <button onClick={onStageAll}
            className="text-[10px] text-text-muted hover:text-text-primary px-1.5 py-0.5 rounded hover:bg-bg-hover transition-colors font-medium shrink-0">
            Stage all
          </button>
        )}
        {onUnstageAll && (
          <button onClick={onUnstageAll}
            className="text-[10px] text-text-muted hover:text-text-primary px-1.5 py-0.5 rounded hover:bg-bg-hover transition-colors font-medium shrink-0">
            Unstage all
          </button>
        )}
      </div>
      {open && (
        <div className="py-0.5">
          {children}
        </div>
      )}
    </div>
  )
}

/* ═══════════════════════════════════════════════════
   Main GitPanel
═══════════════════════════════════════════════════ */

const TABS = [
  { key: 'changes',  label: 'Changes' },
  { key: 'graph',    label: 'History' },
  { key: 'branches', label: 'Branches' },
  { key: 'remotes',  label: 'Remote' },
]

export default function GitPanel({ authFetch, visible }) {
  const [status, setStatus]     = useState({ branch: '', files: [], isRepo: true })
  const [log, setLog]           = useState([])
  const [branches, setBranches] = useState([])
  const [remotes, setRemotes]   = useState([])
  const [commitMsg, setCommitMsg] = useState('')
  const [section, setSection]   = useState('changes')
  const [loading, setLoading]   = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [remoteOp, setRemoteOp] = useState('') // 'push'|'pull'|'fetch'|''
  const [remoteMsg, setRemoteMsg] = useState('') // success/error message from remote op
  const [newBranch, setNewBranch] = useState('') // value for create-branch input
  const [showNewBranch, setShowNewBranch] = useState(false)

  const refresh = useCallback(async (quiet = false) => {
    if (!visible) return
    if (!quiet) setRefreshing(true)
    try {
      const [sRes, lRes, bRes, rRes] = await Promise.all([
        authFetch('/api/git/status'),
        authFetch('/api/git/log'),
        authFetch('/api/git/branches'),
        authFetch('/api/git/remotes'),
      ])
      const [sData, lData, bData, rData] = await Promise.all([
        sRes.json(), lRes.json(), bRes.json(), rRes.json(),
      ])
      setStatus(sData)
      setLog(lData.entries || [])
      setBranches(bData.branches || [])
      setRemotes(rData.remotes || [])
    } catch { /* ignore */ }
    setRefreshing(false)
  }, [authFetch, visible])

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => { refresh(true) }, [refresh])
  /* eslint-enable react-hooks/set-state-in-effect */

  const handleStage   = async (path) => {
    await authFetch('/api/git/stage',   { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path }) })
    refresh(true)
  }
  const handleUnstage = async (path) => {
    await authFetch('/api/git/unstage', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path }) })
    refresh(true)
  }
  const handleStageAll = async () => {
    await authFetch('/api/git/stage',   { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path: '.' }) })
    refresh(true)
  }
  const handleUnstageAll = async () => {
    await authFetch('/api/git/unstage', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path: '.' }) })
    refresh(true)
  }
  const handleCommit = async (e) => {
    e.preventDefault()
    if (!commitMsg.trim() || staged.length === 0) return
    setLoading(true)
    await authFetch('/api/git/commit', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: commitMsg }),
    })
    setCommitMsg('')
    setLoading(false)
    refresh(true)
  }
  const handleCheckout = async (branch) => {
    await authFetch('/api/git/checkout', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ branch }),
    })
    refresh(true)
  }
  const handleCommitAction = async (action, hash) => {
    if (action === 'checkout') {
      await authFetch('/api/git/checkout', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ branch: hash }),
      })
      refresh(true)
    }
  }

  const runRemoteOp = async (op) => {
    setRemoteOp(op)
    setRemoteMsg('')
    try {
      const remote = remotes[0]?.name || 'origin'
      const res = await authFetch(`/api/git/${op}`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ remote, branch: status.branch }),
      })
      const data = await res.json()
      if (data.error) {
        setRemoteMsg('Error: ' + data.error)
      } else {
        setRemoteMsg(data.output || op + ' successful')
        refresh(true)
      }
    } catch (e) {
      setRemoteMsg('Error: ' + e.message)
    }
    setRemoteOp('')
  }

  const handleCreateBranch = async (e) => {
    e.preventDefault()
    if (!newBranch.trim()) return
    setLoading(true)
    try {
      const res = await authFetch('/api/git/branch', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newBranch.trim(), checkout: true }),
      })
      const data = await res.json()
      if (!data.error) {
        setNewBranch('')
        setShowNewBranch(false)
        refresh(true)
      }
    } catch (err) { void err }
    setLoading(false)
  }

  if (!visible) return null

  const staged   = status.files?.filter((f) => f.staged)  || []
  const unstaged = status.files?.filter((f) => !f.staged) || []
  const totalChanges = staged.length + unstaged.length

  // Not a git repo
  if (!status.isRepo && status.isRepo !== undefined) {
    return (
      <div className="h-full flex flex-col bg-bg-secondary">
        <PanelHeader branch={null} onRefresh={refresh} refreshing={refreshing} />
        <div className="flex-1 flex items-center justify-center p-6">
          <div className="text-center max-w-[180px]">
            <div className="w-12 h-12 rounded-xl bg-bg-hover flex items-center justify-center mx-auto mb-3">
              <GitBranch className="w-6 h-6 text-text-muted opacity-40" />
            </div>
            <p className="text-[13px] font-medium text-text-secondary mb-1">Not a git repo</p>
            <p className="text-[11px] text-text-muted">
              Run <code className="text-accent bg-accent/10 px-1 py-0.5 rounded font-mono text-[10px]">git init</code> in the terminal
            </p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden">
      <PanelHeader branch={status.branch} onRefresh={refresh} refreshing={refreshing} />

      {/* Section tabs */}
      <div className="flex border-b border-border shrink-0 bg-bg-secondary">
        {TABS.map(({ key, label }) => {
          const badge = key === 'changes' ? totalChanges : key === 'branches' ? branches.length : 0
          const active = section === key
          return (
            <button key={key} onClick={() => setSection(key)}
              className={`relative flex-1 flex items-center justify-center gap-1.5 py-2 text-[11px] font-medium transition-colors ${
                active ? 'text-text-primary' : 'text-text-muted hover:text-text-secondary'
              }`}>
              {label}
              {badge > 0 && (
                <span className={`text-[9px] font-bold px-1.5 py-0.5 rounded-full ${
                  active ? 'bg-accent/20 text-accent' : 'bg-bg-active text-text-muted'
                }`}>
                  {badge}
                </span>
              )}
              {active && <span className="tab-active-line" />}
            </button>
          )
        })}
      </div>

      {/* Panel content */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden min-h-0">

        {/* ── Changes ── */}
        {section === 'changes' && (
          <div className="flex flex-col">
            {/* Commit box */}
            <form onSubmit={handleCommit} className="p-3 border-b border-border bg-bg-secondary">
              <textarea
                value={commitMsg}
                onChange={(e) => setCommitMsg(e.target.value)}
                placeholder="Summary (press ↵↵ for body)"
                rows={2}
                className="w-full bg-bg-input border border-border rounded-md px-3 py-2 text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 focus:ring-1 focus:ring-accent/20 resize-none leading-relaxed transition-colors"
              />
              <div className="flex gap-2 mt-2">
                {unstaged.length > 0 && staged.length === 0 && (
                  <button type="button" onClick={handleStageAll}
                    className="flex-1 text-[11px] py-1.5 rounded-md border border-border text-text-secondary hover:text-text-primary hover:bg-bg-hover transition-colors font-medium">
                    Stage All
                  </button>
                )}
                <button
                  type="submit"
                  disabled={!commitMsg.trim() || loading || staged.length === 0}
                  className="flex-1 flex items-center justify-center gap-1.5 bg-accent text-white text-[11px] py-1.5 rounded-md hover:bg-accent-hover disabled:opacity-30 disabled:cursor-not-allowed transition-all font-semibold shadow-sm shadow-accent/25"
                >
                  <Check className="w-3 h-3" />
                  {loading ? 'Committing…' : staged.length > 0 ? `Commit ${staged.length} file${staged.length !== 1 ? 's' : ''}` : 'Commit'}
                </button>
              </div>
            </form>

            {/* Staged files */}
            {staged.length > 0 && (
              <SectionHeader
                label="Staged" count={staged.length} colorClass="text-green"
                onUnstageAll={handleUnstageAll}
              >
                {staged.map((f) => (
                  <FileRow key={f.path} file={f} action="unstage" onAction={handleUnstage} />
                ))}
              </SectionHeader>
            )}

            {/* Unstaged / changed files */}
            {unstaged.length > 0 && (
              <SectionHeader
                label="Changes" count={unstaged.length} colorClass="text-yellow"
                onStageAll={handleStageAll}
              >
                {unstaged.map((f) => (
                  <FileRow key={f.path} file={f} action="stage" onAction={handleStage} />
                ))}
              </SectionHeader>
            )}

            {staged.length === 0 && unstaged.length === 0 && (
              <div className="flex flex-col items-center justify-center py-12 text-text-muted px-6">
                <div className="w-10 h-10 rounded-xl bg-green/8 border border-green/15 flex items-center justify-center mb-3">
                  <Check className="w-5 h-5 text-green opacity-70" />
                </div>
                <span className="text-[12px] font-medium text-text-secondary">Working tree clean</span>
                <span className="text-[11px] mt-1">No changes to commit</span>
              </div>
            )}
          </div>
        )}

        {/* ── History (git graph) ── */}
        {section === 'graph' && (
          <GitGraph entries={log} onCommitAction={handleCommitAction} />
        )}

        {/* ── Branches ── */}
        {section === 'branches' && (
          <div className="py-1">
            {/* Create branch */}
            <div className="px-3 pb-2 border-b border-border/40">
              {showNewBranch ? (
                <form onSubmit={handleCreateBranch} className="flex gap-1.5 mt-1.5">
                  <input
                    autoFocus
                    type="text"
                    value={newBranch}
                    onChange={(e) => setNewBranch(e.target.value)}
                    placeholder="new-branch-name"
                    className="flex-1 min-w-0 bg-bg-input border border-border rounded-md px-2.5 py-1 text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 transition-colors"
                  />
                  <button type="submit" disabled={loading || !newBranch.trim()}
                    className="px-2 py-1 text-[11px] bg-accent text-white rounded-md hover:bg-accent-hover disabled:opacity-40 transition-all font-medium">
                    Create
                  </button>
                  <button type="button" onClick={() => { setShowNewBranch(false); setNewBranch('') }}
                    className="px-2 py-1 text-[11px] border border-border rounded-md text-text-muted hover:bg-bg-hover transition-colors">
                    <X className="w-3 h-3" />
                  </button>
                </form>
              ) : (
                <button onClick={() => setShowNewBranch(true)}
                  className="mt-1.5 w-full flex items-center justify-center gap-1.5 py-1.5 text-[11px] border border-dashed border-border text-text-muted hover:text-text-primary hover:border-border-active rounded-md transition-colors">
                  <Plus className="w-3 h-3" />
                  New branch
                </button>
              )}
            </div>

            {branches.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-12 text-text-muted">
                <GitBranch className="w-6 h-6 mb-2 opacity-25" />
                <span className="text-[12px]">No branches</span>
              </div>
            ) : (
              branches.map((b) => (
                <button
                  key={b.name}
                  onClick={() => !b.current && handleCheckout(b.name)}
                  className={`w-full flex items-center gap-2.5 px-3 py-2 transition-colors text-left overflow-hidden ${
                    b.current
                      ? 'text-text-primary bg-accent/5 cursor-default'
                      : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'
                  }`}
                >
                  <GitBranch className={`w-3.5 h-3.5 shrink-0 ${b.current ? 'text-green' : 'text-text-muted'}`} />
                  <span className="truncate text-[12px] font-mono font-medium">{b.name}</span>
                  {b.current && (
                    <span className="ml-auto shrink-0 flex items-center gap-1 text-[10px] text-green font-semibold">
                      <span className="w-1.5 h-1.5 rounded-full bg-green" />
                      current
                    </span>
                  )}
                </button>
              ))
            )}
          </div>
        )}

        {/* ── Remote operations ── */}
        {section === 'remotes' && (
          <div className="p-3 space-y-3">
            {/* Remote list */}
            {remotes.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-8 text-text-muted">
                <AlertCircle className="w-5 h-5 mb-2 opacity-30" />
                <span className="text-[12px]">No remotes configured</span>
                <span className="text-[11px] mt-1 text-center">Run <code className="text-accent bg-accent/10 px-1 rounded font-mono text-[10px]">git remote add origin &lt;url&gt;</code> in the terminal</span>
              </div>
            ) : (
              <div>
                <div className="text-[10px] font-bold uppercase tracking-widest text-text-muted mb-2">Remotes</div>
                {remotes.map((r) => (
                  <div key={r.name} className="flex items-center gap-2 px-2 py-1.5 bg-bg-primary border border-border rounded-lg mb-1.5">
                    <span className="text-[12px] font-mono font-medium text-text-primary shrink-0">{r.name}</span>
                    <span className="text-[10px] text-text-muted truncate">{r.url}</span>
                  </div>
                ))}
              </div>
            )}

            {/* Operation buttons */}
            <div className="space-y-2">
              <div className="text-[10px] font-bold uppercase tracking-widest text-text-muted mb-2">Operations</div>

              <RemoteOpBtn
                icon={Download}
                label="Fetch"
                desc="Download objects & refs"
                loading={remoteOp === 'fetch'}
                disabled={!!remoteOp || remotes.length === 0}
                onClick={() => runRemoteOp('fetch')}
              />
              <RemoteOpBtn
                icon={CloudDownload}
                label="Pull"
                desc="Fetch + merge current branch"
                loading={remoteOp === 'pull'}
                disabled={!!remoteOp || remotes.length === 0}
                onClick={() => runRemoteOp('pull')}
              />
              <RemoteOpBtn
                icon={Upload}
                label="Push"
                desc="Upload commits to remote"
                loading={remoteOp === 'push'}
                disabled={!!remoteOp || remotes.length === 0}
                onClick={() => runRemoteOp('push')}
              />
            </div>

            {/* Operation output */}
            {remoteMsg && (
              <div className={`p-2.5 rounded-lg text-[11px] font-mono border ${
                remoteMsg.startsWith('Error:')
                  ? 'bg-red/5 border-red/20 text-red'
                  : 'bg-green/5 border-green/20 text-green'
              }`}>
                {remoteMsg}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

/* Remote operation button */
// eslint-disable-next-line no-unused-vars
function RemoteOpBtn({ icon: Icon, label, desc, loading, disabled, onClick }) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className="w-full flex items-center gap-3 p-3 bg-bg-primary border border-border rounded-lg hover:bg-bg-hover hover:border-border-active transition-all text-left disabled:opacity-40 disabled:cursor-not-allowed"
    >
      <div className="w-7 h-7 flex items-center justify-center rounded-md bg-accent/10 shrink-0">
        <Icon className={`w-4 h-4 text-accent ${loading ? 'animate-spin' : ''}`} />
      </div>
      <div className="flex-1 min-w-0">
        <div className="text-[12px] font-medium text-text-primary">{label}</div>
        <div className="text-[10px] text-text-muted truncate">{desc}</div>
      </div>
      {loading && (
        <RefreshCw className="w-3.5 h-3.5 text-text-muted animate-spin shrink-0" />
      )}
    </button>
  )
}

/* Shared panel header */
function PanelHeader({ branch, onRefresh, refreshing }) {
  return (
    <div className="flex items-center justify-between px-3 py-2 border-b border-border shrink-0">
      <div className="flex items-center gap-2 min-w-0 overflow-hidden">
        <GitMerge className="w-3.5 h-3.5 text-accent shrink-0" />
        <span className="text-[11px] font-semibold text-text-secondary uppercase tracking-widest shrink-0">
          Source Control
        </span>
        {branch && (
          <span className="flex items-center gap-1 ml-1 px-2 py-0.5 rounded-md text-[11px] font-mono font-semibold text-accent bg-accent/10 border border-accent/15 truncate">
            <GitBranch className="w-2.5 h-2.5 shrink-0" />
            {branch}
          </span>
        )}
      </div>
      <button
        onClick={() => onRefresh(false)}
        className={`p-1.5 rounded-md text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors shrink-0 ${refreshing ? 'animate-spin' : ''}`}
        title="Refresh"
      >
        <RefreshCw className="w-3.5 h-3.5" />
      </button>
    </div>
  )
}
