// TasksPanel — lists the owner's tasks from ~/.wede/tasks.json and runs them in a
// terminal. Read-only users can't run (terminals are editor-gated).

import { Play, ListChecks } from 'lucide-react'

export default function TasksPanel({ tasks = [], onRun, readOnly }) {
  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden">
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border shrink-0">
        <ListChecks className="w-3.5 h-3.5 text-text-muted" />
        <span className="text-[11px] font-semibold text-text-secondary uppercase tracking-wider">Tasks</span>
        <span className="ml-auto text-[10px] text-text-muted">{tasks.length}</span>
      </div>

      <div className="flex-1 overflow-y-auto min-h-0 py-1">
        {tasks.length === 0 ? (
          <div className="px-4 py-8 text-center text-text-muted select-none">
            <ListChecks className="w-6 h-6 mx-auto mb-2 opacity-40" />
            <p className="text-[12px] text-text-secondary font-medium">No tasks yet</p>
            <p className="text-[11px] mt-1 leading-relaxed">Define build/test/run commands in <span className="font-mono text-text-secondary">~/.wede/tasks.json</span></p>
          </div>
        ) : (
          tasks.map((t, i) => (
            <button
              key={i}
              onClick={() => !readOnly && onRun?.(t)}
              disabled={readOnly}
              title={readOnly ? 'Read-only — tasks run in a terminal' : `Run: ${t.command}`}
              className="w-full flex items-center gap-2.5 px-3 py-2 text-left hover:bg-bg-hover transition-colors group disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <span className="w-6 h-6 rounded-md bg-bg-tertiary group-hover:bg-accent/15 flex items-center justify-center shrink-0 transition-colors">
                <Play className="w-3 h-3 text-text-muted group-hover:text-accent transition-colors" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-[12px] text-text-primary font-medium truncate">{t.name}</span>
                <span className="block text-[10px] text-text-muted font-mono truncate">{t.cwd ? `${t.cwd}: ` : ''}{t.command}</span>
              </span>
            </button>
          ))
        )}
      </div>
    </div>
  )
}
