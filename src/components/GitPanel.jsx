import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import {
  GitBranch, Plus, Minus, RefreshCw, Check,
  ChevronDown, ChevronRight, Copy, GitCommit,
  GitMerge, Clock, User, Hash, Upload, Download, CloudDownload,
  AlertCircle, X, Trash2, Eye, EyeOff, Package
} from 'lucide-react'

/* ═══════════════════════════════════════════════════
   Unified-diff parser + inline viewer
═══════════════════════════════════════════════════ */

const MAX_DIFF_LINES = 200

function parseDiff(text) {
  if (!text) return []
  const lines = text.split('\n')
  const result = []
  for (const line of lines) {
    if (line.startsWith('+') && !line.startsWith('+++')) {
      result.push({ type: 'add', text: line })
    } else if (line.startsWith('-') && !line.startsWith('---')) {
      result.push({ type: 'del', text: line })
    } else if (line.startsWith('@@')) {
      result.push({ type: 'hunk', text: line })
    } else if (line.startsWith('+++') || line.startsWith('---')) {
      result.push({ type: 'meta', text: line })
    } else {
      result.push({ type: 'ctx', text: line })
    }
  }
  return result
}

function DiffViewer({ lines }) {
  const truncated = lines.length > MAX_DIFF_LINES
  const visible = truncated ? lines.slice(0, MAX_DIFF_LINES) : lines

  return (
    <pre className="overflow-x-auto text-[11px] font-mono leading-[1.55] select-text p-0 m-0">
      {visible.map((line, i) => {
        let cls = 'block px-3 whitespace-pre'
        if (line.type === 'add')  cls += ' bg-green/10 text-green'
        else if (line.type === 'del')  cls += ' bg-red/10 text-red'
        else if (line.type === 'hunk') cls += ' text-accent/70 bg-accent/5'
        else if (line.type === 'meta') cls += ' text-text-muted'
        else cls += ' text-text-secondary'
        return <span key={i} className={cls}>{line.text || ' '}</span>
      })}
      {truncated && (
        <span className="block px-3 py-1 text-text-muted italic bg-bg-hover">
          … {lines.length - MAX_DIFF_LINES} more lines (open file to see full diff)
        </span>
      )}
    </pre>
  )
}

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

function GraphRow({ row, nextRow, isLast, onContextMenu, isSelected, onClick }) {
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
      onClick={() => onClick?.(row)}
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

/* ═══════════════════════════════════════════════════
   Commit detail panel (diff for a selected commit)
═══════════════════════════════════════════════════ */

function CommitDetail({ commit, authFetch, onClose }) {
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [expandedFile, setExpandedFile] = useState(null)

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (!commit) return
    setLoading(true)
    setData(null)
    setExpandedFile(null)
    authFetch(`/api/git/commit-diff?hash=${encodeURIComponent(commit.hash)}`)
      .then((r) => r.json())
      .then((d) => { setData(d); setLoading(false) })
      .catch(() => setLoading(false))
  }, [commit, authFetch])
  /* eslint-enable react-hooks/set-state-in-effect */

  if (!commit) return null

  const diffLines = parseDiff(data?.diff || '')

  return (
    <div className="border-t border-border bg-bg-primary flex flex-col" style={{ maxHeight: '55%' }}>
      {/* Header */}
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border shrink-0">
        <span className="font-mono text-[10px] text-accent bg-accent/10 px-1.5 py-0.5 rounded font-semibold shrink-0">{commit.short}</span>
        <span className="text-[11px] text-text-secondary truncate flex-1">{commit.message}</span>
        <button onClick={onClose}
          className="p-0.5 rounded text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors shrink-0">
          <X className="w-3.5 h-3.5" />
        </button>
      </div>

      <div className="overflow-y-auto flex-1 min-h-0">
        {loading && (
          <div className="flex items-center justify-center py-8 gap-2 text-text-muted">
            <RefreshCw className="w-3.5 h-3.5 animate-spin" />
            <span className="text-[12px]">Loading diff…</span>
          </div>
        )}

        {!loading && data && (
          <>
            {/* Files changed summary */}
            {data.files && data.files.length > 0 && (
              <div className="border-b border-border/40 py-1">
                {data.files.map((f, i) => (
                  <button
                    key={i}
                    onClick={() => setExpandedFile(expandedFile === f ? null : f)}
                    className="w-full flex items-center gap-2 px-3 py-1 hover:bg-bg-hover transition-colors text-left"
                  >
                    {expandedFile === f
                      ? <EyeOff className="w-3 h-3 text-text-muted shrink-0" />
                      : <Eye className="w-3 h-3 text-text-muted shrink-0" />
                    }
                    <span className="text-[11px] text-text-secondary truncate font-mono">{f}</span>
                  </button>
                ))}
              </div>
            )}

            {/* Diff */}
            {diffLines.length > 0 ? (
              <div className="bg-bg-secondary border-border/30">
                <DiffViewer lines={diffLines} />
              </div>
            ) : (
              !loading && (
                <div className="flex items-center justify-center py-6 text-text-muted">
                  <span className="text-[11px]">No diff available</span>
                </div>
              )
            )}
          </>
        )}
      </div>
    </div>
  )
}

