import { useEffect, useRef, useState, useCallback } from 'react'
import {
  EditorView, keymap, lineNumbers, highlightActiveLineGutter,
  highlightActiveLine, drawSelection, highlightSpecialChars,
  rectangularSelection, crosshairCursor,
} from '@codemirror/view'
import { EditorState, Compartment } from '@codemirror/state'
import { defaultKeymap, indentWithTab, history, historyKeymap } from '@codemirror/commands'
import {
  syntaxHighlighting, defaultHighlightStyle, bracketMatching,
  foldGutter, indentOnInput,
} from '@codemirror/language'
import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete'
import { searchKeymap, highlightSelectionMatches } from '@codemirror/search'
import { oneDark } from '@codemirror/theme-one-dark'
import { showMinimap } from '@replit/codemirror-minimap'
import * as Y from 'yjs'
import { yCollab } from 'y-codemirror.next'
import { useTheme } from '../hooks/useTheme'
import { Code } from 'lucide-react'

import { javascript } from '@codemirror/lang-javascript'
import { html } from '@codemirror/lang-html'
import { css } from '@codemirror/lang-css'
import { json } from '@codemirror/lang-json'
import { python } from '@codemirror/lang-python'
import { go } from '@codemirror/lang-go'
import { markdown } from '@codemirror/lang-markdown'
import { xml } from '@codemirror/lang-xml'
import { sql } from '@codemirror/lang-sql'
import { rust } from '@codemirror/lang-rust'
import { cpp } from '@codemirror/lang-cpp'
import { java } from '@codemirror/lang-java'
import { php } from '@codemirror/lang-php'

const langMap = {
  js: () => javascript(), jsx: () => javascript({ jsx: true }),
  ts: () => javascript({ typescript: true }), tsx: () => javascript({ jsx: true, typescript: true }),
  html: () => html(), htm: () => html(), css: () => css(), json: () => json(),
  py: () => python(), go: () => go(), md: () => markdown(),
  xml: () => xml(), svg: () => xml(), sql: () => sql(),
  rs: () => rust(), c: () => cpp(), cpp: () => cpp(), h: () => cpp(),
  java: () => java(), php: () => php(),
}

function getLang(filename) {
  const ext = filename.split('.').pop().toLowerCase()
  return langMap[ext]?.() || []
}

// Build a theme extension from editor settings.
function makeEditorTheme(settings, isDark) {
  const fontSize = `${settings.fontSize || 13}px`
  const fontFamily = '"JetBrains Mono", "Fira Code", "Cascadia Code", monospace'

  const base = EditorView.theme({
    '&': {
      backgroundColor: 'var(--c-bg-primary)',
      color: 'var(--c-text-primary)',
      fontSize,
      fontFamily,
    },
    '.cm-gutters': {
      backgroundColor: 'var(--c-bg-secondary)',
      color: 'var(--c-text-muted)',
      borderRight: '1px solid var(--c-border)',
      fontFamily,
    },
    '.cm-activeLineGutter': { backgroundColor: 'var(--c-bg-hover)', color: 'var(--c-text-primary)' },
    '.cm-activeLine': { backgroundColor: 'var(--c-accent-glow)' },
    '.cm-cursor': { borderLeftColor: 'var(--c-accent)' },
    '.cm-content': { fontFamily, fontSize },
    // Multi-cursor: secondary cursors are slightly dimmer.
    '.cm-cursor-secondary': { borderLeftColor: 'var(--c-accent)', opacity: '0.6' },
  }, { dark: isDark })

  return base
}

// Build the minimap facet value.  Returns null when disabled (minimap hidden).
function makeMinimapConfig(enabled) {
  if (!enabled) return null
  return {
    create() {
      const dom = document.createElement('div')
      return { dom }
    },
    displayText: 'blocks',
    showOverlay: 'always',
  }
}

// GoToLine overlay widget — rendered inside the editor container.
// Receives a ref object (not .current) to avoid the react-hooks/refs lint rule.
function GoToLineWidget({ viewRef, onClose }) {
  const handleKeyDown = useCallback((e) => {
    const view = viewRef.current
    if (e.key === 'Enter') {
      const n = parseInt(e.target.value, 10)
      if (!isNaN(n) && view) {
        const line = Math.max(1, Math.min(n, view.state.doc.lines))
        const pos = view.state.doc.line(line).from
        view.dispatch({
          selection: { anchor: pos },
          effects: EditorView.scrollIntoView(pos, { y: 'center' }),
        })
        view.focus()
      }
      onClose()
    }
    if (e.key === 'Escape') {
      onClose()
      view?.focus()
    }
  }, [viewRef, onClose])

  return (
    <div className="absolute top-2 right-4 z-20 flex items-center gap-2 bg-bg-elevated border border-border rounded-lg shadow-xl px-3 py-2">
      <span className="text-[11px] text-text-muted">Go to line:</span>
      <input
        autoFocus
        type="number"
        min="1"
        className="w-16 bg-bg-input border border-border rounded px-2 py-0.5 text-[12px] text-text-primary focus:outline-none focus:border-accent/60"
        onKeyDown={handleKeyDown}
      />
    </div>
  )
}

