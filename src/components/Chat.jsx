// Chat — live + persisted per-workspace chat panel.
//
// Props (all required; the integrator mounts <Chat> as a sidebar tab):
//   workspaceId    string — active workspace ID (from IDE state)
//   workspaceName  string — active workspace display name (for the header)
//   token          string — session token for WebSocket auth
//   username       string — display name for this user
//   color          string — hex colour for this user's avatar/messages
//
// The integrator wires the backend route:
//   GET /api/workspaces/{id}/chat -> workspace.Chat().HandleWS
//   (behind auth middleware, public-read OK)

import { useEffect, useRef, useState, useCallback, useMemo } from 'react'
import { GitCommit, MessageSquare, Boxes } from 'lucide-react'
import { useChat } from '../hooks/useChat'

function formatTime(iso) {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  } catch {
    return ''
  }
}

// classifyGit splits the backend's git-activity text into a clean shape.
// The backend posts three kinds of [git] lines:
//   "📦 committed <hash>: <subject>"      — meaningful, kept
//   "✏️ N uncommitted change(s)"          — per-edit churn, noise
//   "✅ working tree clean"               — churn
// We surface commits nicely and treat the rest as low-signal "churn" that is
// hidden unless the user opts into full git activity.
function classifyGit(text) {
  const commit = /^📦\s*committed\s+([0-9a-f]+):\s*([\s\S]*)$/i.exec(text || '')
  if (commit) return { type: 'commit', hash: commit[1], subject: commit[2].trim() }
  return { type: 'churn', text: text || '' }
}

// CommitRow — a compact, clean git commit entry (not a full-width divider).
function CommitRow({ hash, subject }) {
  return (
    <div className="flex items-center gap-2 px-3 py-1 mx-2 my-0.5 rounded-md bg-bg-hover/40">
      <GitCommit className="w-3.5 h-3.5 text-accent shrink-0" />
      <code className="text-[10px] font-mono px-1 py-px rounded bg-accent/10 text-accent shrink-0">{hash}</code>
      <span className="text-[11px] text-text-secondary truncate" title={subject}>{subject}</span>
    </div>
  )
}

// ChurnRow — a very subtle single line for the noisy status events (only shown
// when "all git activity" is enabled).
function ChurnRow({ text }) {
  return (
    <div className="px-3 py-0.5 text-[10px] text-text-muted/60 text-center truncate select-none" title={text}>{text}</div>
  )
}

// UserMessage — a user-authored chat message. When `grouped` it omits the
// avatar/name header (a continuation of the same author's previous message).
function UserMessage({ msg, grouped }) {
  const name = msg.user || 'anon'
  const initial = name[0].toUpperCase()
  if (grouped) {
    return (
      <div className="flex gap-2 px-3 py-0.5 hover:bg-bg-hover/50 group">
        <div className="w-5 shrink-0" />
        <p className="text-[12px] text-text-primary leading-relaxed break-words whitespace-pre-wrap flex-1 min-w-0">{msg.text}</p>
      </div>
    )
  }
  return (
    <div className="flex gap-2 px-3 pt-2 pb-0.5 hover:bg-bg-hover/50 group">
      <div
        className="w-5 h-5 rounded-full flex items-center justify-center text-[9px] font-bold text-white shrink-0 mt-0.5 select-none"
        style={{ backgroundColor: msg.color || '#888888' }}
        title={name}
      >
        {initial}
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-baseline gap-2 min-w-0">
          <span className="text-[11px] font-semibold shrink-0 leading-tight" style={{ color: msg.color || '#888888' }}>{name}</span>
          <span className="text-[10px] text-text-muted opacity-0 group-hover:opacity-100 transition-opacity shrink-0">{formatTime(msg.time)}</span>
        </div>
        <p className="text-[12px] text-text-primary leading-relaxed break-words whitespace-pre-wrap mt-0.5">{msg.text}</p>
      </div>
    </div>
  )
}

