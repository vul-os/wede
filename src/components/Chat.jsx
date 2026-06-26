// Chat — live + persisted per-workspace chat panel.
//
// Props (all required; the integrator mounts <Chat> as a sidebar tab):
//   workspaceId  string — active workspace ID (from IDE state)
//   token        string — session token for WebSocket auth
//   username     string — display name for this user
//   color        string — hex colour for this user's avatar/messages
//
// The integrator wires the backend route:
//   GET /api/workspaces/{id}/chat -> workspace.Chat().HandleWS
//   (behind auth middleware, public-read OK)
// and mounts <Chat workspaceId={id} token={token} username={username} color={color} />
// as a sidebar tab inside IDE.jsx.

import { useEffect, useRef, useState, useCallback } from 'react'
import { useChat } from '../hooks/useChat'

function formatTime(iso) {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  } catch {
    return ''
  }
}

// UserMessage — a user-authored chat message with colored avatar.
function UserMessage({ msg }) {
  const name = msg.user || 'anon'
  const initial = name[0].toUpperCase()
  return (
    <div className="flex gap-2 px-3 py-1.5 hover:bg-bg-hover/50 group">
      <div
        className="w-5 h-5 rounded-full flex items-center justify-center text-[9px] font-bold text-white shrink-0 mt-0.5 select-none"
        style={{ backgroundColor: msg.color || '#888888' }}
        title={name}
      >
        {initial}
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-baseline gap-2 min-w-0">
          <span
            className="text-[11px] font-semibold shrink-0 leading-tight"
            style={{ color: msg.color || '#888888' }}
          >
            {name}
          </span>
          <span className="text-[10px] text-text-muted opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
            {formatTime(msg.time)}
          </span>
        </div>
        <p className="text-[12px] text-text-primary leading-relaxed break-words whitespace-pre-wrap mt-0.5">
          {msg.text}
        </p>
      </div>
    </div>
  )
}

// SystemGitMessage — a centered, subtle label for system/git activity events.
function SystemGitMessage({ msg }) {
  const isGit = msg.kind === 'git'
  return (
    <div className="flex items-center gap-2 px-3 py-1 my-0.5">
      <div className="flex-1 h-px bg-border/40" />
      <span
        className={`text-[10px] text-center shrink-0 max-w-[80%] truncate ${
          isGit ? 'text-accent/70' : 'text-text-muted'
        }`}
        title={msg.text}
      >
        {msg.text}
      </span>
      <div className="flex-1 h-px bg-border/40" />
    </div>
  )
}

function MessageRow({ msg }) {
  if (msg.kind === 'user') return <UserMessage msg={msg} />
  return <SystemGitMessage msg={msg} />
}

export default function Chat({ workspaceId, token, username, color }) {
  const { messages, sendMessage } = useChat(workspaceId, token, username, color)
  const [input, setInput] = useState('')
  const listRef = useRef(null)
  const bottomRef = useRef(null)
  const textareaRef = useRef(null)

  // Auto-scroll to bottom when new messages arrive, unless the user has scrolled up.
  useEffect(() => {
    const el = listRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80
    if (atBottom) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [messages])

  const handleSend = useCallback(() => {
    const text = input.trim()
    if (!text) return
    sendMessage(text)
    setInput('')
    // Reset textarea height
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }
  }, [input, sendMessage])

  const handleKey = useCallback((e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }, [handleSend])

  const handleInput = useCallback((e) => {
    setInput(e.target.value)
    // Auto-grow textarea up to 96 px.
    e.target.style.height = 'auto'
    e.target.style.height = Math.min(e.target.scrollHeight, 96) + 'px'
  }, [])

  return (
    <div className="h-full flex flex-col bg-bg-secondary overflow-hidden">

      {/* ── Header ── */}
      <div className="px-3 py-2 border-b border-border shrink-0 flex items-center gap-2">
        <span className="text-[11px] font-semibold text-text-secondary uppercase tracking-wider">
          Chat
        </span>
        {messages.length > 0 && (
          <span className="text-[10px] text-text-muted">
            {messages.length} message{messages.length !== 1 ? 's' : ''}
          </span>
        )}
      </div>

      {/* ── Message list ── */}
      <div
        ref={listRef}
        className="flex-1 overflow-y-auto overflow-x-hidden min-h-0 py-1"
      >
        {messages.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full py-12 px-4 text-center select-none">
            <div className="w-10 h-10 rounded-xl bg-bg-hover flex items-center justify-center mb-3">
              <span className="text-lg" role="img" aria-label="chat">💬</span>
            </div>
            <p className="text-[12px] font-medium text-text-secondary">No messages yet</p>
            <p className="text-[11px] text-text-muted mt-1">
              Say hello to your teammates
            </p>
          </div>
        ) : (
          messages.map((msg, i) => (
            <MessageRow key={msg.id || i} msg={msg} />
          ))
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