export default function Editor({ file, content, onChange, onSave, onCursorChange, settings = {}, lspExtension = null, onRegisterActions, collab = null, editable = true }) {
  const containerRef = useRef(null)
  const viewRef = useRef(null)
  const onChangeRef = useRef(onChange)
  const onSaveRef = useRef(onSave)
  const onCursorRef = useRef(onCursorChange)

  const [showGoToLine, setShowGoToLine] = useState(false)

  // Compartments for live reconfiguration without destroying the editor.
  const themeCompRef   = useRef(new Compartment())
  const wrapCompRef    = useRef(new Compartment())
  const tabCompRef     = useRef(new Compartment())
  const minimapCompRef = useRef(new Compartment())
  const lspCompRef     = useRef(new Compartment())
  const editCompRef    = useRef(new Compartment())

  const { isDark } = useTheme()

  // Keep callback refs in sync without triggering re-renders (updated in an
  // effect rather than directly in render to satisfy react-hooks/refs).
  useEffect(() => {
    onChangeRef.current = onChange
    onSaveRef.current = onSave
    onCursorRef.current = onCursorChange
  })

  // Register editor actions (goToLine) with the parent IDE.
  useEffect(() => {
    onRegisterActions?.({ goToLine: () => setShowGoToLine(true) })
  }, [onRegisterActions])

  // Rebuild editor when file changes (new language, new content).
  useEffect(() => {
    if (!containerRef.current) return

    const themeComp   = themeCompRef.current
    const wrapComp    = wrapCompRef.current
    const tabComp     = tabCompRef.current
    const minimapComp = minimapCompRef.current
    const lspComp     = lspCompRef.current
    const editComp    = editCompRef.current

    const minimapEnabled = settings.minimap ?? false

    // Collaboration: when a synced Y.Text + awareness are provided, yCollab owns
    // the document text (binds CodeMirror <-> Y.Text and renders remote cursors).
    // The doc is seeded from the Y.Text (empty until the provider syncs, then the
    // server's disk-backed content); we must NOT also seed from the `content`
    // prop or the editor would double-insert.
    const yt = collab?.ytext
    const aw = collab?.awareness
    const collabOn = !!(yt && aw)
    const collabExt = collabOn
      ? [yCollab(yt, aw, { undoManager: new Y.UndoManager(yt) })]
      : []

    const state = EditorState.create({
      doc: collabOn ? yt.toString() : (content || ''),
      extensions: [
        ...collabExt,
        lineNumbers(),
        highlightActiveLineGutter(),
        highlightActiveLine(),
        highlightSpecialChars(),
        drawSelection(),
        bracketMatching(),
        closeBrackets(),
        indentOnInput(),
        foldGutter(),
        highlightSelectionMatches(),
        history(),
        // Multi-cursor support: Alt+Click adds a cursor; Alt+drag selects
        // a rectangular region. crosshairCursor shows a crosshair when Alt
        // is held, giving users a visual cue that multi-cursor is active.
        rectangularSelection(),
        crosshairCursor(),
        themeComp.of([isDark ? oneDark : [], makeEditorTheme(settings, isDark)]),
        wrapComp.of(settings.wordWrap ? EditorView.lineWrapping : []),
        tabComp.of(EditorState.tabSize.of(settings.tabWidth || 2)),
        // Minimap compartment — reconfigured live when settings.minimap changes.
        minimapComp.of(showMinimap.of(makeMinimapConfig(minimapEnabled))),
        // LSP compartment — reconfigured when the file or lsp setting changes.
        lspComp.of(lspExtension ? lspExtension : []),
        // Read-only compartment — viewer role gets EditorState.readOnly + EditorView.editable(false).
        editComp.of(editable ? [] : [EditorState.readOnly.of(true), EditorView.editable.of(false)]),
        syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        getLang(file?.name || ''),
        keymap.of([
          ...closeBracketsKeymap, ...defaultKeymap,
          ...searchKeymap, ...historyKeymap, indentWithTab,
        ]),
        keymap.of([
          // When collab owns the doc, the backend CRDT write-back persists edits;
          // a manual REST save would fight it, so Mod-s is a no-op here.
          { key: 'Mod-s', run: () => { if (!collabOn) onSaveRef.current?.(); return true } },
          { key: 'Ctrl-g', run: () => { setShowGoToLine(true); return true } },
        ]),
        EditorView.updateListener.of((update) => {
          // Skip onChange under collab so IDE never marks the tab modified or
          // triggers its debounced REST auto-save (the doc WS handles sync+persist).
          if (!collabOn && update.docChanged) onChangeRef.current?.(update.state.doc.toString())
          if (update.selectionSet || update.docChanged) {
            const pos = update.state.selection.main.head
            const line = update.state.doc.lineAt(pos)
            onCursorRef.current?.({ line: line.number, col: pos - line.from + 1 })
          }
        }),
        EditorView.theme({
          '&': { height: '100%' },
          '.cm-scroller': { overflow: 'auto', fontFamily: '"JetBrains Mono", "Fira Code", monospace' },
        }),
      ],
    })

    const view = new EditorView({ state, parent: containerRef.current })
    viewRef.current = view
    return () => view.destroy()
  }, [file?.path, collab?.ytext]) // eslint-disable-line react-hooks/exhaustive-deps

  // Live theme switch (dark ↔ light).
  useEffect(() => {
    if (!viewRef.current) return
    viewRef.current.dispatch({
      effects: themeCompRef.current.reconfigure([isDark ? oneDark : [], makeEditorTheme(settings, isDark)]),
    })
  }, [isDark]) // eslint-disable-line react-hooks/exhaustive-deps

  // Live settings: word wrap.
  useEffect(() => {
    if (!viewRef.current) return
    viewRef.current.dispatch({
      effects: wrapCompRef.current.reconfigure(settings.wordWrap ? EditorView.lineWrapping : []),
    })
  }, [settings.wordWrap])

  // Live settings: tab width.
  useEffect(() => {
    if (!viewRef.current) return
    viewRef.current.dispatch({
      effects: tabCompRef.current.reconfigure(EditorState.tabSize.of(settings.tabWidth || 2)),
    })
  }, [settings.tabWidth])

  // Live settings: font size — rebuild the theme compartment.
  useEffect(() => {
    if (!viewRef.current) return
    viewRef.current.dispatch({
      effects: themeCompRef.current.reconfigure([isDark ? oneDark : [], makeEditorTheme(settings, isDark)]),
    })
  }, [settings.fontSize]) // eslint-disable-line react-hooks/exhaustive-deps

  // Live settings: minimap toggle.
  useEffect(() => {
    if (!viewRef.current) return
    const minimapEnabled = settings.minimap ?? false
    viewRef.current.dispatch({
      effects: minimapCompRef.current.reconfigure(showMinimap.of(makeMinimapConfig(minimapEnabled))),
    })
  }, [settings.minimap])

  // Live LSP extension swap — fires when file changes or lsp toggle changes.
  useEffect(() => {
    if (!viewRef.current) return
    viewRef.current.dispatch({
      effects: lspCompRef.current.reconfigure(lspExtension ? lspExtension : []),
    })
  }, [lspExtension])

  // Live editable toggle — viewer role makes the editor read-only without
  // destroying and rebuilding it.
  useEffect(() => {
    if (!viewRef.current) return
    viewRef.current.dispatch({
      effects: editCompRef.current.reconfigure(
        editable ? [] : [EditorState.readOnly.of(true), EditorView.editable.of(false)]
      ),
    })
  }, [editable])

  // Sync external content changes (e.g. auto-save feedback or file reload).
  // Disabled under collab: yCollab owns the document, so pushing the `content`
  // prop here would clobber the synced text.
  useEffect(() => {
    if (collab?.ytext) return
    const view = viewRef.current
    if (!view) return
    const current = view.state.doc.toString()
    if (content !== undefined && content !== current) {
      view.dispatch({ changes: { from: 0, to: current.length, insert: content || '' } })
    }
  }, [content, collab?.ytext])

  // Scroll to a specific line (used by search result navigation).
  useEffect(() => {
    if (!viewRef.current || !file?.targetLine) return
    const view = viewRef.current
    const line = Math.max(1, Math.min(file.targetLine, view.state.doc.lines))
    const pos = view.state.doc.line(line).from
    view.dispatch({
      selection: { anchor: pos },
      effects: EditorView.scrollIntoView(pos, { y: 'center' }),
    })
  }, [file?.targetLine])

  if (!file) {
    return (
      <div className="h-full flex items-center justify-center bg-bg-primary">
        <div className="text-center text-text-muted animate-fade-in">
          <Code className="w-12 h-12 mx-auto mb-3 opacity-20" />
          <p className="text-sm">No file open</p>
          <p className="text-xs mt-1">Select a file from the explorer</p>
        </div>
      </div>
    )
  }

  return (
    <div className="relative h-full overflow-hidden">
      <div ref={containerRef} className="h-full overflow-hidden" />
      {showGoToLine && (
        <GoToLineWidget
          viewRef={viewRef}
          onClose={() => setShowGoToLine(false)}
        />
      )}
    </div>
  )
}
