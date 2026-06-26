// RemoteCursors — renders other collaborators' live mouse pointers across the
// whole viewport. Positions arrive as viewport fractions (0..1) over the collab
// socket, so they line up regardless of each person's window size.

import { MousePointer2 } from 'lucide-react'

export default function RemoteCursors({ mice, roster }) {
  const byId = {}
  for (const m of roster) byId[m.id] = m
  const entries = Object.entries(mice || {}).filter(([id]) => byId[id])
  if (entries.length === 0) return null

  return (
    <div className="fixed inset-0 pointer-events-none z-[100] overflow-hidden">
      {entries.map(([id, pos]) => {
        const m = byId[id]
        const color = m.color || '#888888'
        return (
          <div
            key={id}
            className="absolute top-0 left-0 transition-transform duration-75 ease-linear will-change-transform"
            style={{ transform: `translate(${pos.x * 100}vw, ${pos.y * 100}vh)` }}
          >
            <MousePointer2 className="w-4 h-4 drop-shadow-sm" style={{ color, fill: color }} />
            <span
              className="absolute left-3.5 top-3 px-1.5 py-0.5 rounded-md text-[10px] font-medium text-white whitespace-nowrap shadow-md"
              style={{ backgroundColor: color }}
            >
              {m.username || 'anon'}
            </span>
          </div>
        )
      })}
    </div>
  )
}
