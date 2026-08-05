import { useState, useRef, type FormEvent, type KeyboardEvent } from 'react'
import { Send, ChevronUp, ChevronDown } from 'lucide-react'

interface KeyDef {
  label: string
  send?: string
  modifier?: boolean
}

const keys: KeyDef[] = [
  { label: 'Tab', send: '\t' },
  { label: 'Esc', send: '\x1b' },
  { label: 'Ctrl', modifier: true },
  { label: '↑', send: '\x1b[A' },
  { label: '↓', send: '\x1b[B' },
  { label: '←', send: '\x1b[D' },
  { label: '→', send: '\x1b[C' },
  { label: '~', send: '~' },
  { label: '/', send: '/' },
  { label: '-', send: '-' },
  { label: '|', send: '|' },
  { label: '$', send: '$' },
]

const ctrlKeys: KeyDef[] = [
  { label: 'C', send: '\x03' },
  { label: 'D', send: '\x04' },
  { label: 'Z', send: '\x1a' },
  { label: 'L', send: '\x0c' },
  { label: 'A', send: '\x01' },
  { label: 'E', send: '\x05' },
  { label: 'R', send: '\x12' },
  { label: 'W', send: '\x17' },
]

interface TerminalToolbarProps {
  onSend: (data: string) => void
}

export default function TerminalToolbar({ onSend }: TerminalToolbarProps) {
  const [ctrl, setCtrl] = useState(false)
  const [input, setInput] = useState('')
  const [history, setHistory] = useState<string[]>([])
  const [histIdx, setHistIdx] = useState(-1)
  const inputRef = useRef<HTMLInputElement>(null)

  const handleKey = (key: KeyDef) => {
    if (key.modifier) {
      setCtrl(c => !c)
      return
    }
    if (ctrl) {
      const ch = (key.send || '').toUpperCase()
      if (ch.length === 1 && ch >= 'A' && ch <= 'Z') {
        onSend(String.fromCharCode(ch.charCodeAt(0) - 64))
      } else {
        onSend(key.send || '')
      }
      setCtrl(false)
      return
    }
    onSend(key.send || '')
  }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!input) return
    onSend(input + '\n')
    setHistory(prev => [input, ...prev].slice(0, 50))
    setHistIdx(-1)
    setInput('')
  }

  const handleInputKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (histIdx < history.length - 1) {
        const next = histIdx + 1
        setHistIdx(next)
        setInput(history[next])
      }
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (histIdx > 0) {
        const next = histIdx - 1
        setHistIdx(next)
        setInput(history[next])
      } else if (histIdx === 0) {
        setHistIdx(-1)
        setInput('')
      }
    }
  }

  return (
    <div className="shrink-0 bg-bg-tertiary border-t border-border">
      {/* Command input bar */}
      <form onSubmit={handleSubmit} className="flex items-center gap-1.5 px-2 py-1.5 border-b border-border">
        <span className="text-accent text-xs font-mono shrink-0">$</span>
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleInputKey}
          placeholder="Type command..."
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="off"
          spellCheck="false"
          className="flex-1 bg-bg-primary border border-border rounded px-2 py-1.5 text-xs font-mono text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent min-w-0"
        />
        <button
          type="button"
          onClick={() => onSend('\x1b[A')}
          className="p-1.5 rounded bg-bg-secondary border border-border text-text-muted active:bg-bg-active shrink-0"
        >
          <ChevronUp className="w-3.5 h-3.5" />
        </button>
        <button
          type="button"
          onClick={() => onSend('\x1b[B')}
          className="p-1.5 rounded bg-bg-secondary border border-border text-text-muted active:bg-bg-active shrink-0"
        >
          <ChevronDown className="w-3.5 h-3.5" />
        </button>
        <button
          type="submit"
          className="flex items-center justify-center w-8 h-8 rounded-full bg-accent text-bg-primary active:opacity-70 transition-opacity shrink-0 shadow-sm"
        >
          <Send className="w-4 h-4" />
        </button>
      </form>

      {/* Special keys row */}
      <div className="flex items-center gap-1 px-2 py-1 overflow-x-auto">
        {ctrl ? (
          <>
            <button
              onClick={() => setCtrl(false)}
              className="px-2 py-1 text-[11px] font-bold rounded bg-accent text-bg-primary shrink-0"
            >
              Ctrl
            </button>
            {ctrlKeys.map((k) => (
              <button
                key={k.label}
                onClick={() => { onSend(k.send || ''); setCtrl(false) }}
                className="px-2.5 py-1 text-[11px] font-medium rounded bg-bg-secondary border border-border text-text-primary active:bg-bg-active shrink-0"
              >
                {k.label}
              </button>
            ))}
          </>
        ) : (
          keys.map((k) => (
            <button
              key={k.label}
              onClick={() => handleKey(k)}
              className={`px-2.5 py-1 text-[11px] font-medium rounded border shrink-0 ${
                k.modifier
                  ? 'bg-bg-secondary border-border text-text-secondary active:bg-accent active:text-bg-primary'
                  : 'bg-bg-secondary border-border text-text-primary active:bg-bg-active'
              }`}
            >
              {k.label}
            </button>
          ))
        )}
      </div>
    </div>
  )
}