export default function Chat({ workspaceId, workspaceName, token, username, color }) {
  const [channel, setChannel] = useState('public')
  const [showAllGit, setShowAllGit] = useState(false) // include churn (uncommitted/clean) too
  const { messages, sendMessage } = useChat(workspaceId, token, username, color, channel)
  const [input, setInput] = useState('')
  const listRef = useRef(null)
  const bottomRef = useRef(null)
  const textareaRef = useRef(null)

  // Build the display list: classify git events, drop churn unless opted in, and
  // mark grouped (continuation) user messages. This is what tames the "wall of
  // git spam" — commits stay, per-edit churn is hidden by default.
  const rows = useMemo(() => {
    const out = []
    let prevUser = null
    let prevAuthorTime = 0
    for (const msg of messages) {
      if (msg.kind === 'user') {
        const t = msg.time ? Date.parse(msg.time) : 0
        const grouped = prevUser === (msg.user || 'anon') && t - prevAuthorTime < 5 * 60 * 1000
        out.push({ ...msg, _row: 'user', _grouped: grouped })
        prevUser = msg.user || 'anon'
        prevAuthorTime = t || prevAuthorTime
        continue
      }
      // reset grouping across a non-user event
      prevUser = null
      if (msg.kind === 'git') {
        const g = classifyGit(msg.text)
        if (g.type === 'commit') out.push({ ...msg, _row: 'commit', hash: g.hash, subject: g.subject })
        else if (showAllGit) out.push({ ...msg, _row: 'churn' })
        continue
      }
      // other system messages
      if (showAllGit) out.push({ ...msg, _row: 'churn' })
    }
    return out
  }, [messages, showAllGit])

  const hiddenGitCount = useMemo(
    () => (showAllGit ? 0 : messages.filter((m) => m.kind === 'git' && classifyGit(m.text).type === 'churn').length),
    [messages, showAllGit]
  )

  useEffect(() => {
    const el = listRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80
    if (atBottom) bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [rows])

  const handleSend = useCallback(() => {
    const text = input.trim()
    if (!text) return
    sendMessage(text)
    setInput('')
    if (textareaRef.current) textareaRef.current.style.height = 'auto'
  }, [input, sendMessage])

  const handleKey = useCallback((e) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend() }
  }, [handleSend])

  const handleInput = useCallback((e) => {
    setInput(e.target.value)
    e.target.style.height = 'auto'
    e.target.style.height = Math.min(e.target.scrollHeight, 96) + 'px'
  }, [])

  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden">

      {/* ── Header: channel toggle + workspace + git-activity toggle ── */}
      <div className="px-3 py-2 border-b border-border shrink-0 space-y-1.5">
        <div className="flex items-center justify-between gap-2">
          <div className="inline-flex rounded-md border border-border overflow-hidden shrink-0">
            {['public', 'private'].map((ch) => (
              <button
                key={ch}
                onClick={() => setChannel(ch)}
                className={`px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider transition-colors ${
                  channel === ch ? 'bg-accent text-white' : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'
                }`}
              >
                {ch}
              </button>
            ))}
          </div>
          <button
            onClick={() => setShowAllGit((v) => !v)}
            title={showAllGit ? 'Hide git status churn (keep commits)' : 'Show all git activity'}
            className={`flex items-center gap-1 px-1.5 py-1 rounded text-[10px] font-medium transition-colors ${
              showAllGit ? 'text-accent bg-accent/10' : 'text-text-muted hover:text-text-primary hover:bg-bg-hover'
            }`}
          >
            <GitCommit className="w-3.5 h-3.5" />
            git
          </button>
        </div>
        {workspaceName && (
          <div className="flex items-center gap-1 text-[10px] text-text-muted truncate" title={`Chat for workspace "${workspaceName}" — ${channel === 'public' ? '.wede/chat.md' : '.wede/private/ (gitignored)'}`}>
            <Boxes className="w-3 h-3 text-accent/60 shrink-0" />
            <span className="font-medium text-text-secondary truncate">{workspaceName}</span>
            <span className="text-text-muted/70 shrink-0">· {channel === 'public' ? '.wede/chat.md' : 'private · gitignored'}</span>
          </div>
        )}
      </div>

      {/* ── Message list ── */}
      <div ref={listRef} className="flex-1 overflow-y-auto overflow-x-hidden min-h-0 py-1">
        {rows.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full py-12 px-4 text-center select-none">
            <div className="w-10 h-10 rounded-xl bg-bg-hover flex items-center justify-center mb-3">
              <MessageSquare className="w-5 h-5 text-text-muted" />
            </div>
            <p className="text-[12px] font-medium text-text-secondary">
              {channel === 'public' ? 'No messages yet' : 'No private messages yet'}
            </p>
            <p className="text-[11px] text-text-muted mt-1 max-w-[220px]">
              {channel === 'public'
                ? 'Public chat is committed to .wede/chat.md so the repo and LLMs can read it'
                : 'Private chat stays local in .wede/private/ (gitignored)'}
            </p>
            {hiddenGitCount > 0 && (
              <button onClick={() => setShowAllGit(true)} className="mt-3 text-[11px] text-accent hover:underline">
                Show {hiddenGitCount} git activity event{hiddenGitCount !== 1 ? 's' : ''}
              </button>
            )}
          </div>
        ) : (
          <>
            {rows.map((row, i) => {
              if (row._row === 'user') return <UserMessage key={row.id || i} msg={row} grouped={row._grouped} />
              if (row._row === 'commit') return <CommitRow key={row.id || i} hash={row.hash} subject={row.subject} />
              return <ChurnRow key={row.id || i} text={row.text} />
            })}
            {hiddenGitCount > 0 && (
              <div className="px-3 py-1.5 text-center">
                <button onClick={() => setShowAllGit(true)} className="text-[10px] text-text-muted hover:text-accent transition-colors">
                  + {hiddenGitCount} hidden git status event{hiddenGitCount !== 1 ? 's' : ''}
                </button>
              </div>
            )}
          </>
        )}
        <div ref={bottomRef} />
      </div>

      {/* ── Input ── */}
      <div className="shrink-0 border-t border-border px-3 py-2">
        <div className="flex gap-2 items-end">
          <textarea
            ref={textareaRef}
            value={input}
            onChange={handleInput}
            onKeyDown={handleKey}
            placeholder="Message… (Enter to send, Shift+Enter for newline)"
            rows={1}
            className="flex-1 bg-bg-input border border-border rounded-md px-3 py-2 text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 focus:ring-1 focus:ring-accent/20 resize-none leading-relaxed transition-colors"
            style={{ minHeight: 36, maxHeight: 96 }}
          />
          <button
            onClick={handleSend}
            disabled={!input.trim()}
            className="px-3 py-2 bg-accent text-white rounded-md text-[11px] font-semibold hover:bg-accent-hover disabled:opacity-30 disabled:cursor-not-allowed transition-all shrink-0"
          >
            Send
          </button>
        </div>
      </div>
    </div>
  )
}
