// breakpoints.js — a CodeMirror 6 extension: a clickable breakpoint gutter plus
// a current-execution-line highlight, driven from React via two StateEffects.
//
//   setBreakpointsEffect.of(number[])   replace the breakpoint lines (1-based)
//   setStopLineEffect.of(number|null)   set the paused/current line (1-based)
//
// Clicking the gutter toggles a breakpoint and calls onToggle(line, lines).

import { gutter, GutterMarker, Decoration, EditorView } from '@codemirror/view'
import { StateField, StateEffect } from '@codemirror/state'

export const setBreakpointsEffect = StateEffect.define()
export const setStopLineEffect = StateEffect.define()

const bpField = StateField.define({
  create: () => ({ lines: new Set(), stop: null }),
  update(value, tr) {
    let { lines, stop } = value
    for (const e of tr.effects) {
      if (e.is(setBreakpointsEffect)) lines = new Set(e.value)
      if (e.is(setStopLineEffect)) stop = e.value
    }
    return { lines, stop }
  },
})

class DotMarker extends GutterMarker {
  toDOM() {
    const s = document.createElement('span')
    s.className = 'cm-bp-dot'
    return s
  }
}
const dot = new DotMarker()

const stopLineDeco = EditorView.decorations.compute([bpField], (state) => {
  const { stop } = state.field(bpField)
  if (!stop || stop < 1 || stop > state.doc.lines) return Decoration.none
  const line = state.doc.line(stop)
  return Decoration.set([Decoration.line({ class: 'cm-debug-stop' }).range(line.from)])
})

const bpTheme = EditorView.baseTheme({
  '.cm-breakpoint-gutter': { width: '12px', minWidth: '12px' },
  '.cm-breakpoint-gutter .cm-gutterElement': { cursor: 'pointer' },
  '.cm-bp-dot': {
    display: 'inline-block', width: '8px', height: '8px', borderRadius: '50%',
    background: 'var(--c-red)', boxShadow: '0 0 0 1px color-mix(in srgb, var(--c-red) 40%, transparent)',
  },
  '.cm-breakpoint-gutter .cm-gutterElement:hover .cm-bp-empty': {
    display: 'inline-block', width: '8px', height: '8px', borderRadius: '50%',
    background: 'color-mix(in srgb, var(--c-red) 35%, transparent)',
  },
  '.cm-debug-stop': { backgroundColor: 'color-mix(in srgb, var(--c-yellow) 20%, transparent)' },
})

export function breakpointGutter(onToggle) {
  return [
    bpField,
    gutter({
      class: 'cm-breakpoint-gutter',
      lineMarker(view, line) {
        const ln = view.state.doc.lineAt(line.from).number
        return view.state.field(bpField).lines.has(ln) ? dot : null
      },
      initialSpacer: () => dot,
      domEventHandlers: {
        mousedown(view, line) {
          const ln = view.state.doc.lineAt(line.from).number
          const cur = view.state.field(bpField).lines
          const next = new Set(cur)
          if (next.has(ln)) next.delete(ln)
          else next.add(ln)
          const lines = [...next].sort((a, b) => a - b)
          view.dispatch({ effects: setBreakpointsEffect.of(lines) })
          onToggle?.(ln, lines)
          return true
        },
      },
    }),
    stopLineDeco,
    bpTheme,
  ]
}
