// DebugPanel — Run & Debug sidebar: launch, stepping controls, call stack,
// variables, and debug output, driven by the useDap hook.

import { Play, Square, Bug, ArrowDownToLine, ArrowUpFromLine, CornerDownRight, ChevronRight } from 'lucide-react'

const STATUS = {
  idle:       { label: 'Ready',     dot: 'bg-text-muted' },
  starting:   { label: 'Starting…', dot: 'bg-yellow animate-pulse' },
  running:    { label: 'Running',   dot: 'bg-green' },
  stopped:    { label: 'Paused',    dot: 'bg-yellow' },
  terminated: { label: 'Finished',  dot: 'bg-text-muted' },
}

const STEP_BTN = 'p-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-hover disabled:opacity-30 disabled:cursor-not-allowed transition-colors'

export default function DebugPanel({ dap, canDebug, targetName, targetLang, onStart, readOnly, hint }) {
  const { status, frames, scopes, output, stop, cont, stepOver, stepIn, stepOut } = dap
  const live = status === 'running' || status === 'stopped' || status === 'starting'
  const paused = status === 'stopped'
  const st = STATUS[status] || STATUS.idle

  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border shrink-0">
        <Bug className="w-3.5 h-3.5 text-text-muted" />
        <span className="text-[11px] font-semibold text-text-secondary uppercase tracking-wider">Run &amp; Debug</span>
        <span className="ml-auto flex items-center gap-1.5">
          <span className={`w-1.5 h-1.5 rounded-full ${st.dot}`} />
          <span className="text-[10px] text-text-muted">{st.label}</span>
        </span>
      </div>

      {/* Launch / controls */}
      <div className="px-3 py-2 border-b border-border shrink-0">
        {!live ? (
          <button
            onClick={onStart}
            disabled={readOnly || !canDebug}
            title={readOnly ? 'Read-only' : canDebug ? `Debug ${targetName}` : (hint || 'Open a debuggable file')}
            className="w-full flex items-center justify-center gap-1.5 py-1.5 bg-green text-white rounded-md text-[12px] font-semibold hover:opacity-90 disabled:opacity-40 disabled:cursor-not-allowed transition-opacity"
          >
            <Play className="w-3.5 h-3.5" /> Start Debugging
          </button>
        ) : (
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-0.5">
              <button onClick={cont} disabled={!paused} title="Continue (F5)" className={STEP_BTN}><Play className="w-3.5 h-3.5" /></button>
              <button onClick={stepOver} disabled={!paused} title="Step Over (F10)" className={STEP_BTN}><CornerDownRight className="w-3.5 h-3.5" /></button>
              <button onClick={stepIn} disabled={!paused} title="Step Into (F11)" className={STEP_BTN}><ArrowDownToLine className="w-3.5 h-3.5" /></button>
              <button onClick={stepOut} disabled={!paused} title="Step Out (Shift+F11)" className={STEP_BTN}><ArrowUpFromLine className="w-3.5 h-3.5" /></button>
            </div>
            <button onClick={stop} title="Stop"
              className="p-1.5 rounded-md text-red hover:bg-red/10 transition-colors">
              <Square className="w-3.5 h-3.5" />
            </button>
          </div>
        )}
        {!live && (
          <p className="text-[10px] text-text-muted mt-1.5 truncate">
            {canDebug ? <>Target: <span className="font-mono text-text-secondary">{targetName}</span> ({targetLang})</> : (hint || 'Open a Go or Python file to debug.')}
          </p>
        )}
      </div>

      <div className="flex-1 overflow-y-auto min-h-0">
        {/* Call stack */}
        {frames.length > 0 && (
          <Section title="Call Stack">
            {frames.map((f) => (
              <div key={f.id} className="flex items-baseline gap-2 px-3 py-1 text-[11px] hover:bg-bg-hover/50">
                <span className="text-text-primary truncate">{f.name}</span>
                <span className="ml-auto text-[10px] text-text-muted font-mono shrink-0">
                  {f.source?.name ? `${f.source.name}:${f.line}` : f.line}
                </span>
              </div>
            ))}
          </Section>
        )}

        {/* Variables */}
        {scopes.length > 0 && (
          <Section title="Variables">
            {scopes.map((s) => (
              <div key={s.name}>
                <div className="flex items-center gap-1 px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-text-muted">
                  <ChevronRight className="w-3 h-3" /> {s.name}
                </div>
                {s.variables.length === 0
                  ? <div className="pl-7 pr-3 py-0.5 text-[11px] text-text-muted italic">—</div>
                  : s.variables.map((v, i) => (
                    <div key={i} className="flex items-baseline gap-1.5 pl-7 pr-3 py-0.5 text-[11px] font-mono hover:bg-bg-hover/50">
                      <span className="text-cyan shrink-0">{v.name}</span>
                      {v.type && <span className="text-text-muted text-[9px] shrink-0">{v.type}</span>}
                      <span className="text-text-secondary truncate">{v.value}</span>
                    </div>
                  ))}
              </div>
            ))}
          </Section>
        )}

        {/* Debug console */}
        {output.length > 0 && (
          <Section title="Debug Console">
            <pre className="px-3 py-1 text-[11px] font-mono text-text-secondary whitespace-pre-wrap break-words leading-relaxed">{output.join('')}</pre>
          </Section>
        )}

        {!live && frames.length === 0 && output.length === 0 && (
          <div className="px-4 py-8 text-center text-text-muted select-none">
            <Bug className="w-6 h-6 mx-auto mb-2 opacity-40" />
            <p className="text-[11px] leading-relaxed">Set breakpoints in the gutter, then Start Debugging. Needs a debug adapter (e.g. <span className="font-mono">dlv</span>, <span className="font-mono">debugpy</span>) installed.</p>
          </div>
        )}
      </div>
    </div>
  )
}

function Section({ title, children }) {
  return (
    <div className="border-b border-border/40 py-1">
      <div className="px-3 py-1 text-[10px] font-bold uppercase tracking-wider text-text-muted">{title}</div>
      {children}
    </div>
  )
}