function GitGraph({ entries, authFetch, onCommitAction }) {
  const rows = useMemo(() => buildGraph(entries), [entries])
  const [menu, setMenu] = useState(null)
  const [selected, setSelected] = useState(null)

  const handleCtx = (e, row) => {
    e.preventDefault()
    setMenu({ x: e.clientX, y: e.clientY, commit: row })
  }

  const handleClick = (row) => {
    setSelected((prev) => prev?.hash === row.hash ? null : row)
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
    <div className="flex flex-col h-full min-h-0">
      <div className="overflow-y-auto overflow-x-hidden flex-1 min-h-0" onClick={(e) => {
        if (e.target === e.currentTarget) setSelected(null)
      }}>
        {rows.map((row, i) => (
          <GraphRow
            key={row.hash}
            row={row}
            nextRow={rows[i + 1]}
            isLast={i === rows.length - 1}
            onContextMenu={handleCtx}
            isSelected={selected?.hash === row.hash}
            onClick={handleClick}
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

      {selected && (
        <CommitDetail
          commit={selected}
          authFetch={authFetch}
          onClose={() => setSelected(null)}
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
  conflict:  { label: '!', color: 'text-red',    ring: 'bg-red/10 border-red/25' },
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
   Toast notification (lightweight, inline)
═══════════════════════════════════════════════════ */

function Toast({ message, type, onDismiss }) {
  useEffect(() => {
    const t = setTimeout(onDismiss, 4000)
    return () => clearTimeout(t)
  }, [onDismiss])

  return (
    <div className={`mx-3 mb-2 flex items-center gap-2 px-3 py-2 rounded-lg text-[11px] border animate-fade-in ${
      type === 'error'
        ? 'bg-red/8 border-red/20 text-red'
        : 'bg-green/8 border-green/20 text-green'
    }`}>
      <AlertCircle className="w-3.5 h-3.5 shrink-0" />
      <span className="flex-1 truncate">{message}</span>
      <button onClick={onDismiss} className="shrink-0 opacity-60 hover:opacity-100">
        <X className="w-3 h-3" />
      </button>
    </div>
  )
}

/* ═══════════════════════════════════════════════════
   Per-hunk staging helper
═══════════════════════════════════════════════════ */

// parseHunks splits diff text into header + array of hunks.
// Each hunk includes the file header lines so it forms a valid patch.
function parseHunks(diffText, filePath) {
  if (!diffText) return []
  const lines = diffText.split('\n')
  const headerLines = []
  const hunks = []
  let i = 0

  // Collect header lines (--- / +++ / diff --git etc.) until first @@
  while (i < lines.length && !lines[i].startsWith('@@')) {
    headerLines.push(lines[i])
    i++
  }

  // Build a minimal header if none found
  const header = headerLines.length > 0
    ? headerLines.join('\n') + '\n'
    : `--- a/${filePath}\n+++ b/${filePath}\n`

  // Parse each @@ block
  while (i < lines.length) {
    if (lines[i].startsWith('@@')) {
      const hunkLines = [lines[i]]
      i++
      while (i < lines.length && !lines[i].startsWith('@@')) {
        hunkLines.push(lines[i])
        i++
      }
      hunks.push({ patch: header + hunkLines.join('\n') + '\n', headerLine: hunkLines[0] })
    } else {
      i++
    }
  }
  return hunks
}

/* ═══════════════════════════════════════════════════
   Inline diff panel for a changed file (with per-hunk staging)
═══════════════════════════════════════════════════ */

function FileDiffPanel({ file, staged, authFetch, onRefresh }) {
  const [diffText, setDiffText] = useState(null)
  const [loading, setLoading] = useState(true)
  const [stagingHunk, setStagingHunk] = useState(null) // index being staged

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    setLoading(true)
    setDiffText(null)
    const url = `/api/git/diff?file=${encodeURIComponent(file.path)}&staged=${staged ? 'true' : 'false'}`
    authFetch(url)
      .then((r) => r.json())
      .then((data) => { setDiffText(data.diff || ''); setLoading(false) })
      .catch(() => { setDiffText(''); setLoading(false) })
  }, [file.path, staged, authFetch])
  /* eslint-enable react-hooks/set-state-in-effect */

  const handleStageHunk = async (hunk, idx) => {
    setStagingHunk(idx)
    try {
      await authFetch('/api/git/stage-hunk', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ patch: hunk.patch, reverse: staged }),
      })
      onRefresh?.()
    } catch { /* ignore */ }
    setStagingHunk(null)
  }

  const lines = diffText != null ? parseDiff(diffText) : null
  const hunks = diffText ? parseHunks(diffText, file.path) : []

  // Build a map from hunk header line → hunk index so we can show the button
  const hunkHeaderToIdx = {}
  hunks.forEach((h, i) => { hunkHeaderToIdx[h.headerLine] = i })

  return (
    <div className="border-t border-border/40 bg-bg-primary overflow-x-auto" style={{ maxHeight: 280 }}>
      {loading ? (
        <div className="flex items-center gap-2 px-3 py-3 text-text-muted">
          <RefreshCw className="w-3 h-3 animate-spin shrink-0" />
          <span className="text-[11px]">Loading diff…</span>
        </div>
      ) : lines && lines.length > 0 ? (
        <div className="overflow-y-auto" style={{ maxHeight: 280 }}>
          <pre className="overflow-x-auto text-[11px] font-mono leading-[1.55] select-text p-0 m-0">
            {lines.slice(0, MAX_DIFF_LINES).map((line, i) => {
              let cls = 'block px-3 whitespace-pre'
              if (line.type === 'add')  cls += ' bg-green/10 text-green'
              else if (line.type === 'del')  cls += ' bg-red/10 text-red'
              else if (line.type === 'hunk') cls += ' text-accent/70 bg-accent/5'
              else if (line.type === 'meta') cls += ' text-text-muted'
              else cls += ' text-text-secondary'

              const hunkIdx = line.type === 'hunk' ? hunkHeaderToIdx[line.text] : undefined
              const isHunkLine = hunkIdx !== undefined

              return (
                <span key={i} className={cls + (isHunkLine ? ' flex items-center justify-between group pr-1' : '')}>
                  <span>{line.text || ' '}</span>
                  {isHunkLine && (
                    <button
                      onClick={(e) => { e.stopPropagation(); handleStageHunk(hunks[hunkIdx], hunkIdx) }}
                      disabled={stagingHunk !== null}
                      className="opacity-0 group-hover:opacity-100 ml-2 px-1.5 py-0.5 text-[9px] font-semibold rounded bg-accent/20 text-accent hover:bg-accent/35 transition-all disabled:opacity-30 shrink-0"
                      title={staged ? 'Unstage hunk' : 'Stage hunk'}
                    >
                      {stagingHunk === hunkIdx
                        ? <RefreshCw className="w-2.5 h-2.5 animate-spin inline" />
                        : (staged ? '–' : '+')
                      }
                    </button>
                  )}
                </span>
              )
            })}
            {lines.length > MAX_DIFF_LINES && (
              <span className="block px-3 py-1 text-text-muted italic bg-bg-hover">
                … {lines.length - MAX_DIFF_LINES} more lines (open file to see full diff)
              </span>
            )}
          </pre>
        </div>
      ) : (
        <div className="px-3 py-3 text-[11px] text-text-muted italic">No diff available</div>
      )}
    </div>
  )
}

/* ═══════════════════════════════════════════════════
   Conflict resolver — shown for conflicted files
═══════════════════════════════════════════════════ */

function ConflictResolver({ file, authFetch, onRefresh }) {
  const [regions, setRegions] = useState(null)
  const [loading, setLoading] = useState(true)
  const [resolutions, setResolutions] = useState({}) // index → 'current'|'incoming'|'both'
  const [resolving, setResolving] = useState(false)

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    setLoading(true)
    setRegions(null)
    setResolutions({})
    authFetch(`/api/git/conflict?file=${encodeURIComponent(file.path)}`)
      .then((r) => r.json())
      .then((data) => { setRegions(data.regions || []); setLoading(false) })
      .catch(() => { setRegions([]); setLoading(false) })
  }, [file.path, authFetch])
  /* eslint-enable react-hooks/set-state-in-effect */

  const allResolved = regions && regions.length > 0 && regions.every((r) => resolutions[r.index])

  const handleResolve = async () => {
    if (!allResolved) return
    setResolving(true)
    try {
      const resArr = Object.entries(resolutions).map(([idx, choice]) => ({ index: Number(idx), choice }))
      await authFetch('/api/git/conflict/resolve', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: file.path, resolutions: resArr }),
      })
      onRefresh?.()
    } catch { /* ignore */ }
    setResolving(false)
  }

  const setChoice = (idx, choice) => {
    setResolutions((prev) => ({ ...prev, [idx]: choice }))
  }

  return (
    <div className="border-t border-border/40 bg-bg-primary" style={{ maxHeight: 320, overflowY: 'auto' }}>
      {loading ? (
        <div className="flex items-center gap-2 px-3 py-3 text-text-muted">
          <RefreshCw className="w-3 h-3 animate-spin shrink-0" />
          <span className="text-[11px]">Loading conflicts…</span>
        </div>
      ) : regions && regions.length > 0 ? (
        <div className="p-2 space-y-2">
          {regions.map((region) => {
            const choice = resolutions[region.index]
            return (
              <div key={region.index} className="border border-border rounded-lg overflow-hidden text-[11px]">
                {/* Region header */}
                <div className="px-2.5 py-1 bg-bg-elevated border-b border-border text-[10px] text-text-muted font-mono">
                  Conflict #{region.index + 1} · lines {region.startLine}–{region.endLine}
                </div>

                {/* Current (HEAD) */}
                <div className={`border-b border-border/40 ${choice === 'current' || choice === 'both' ? 'bg-green/8' : 'bg-bg-secondary'}`}>
                  <div className="px-2.5 pt-1 pb-0.5 flex items-center justify-between">
                    <span className="text-[10px] text-green font-semibold uppercase tracking-wide">Current (HEAD)</span>
                    <div className="flex gap-1">
                      <button
                        onClick={() => setChoice(region.index, 'current')}
                        className={`px-2 py-0.5 rounded text-[10px] font-medium transition-colors ${choice === 'current' ? 'bg-green/25 text-green' : 'text-text-muted hover:bg-green/15 hover:text-green'}`}
                      >Accept</button>
                    </div>
                  </div>
                  <pre className="px-3 pb-1.5 text-[11px] font-mono text-green/80 whitespace-pre-wrap overflow-x-auto">
                    {region.currentLines.length > 0 ? region.currentLines.join('\n') : <span className="italic opacity-50">empty</span>}
                  </pre>
                </div>

                {/* Separator */}
                <div className="px-2.5 py-1 bg-bg-active text-[10px] text-text-muted font-mono border-b border-border/40">
                  =======
                  <button
                    onClick={() => setChoice(region.index, 'both')}
                    className={`ml-2 px-2 py-0.5 rounded text-[10px] font-medium transition-colors ${choice === 'both' ? 'bg-accent/20 text-accent' : 'text-text-muted hover:bg-accent/10 hover:text-accent'}`}
                  >Accept Both</button>
                </div>

                {/* Incoming */}
                <div className={`${choice === 'incoming' || choice === 'both' ? 'bg-accent/8' : 'bg-bg-secondary'}`}>
                  <div className="px-2.5 pt-1 pb-0.5 flex items-center justify-between">
                    <span className="text-[10px] text-accent font-semibold uppercase tracking-wide">Incoming</span>
                    <div className="flex gap-1">
                      <button
                        onClick={() => setChoice(region.index, 'incoming')}
                        className={`px-2 py-0.5 rounded text-[10px] font-medium transition-colors ${choice === 'incoming' ? 'bg-accent/25 text-accent' : 'text-text-muted hover:bg-accent/15 hover:text-accent'}`}
                      >Accept</button>
                    </div>
                  </div>
                  <pre className="px-3 pb-1.5 text-[11px] font-mono text-accent/80 whitespace-pre-wrap overflow-x-auto">
                    {region.incomingLines.length > 0 ? region.incomingLines.join('\n') : <span className="italic opacity-50">empty</span>}
                  </pre>
                </div>
              </div>
            )
          })}

          {allResolved && (
            <button
              onClick={handleResolve}
              disabled={resolving}
              className="w-full flex items-center justify-center gap-1.5 py-2 text-[11px] font-semibold bg-green text-white rounded-lg hover:bg-green/80 disabled:opacity-40 transition-colors shadow-sm"
            >
              {resolving ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <AlertCircle className="w-3.5 h-3.5" />}
              Resolve &amp; Stage
            </button>
          )}
        </div>
      ) : (
        <div className="px-3 py-3 text-[11px] text-text-muted italic">No conflict markers found</div>
      )}
    </div>
  )
}

