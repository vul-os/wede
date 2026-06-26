// PresenceRoster — colored avatar circles for everyone connected to the room.
// Each avatar shows the member's initial in their assigned color, with a tooltip
// of their name and (if known) the file they're viewing.

function initial(name) {
  const n = (name || 'anon').trim()
  return n ? n[0].toUpperCase() : '?'
}

export default function PresenceRoster({ roster }) {
  if (!roster || roster.length === 0) return null

  const shown = roster.slice(0, 6)
  const extra = roster.length - shown.length

  return (
    <div className="flex items-center -space-x-1.5" title="People in this project">
      {shown.map((m) => (
        <div
          key={m.id}
          className="w-6 h-6 rounded-full flex items-center justify-center text-[10px] font-semibold text-white ring-2 ring-bg-tertiary select-none"
          style={{ backgroundColor: m.color || '#888' }}
          title={m.file ? `${m.username || 'anon'} · ${m.file}` : (m.username || 'anon')}>
          {initial(m.username)}
        </div>
      ))}
      {extra > 0 && (
        <div
          className="w-6 h-6 rounded-full flex items-center justify-center text-[10px] font-semibold text-text-secondary bg-bg-active ring-2 ring-bg-tertiary select-none"
          title={`+${extra} more`}>
          +{extra}
        </div>
      )}
    </div>
  )
}
