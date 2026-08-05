// Pure helpers for terminal naming — a tab can be renamed manually, or name
// itself from a PTY OSC 0/1/2 title escape (as macOS/Linux terminals do).
// Kept side-effect free so the rules are unit-tested independently of the hook.

import type { TerminalSession } from '../types'

// normalizeTitle collapses runs of whitespace, trims, and caps the length so a
// noisy PTY title becomes a tidy tab label. Returns '' when nothing is usable.
export function normalizeTitle(title: string | null | undefined): string {
  const v = (title || '').replace(/\s+/g, ' ').trim()
  if (!v) return ''
  return v.length > 40 ? v.slice(0, 39) + '…' : v
}

// applyAutoName returns the terminals list with terminal `id` renamed to the
// PTY-provided title — unless the user manually named it (manual wins) or the
// name is already current, in which case the original array is returned so React
// can skip the re-render.
export function applyAutoName(terminals: TerminalSession[], id: number, title: string | null | undefined): TerminalSession[] {
  const short = normalizeTitle(title)
  if (!short) return terminals
  let changed = false
  const next = terminals.map((t) => {
    if (t.id === id && !t.manual && t.name !== short) {
      changed = true
      return { ...t, name: short }
    }
    return t
  })
  return changed ? next : terminals
}
