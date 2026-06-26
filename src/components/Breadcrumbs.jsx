import { ChevronRight } from 'lucide-react'

// Breadcrumbs — a VS Code-style path bar showing the active file's location.
// Display-only: splits the workspace-relative path into segments, last = filename.
export default function Breadcrumbs({ path }) {
  if (!path) return null
  const segments = path.split('/').filter(Boolean)
  if (segments.length === 0) return null

  return (
    <div
      className="flex items-center gap-0.5 px-3 h-6 text-[11px] text-text-muted bg-bg-primary border-b border-border/50 shrink-0 overflow-x-auto whitespace-nowrap select-none"
      style={{ scrollbarWidth: 'none' }}
      title={path}>
      {segments.map((seg, i) => {
        const isLast = i === segments.length - 1
        return (
          <span key={`${i}-${seg}`} className="flex items-center gap-0.5 shrink-0">
            {i > 0 && <ChevronRight className="w-3 h-3 opacity-40" />}
            <span className={isLast ? 'text-text-secondary font-medium' : ''}>{seg}</span>
          </span>
        )
      })}
    </div>
  )
}