/* ═══════════════════════════════════════════════════
   Changed-file row (with inline diff + discard)
═══════════════════════════════════════════════════ */

function FileRow({ file, action, onAction, onDiscard, authFetch, onRefresh }) {
  const filename = file.path.split('/').pop()
  const dir = file.path.includes('/') ? file.path.slice(0, file.path.lastIndexOf('/')) : ''
  const [diffOpen, setDiffOpen] = useState(false)
  const isUnstaged = action === 'stage'
  const isConflicted = file.conflicted

  return (
    <div>
      <div
        className={`flex items-center px-3 py-1.5 hover:bg-bg-hover transition-colors group overflow-hidden cursor-pointer ${isConflicted ? 'bg-red/5' : ''}`}
        onClick={() => setDiffOpen((v) => !v)}
      >
        <FileBadge status={file.status} />
        <div className="flex-1 min-w-0 ml-2.5 overflow-hidden">
          <div className="flex items-baseline gap-1.5 min-w-0 overflow-hidden">
            <span className="text-[12px] text-text-primary font-medium truncate leading-tight">{filename}</span>
            {dir && <span className="text-[10px] text-text-muted truncate shrink">{dir}</span>}
          </div>
        </div>
        <div className="flex items-center gap-1 ml-2 shrink-0 opacity-0 group-hover:opacity-100 transition-all">
          {/* Diff toggle indicator */}
          <span className="w-4 h-4 flex items-center justify-center text-text-muted">
            {diffOpen
              ? <EyeOff className="w-3 h-3" />
              : <Eye className="w-3 h-3" />
            }
          </span>
          {/* Discard button (unstaged, non-conflicted only) */}
          {isUnstaged && !isConflicted && onDiscard && (
            <button
              onClick={(e) => { e.stopPropagation(); onDiscard(file.path) }}
              className="w-6 h-6 flex items-center justify-center rounded-md bg-bg-active hover:bg-red/15 text-text-muted hover:text-red transition-all"
              title="Discard changes"
            >
              <Trash2 className="w-3 h-3" />
            </button>
          )}
          {/* Stage / Unstage (not shown for conflicted files) */}
          {!isConflicted && (
            <button
              onClick={(e) => { e.stopPropagation(); onAction(file.path) }}
              className="w-6 h-6 flex items-center justify-center rounded-md bg-bg-active hover:bg-border-active text-text-muted hover:text-text-primary transition-all"
              title={action === 'stage' ? 'Stage file' : 'Unstage file'}
            >
              {action === 'stage' ? <Plus className="w-3 h-3" /> : <Minus className="w-3 h-3" />}
            </button>
          )}
        </div>
      </div>

      {/* Conflict resolver or inline diff */}
      {diffOpen && (
        isConflicted
          ? <ConflictResolver file={file} authFetch={authFetch} onRefresh={onRefresh} />
          : <FileDiffPanel file={file} staged={!isUnstaged} authFetch={authFetch} onRefresh={onRefresh} />
      )}
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
   Stash section
═══════════════════════════════════════════════════ */

function StashSection({ stashes, authFetch, onRefresh, onToast }) {
  const [open, setOpen] = useState(false)
  const [stashing, setStashing] = useState(false)
  const [popping, setPopping] = useState(null) // index being popped

  const handleStash = async () => {
    setStashing(true)
    try {
      const res = await authFetch('/api/git/stash', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: '' }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        onToast(d.error || 'Stash failed', 'error')
      } else {
        onToast('Changes stashed', 'success')
        onRefresh()
      }
    } catch (e) {
      onToast(e.message || 'Stash failed', 'error')
    }
    setStashing(false)
  }

  const handlePop = async (index) => {
    setPopping(index)
    try {
      const res = await authFetch('/api/git/stash/pop', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ index }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        onToast(d.error || 'Pop failed', 'error')
      } else {
        onToast('Stash applied', 'success')
        onRefresh()
      }
    } catch (e) {
      onToast(e.message || 'Pop failed', 'error')
    }
    setPopping(null)
  }

  return (
    <div className="border-t border-border/40">
      {/* Header */}
      <div className="flex items-center px-3 py-1.5 select-none">
        <button
          onClick={() => setOpen((v) => !v)}
          className="flex items-center gap-1.5 flex-1 min-w-0"
        >
          {open
            ? <ChevronDown className="w-3 h-3 text-text-muted shrink-0" />
            : <ChevronRight className="w-3 h-3 text-text-muted shrink-0" />
          }
          <Package className="w-3 h-3 text-text-muted shrink-0" />
          <span className="text-[10px] font-bold uppercase tracking-widest text-text-muted ml-0.5">Stash</span>
          {stashes.length > 0 && (
            <span className="ml-1 text-[10px] font-semibold text-text-muted opacity-60">{stashes.length}</span>
          )}
        </button>
        <button
          onClick={handleStash}
          disabled={stashing}
          className="text-[10px] text-text-muted hover:text-text-primary px-1.5 py-0.5 rounded hover:bg-bg-hover transition-colors font-medium shrink-0 disabled:opacity-40"
          title="Stash changes"
        >
          {stashing ? <RefreshCw className="w-3 h-3 animate-spin inline" /> : 'Stash'}
        </button>
      </div>

      {open && (
        <div className="py-0.5 border-t border-border/30">
          {stashes.length === 0 ? (
            <div className="px-3 py-2 text-[11px] text-text-muted italic">No stashes</div>
          ) : (
            stashes.map((s) => (
              <div key={s.index} className="flex items-center gap-2 px-3 py-1.5 hover:bg-bg-hover transition-colors group">
                <div className="flex-1 min-w-0">
                  <div className="text-[11px] text-text-secondary truncate">
                    {s.message || `stash@{${s.index}}`}
                  </div>
                  {s.date && (
                    <div className="text-[10px] text-text-muted">{s.date}</div>
                  )}
                </div>
                <button
                  onClick={() => handlePop(s.index)}
                  disabled={popping !== null}
                  className="opacity-0 group-hover:opacity-100 text-[10px] text-accent hover:text-accent bg-accent/10 hover:bg-accent/20 px-2 py-0.5 rounded transition-all font-medium shrink-0 disabled:opacity-40"
                >
                  {popping === s.index ? <RefreshCw className="w-3 h-3 animate-spin inline" /> : 'Pop'}
                </button>
              </div>
            ))
          )}
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
  const [stashes, setStashes]   = useState([])
  const [commitMsg, setCommitMsg] = useState('')
  const [section, setSection]   = useState('changes')
  const [loading, setLoading]   = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [remoteOp, setRemoteOp] = useState('') // 'push'|'pull'|'fetch'|''
  const [remoteMsg, setRemoteMsg] = useState('') // success/error message from remote op
  const [newBranch, setNewBranch] = useState('') // value for create-branch input
  const [showNewBranch, setShowNewBranch] = useState(false)
  const [toast, setToast]       = useState(null) // { message, type }
  // Remote add state
  const [showAddRemote, setShowAddRemote] = useState(false)
  const [newRemoteName, setNewRemoteName] = useState('')
  const [newRemoteUrl, setNewRemoteUrl] = useState('')
  const [addingRemote, setAddingRemote] = useState(false)

  const showToast = useCallback((message, type = 'error') => {
    setToast({ message, type })
  }, [])

  const refresh = useCallback(async (quiet = false) => {
    if (!visible) return
    if (!quiet) setRefreshing(true)
    try {
      const [sRes, lRes, bRes, rRes, stRes] = await Promise.all([
        authFetch('/api/git/status'),
        authFetch('/api/git/log'),
        authFetch('/api/git/branches'),
        authFetch('/api/git/remotes'),
        authFetch('/api/git/stash'),
      ])
      const [sData, lData, bData, rData, stData] = await Promise.all([
        sRes.json(), lRes.json(), bRes.json(), rRes.json(), stRes.json(),
      ])
      setStatus(sData)
      setLog(lData.entries || [])
      setBranches(bData.branches || [])
      setRemotes(rData.remotes || [])
      setStashes(stData.stashes || [])
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
  const handleDiscard = async (path) => {
    try {
      const res = await authFetch('/api/git/discard', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        showToast(d.error || 'Discard failed', 'error')
      } else {
        refresh(true)
      }
    } catch (e) {
      showToast(e.message || 'Discard failed', 'error')
    }
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
  const handleDeleteBranch = async (branch) => {
    if (!window.confirm(`Delete branch "${branch}"? (unmerged branches are kept)`)) return
    await authFetch('/api/git/branch/delete', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: branch, force: false }),
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

  const handleAddRemote = async (e) => {
    e.preventDefault()
    if (!newRemoteName.trim() || !newRemoteUrl.trim()) return
    setAddingRemote(true)
    try {
      const res = await authFetch('/api/git/remotes/add', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newRemoteName.trim(), url: newRemoteUrl.trim() }),
      })
      const data = await res.json()
      if (data.error) {
        showToast(data.error, 'error')
      } else {
        showToast('Remote added', 'success')
        setNewRemoteName('')
        setNewRemoteUrl('')
        setShowAddRemote(false)
        refresh(true)
      }
    } catch (err) {
      showToast(err.message || 'Failed to add remote', 'error')
    }
    setAddingRemote(false)
  }

  const handleRemoveRemote = async (name) => {
    try {
      const res = await authFetch('/api/git/remotes/remove', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      })
      const data = await res.json()
      if (data.error) {
        showToast(data.error, 'error')
      } else {
        showToast('Remote removed', 'success')
        refresh(true)
      }
    } catch (err) {
      showToast(err.message || 'Failed to remove remote', 'error')
    }
  }

  if (!visible) return null

  const conflicted = status.files?.filter((f) => f.conflicted) || []
  const staged     = status.files?.filter((f) => f.staged && !f.conflicted)  || []
  const unstaged   = status.files?.filter((f) => !f.staged && !f.conflicted) || []
  const totalChanges = staged.length + unstaged.length + conflicted.length

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

      {/* Toast */}
      {toast && (
        <Toast message={toast.message} type={toast.type} onDismiss={() => setToast(null)} />
      )}

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

            {/* Conflicted files */}
            {conflicted.length > 0 && (
              <SectionHeader
                label="Conflicts" count={conflicted.length} colorClass="text-red"
              >
                {conflicted.map((f) => (
                  <FileRow key={f.path} file={f} action="stage" onAction={handleStage} authFetch={authFetch} onRefresh={() => refresh(true)} />
                ))}
              </SectionHeader>
            )}

            {/* Staged files */}
            {staged.length > 0 && (
              <SectionHeader
                label="Staged" count={staged.length} colorClass="text-green"
                onUnstageAll={handleUnstageAll}
              >
                {staged.map((f) => (
                  <FileRow key={f.path} file={f} action="unstage" onAction={handleUnstage} authFetch={authFetch} onRefresh={() => refresh(true)} />
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
                  <FileRow key={f.path} file={f} action="stage" onAction={handleStage} onDiscard={handleDiscard} authFetch={authFetch} onRefresh={() => refresh(true)} />
                ))}
              </SectionHeader>
            )}

            {staged.length === 0 && unstaged.length === 0 && conflicted.length === 0 && (
              <div className="flex flex-col items-center justify-center py-12 text-text-muted px-6">
                <div className="w-10 h-10 rounded-xl bg-green/8 border border-green/15 flex items-center justify-center mb-3">
                  <Check className="w-5 h-5 text-green opacity-70" />
                </div>
                <span className="text-[12px] font-medium text-text-secondary">Working tree clean</span>
                <span className="text-[11px] mt-1">No changes to commit</span>
              </div>
            )}

            {/* Stash section */}
            <StashSection
              stashes={stashes}
              authFetch={authFetch}
              onRefresh={() => refresh(true)}
              onToast={showToast}
            />
          </div>
        )}

        {/* ── History (git graph) ── */}
        {section === 'graph' && (
          <div className="flex flex-col h-full min-h-0">
            <GitGraph entries={log} authFetch={authFetch} onCommitAction={handleCommitAction} />
          </div>
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
                <div
                  key={b.name}
                  role="button"
                  tabIndex={0}
                  onClick={() => !b.current && handleCheckout(b.name)}
                  onKeyDown={(e) => { if (!b.current && (e.key === 'Enter' || e.key === ' ')) handleCheckout(b.name) }}
                  className={`group w-full flex items-center gap-2.5 px-3 py-2 transition-colors text-left overflow-hidden ${
                    b.current
                      ? 'text-text-primary bg-accent/5 cursor-default'
                      : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary cursor-pointer'
                  }`}
                >
                  <GitBranch className={`w-3.5 h-3.5 shrink-0 ${b.current ? 'text-green' : 'text-text-muted'}`} />
                  <span className="truncate text-[12px] font-mono font-medium">{b.name}</span>
                  {b.current ? (
                    <span className="ml-auto shrink-0 flex items-center gap-1 text-[10px] text-green font-semibold">
                      <span className="w-1.5 h-1.5 rounded-full bg-green" />
                      current
                    </span>
                  ) : (
                    <button
                      onClick={(e) => { e.stopPropagation(); handleDeleteBranch(b.name) }}
                      title={`Delete branch ${b.name}`}
                      className="ml-auto shrink-0 p-1 rounded text-text-muted opacity-0 group-hover:opacity-100 hover:text-red hover:bg-red/10 transition-all">
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  )}
                </div>
              ))
            )}
          </div>
        )}

        {/* ── Remote operations ── */}
        {section === 'remotes' && (
          <div className="p-3 space-y-3">
            {/* Remote list with add/remove */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <div className="text-[10px] font-bold uppercase tracking-widest text-text-muted">Remotes</div>
                <button
                  onClick={() => setShowAddRemote((v) => !v)}
                  className={`flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium transition-colors ${showAddRemote ? 'bg-accent/15 text-accent' : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'}`}
                  title="Add remote"
                >
                  <Plus className="w-3 h-3" />
                  Add
                </button>
              </div>

              {/* Add remote form */}
              {showAddRemote && (
                <form onSubmit={handleAddRemote} className="mb-2 p-2.5 bg-bg-primary border border-border rounded-lg space-y-1.5 animate-fade-in">
                  <input
                    autoFocus
                    type="text"
                    value={newRemoteName}
                    onChange={(e) => setNewRemoteName(e.target.value)}
                    placeholder="Name (e.g. origin)"
                    className="w-full bg-bg-input border border-border rounded px-2.5 py-1 text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 transition-colors"
                  />
                  <input
                    type="text"
                    value={newRemoteUrl}
                    onChange={(e) => setNewRemoteUrl(e.target.value)}
                    placeholder="URL (https://... or git@...)"
                    className="w-full bg-bg-input border border-border rounded px-2.5 py-1 text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 transition-colors"
                  />
                  <div className="flex gap-1.5">
                    <button
                      type="submit"
                      disabled={addingRemote || !newRemoteName.trim() || !newRemoteUrl.trim()}
                      className="flex-1 py-1 text-[11px] bg-accent text-white rounded hover:bg-accent-hover disabled:opacity-40 transition-all font-medium"
                    >
                      {addingRemote ? 'Adding…' : 'Add Remote'}
                    </button>
                    <button
                      type="button"
                      onClick={() => { setShowAddRemote(false); setNewRemoteName(''); setNewRemoteUrl('') }}
                      className="px-2 py-1 text-[11px] border border-border rounded text-text-muted hover:bg-bg-hover transition-colors"
                    >
                      <X className="w-3 h-3" />
                    </button>
                  </div>
                </form>
              )}

              {remotes.length === 0 && !showAddRemote ? (
                <div className="flex flex-col items-center justify-center py-8 text-text-muted">
                  <AlertCircle className="w-5 h-5 mb-2 opacity-30" />
                  <span className="text-[12px]">No remotes configured</span>
                </div>
              ) : (
                remotes.map((r) => (
                  <div key={r.name} className="flex items-center gap-2 px-2 py-1.5 bg-bg-primary border border-border rounded-lg mb-1.5 group">
                    <span className="text-[12px] font-mono font-medium text-text-primary shrink-0">{r.name}</span>
                    <span className="text-[10px] text-text-muted truncate flex-1">{r.url}</span>
                    <button
                      onClick={() => handleRemoveRemote(r.name)}
                      className="opacity-0 group-hover:opacity-100 w-5 h-5 flex items-center justify-center rounded text-text-muted hover:text-red hover:bg-red/10 transition-all shrink-0"
                      title="Remove remote"
                    >
                      <Trash2 className="w-3 h-3" />
                    </button>
                  </div>
                ))
              )}
            </div>

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
