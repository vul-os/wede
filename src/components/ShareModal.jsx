import { useState, useEffect, useCallback } from 'react'
import { X, Copy, Check, Trash2, Users } from 'lucide-react'

/**
 * ShareModal — owner-only workspace invite panel.
 *
 * Props:
 *   authFetch  — the authenticated fetch helper from useAuth
 *   onClose    — called when the modal should be dismissed
 *
 * The modal is only rendered when role === 'owner' (enforced by the IDE
 * caller). It communicates with three endpoints:
 *   POST   /api/auth/tokens          mint a new invite link
 *   GET    /api/auth/tokens          list live tokens
 *   DELETE /api/auth/tokens/{id}     revoke a token
 */
export default function ShareModal({ authFetch, onClose }) {
  const [role, setRole] = useState('viewer')
  const [name, setName] = useState('')
  const [ttlHours, setTtlHours] = useState('')
  const [minting, setMinting] = useState(false)
  const [mintError, setMintError] = useState(null)
  const [generatedLink, setGeneratedLink] = useState(null)
  const [copied, setCopied] = useState(false)
  const [tokens, setTokens] = useState([])
  const [listLoading, setListLoading] = useState(false)
  const [revoking, setRevoking] = useState(null)

  const fetchTokens = useCallback(async () => {
    setListLoading(true)
    try {
      const res = await authFetch('/api/auth/tokens')
      const data = await res.json()
      setTokens(data.tokens || [])
    } catch {
      // network error — show empty list; don't block the user
    } finally {
      setListLoading(false)
    }
  }, [authFetch])

   
  useEffect(() => {
    fetchTokens()
  }, [fetchTokens])
   

  const handleMint = async (e) => {
    e.preventDefault()
    setMintError(null)
    setMinting(true)
    setGeneratedLink(null)
    setCopied(false)
    try {
      const res = await authFetch('/api/auth/tokens', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          role,
          username: name.trim(),
          ttlHours: ttlHours ? parseFloat(ttlHours) : 0,
        }),
      })
      const data = await res.json()
      if (!res.ok) {
        setMintError(data.error || 'Failed to create invite link')
        return
      }
      setGeneratedLink(`${window.location.origin}/?invite=${data.raw}`)
      await fetchTokens()
    } catch {
      setMintError('Network error — please try again')
    } finally {
      setMinting(false)
    }
  }

  const handleCopy = () => {
    if (!generatedLink) return
    navigator.clipboard.writeText(generatedLink).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }).catch(() => {})
  }

  const handleRevoke = async (id) => {
    setRevoking(id)
    try {
      await authFetch(`/api/auth/tokens/${id}`, { method: 'DELETE' })
      setTokens((prev) => prev.filter((t) => t.id !== id))
    } catch {
      // ignore — list will be stale; user can close+reopen
    } finally {
      setRevoking(null)
    }
  }

  // Close on backdrop click
  const handleBackdrop = (e) => {
    if (e.target === e.currentTarget) onClose()
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={handleBackdrop}
    >
      <div className="relative bg-bg-elevated border border-border rounded-xl shadow-2xl shadow-shadow-lg w-full max-w-md mx-4 overflow-hidden animate-fade-in">
        {/* ── Header ── */}
        <div className="flex items-center justify-between px-5 pt-4 pb-3 border-b border-border">
          <div className="flex items-center gap-2">
            <Users className="w-4 h-4 text-accent" />
            <h2 className="text-sm font-semibold text-text-primary">Share Workspace</h2>
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded-md text-text-muted hover:text-text-primary hover:bg-bg-hover transition-colors"
            title="Close"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="px-5 py-4 space-y-4 max-h-[80vh] overflow-y-auto">
          {/* ── Mint form ── */}
          <form onSubmit={handleMint} className="space-y-3">
            {/* Role picker */}
            <div className="flex gap-2">
              {['viewer', 'editor'].map((r) => (
                <button
                  key={r}
                  type="button"
                  onClick={() => setRole(r)}
                  className={`flex-1 py-1.5 rounded-md text-[12px] font-medium border transition-colors ${
                    role === r
                      ? 'bg-accent/15 text-accent border-accent/40'
                      : 'bg-bg-secondary text-text-secondary border-border hover:text-text-primary hover:border-border-strong'
                  }`}
                >
                  {r.charAt(0).toUpperCase() + r.slice(1)}
                </button>
              ))}
            </div>

            <p className="text-[11px] text-text-muted leading-relaxed">
              {role === 'viewer'
                ? 'Viewer — read-only: no terminal, no file writes, no git mutations.'
                : 'Editor — full access: terminal, file writes, git operations, and collab editing.'}
            </p>

            <input
              type="text"
              placeholder="Name (optional)"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={32}
              className="w-full bg-bg-input border border-border rounded-md px-3 py-1.5 text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 transition-colors"
            />

            <input
              type="number"
              placeholder="Expires in hours — leave blank for no expiry"
              value={ttlHours}
              onChange={(e) => setTtlHours(e.target.value)}
              min="0.1"
              step="any"
              className="w-full bg-bg-input border border-border rounded-md px-3 py-1.5 text-[12px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 transition-colors"
            />

            {mintError && (
              <p className="text-[11px] text-red">{mintError}</p>
            )}

            <button
              type="submit"
              disabled={minting}
              className="w-full py-2 rounded-md text-[12px] font-medium bg-accent text-white hover:bg-accent-hover disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              {minting ? 'Creating…' : 'Create Invite Link'}
            </button>
          </form>

          {/* ── Generated link ── */}
          {generatedLink && (
            <div className="p-3 bg-bg-secondary border border-border rounded-lg space-y-2">
              <p className="text-[10px] font-semibold text-text-muted uppercase tracking-wider">Invite link</p>
              <div className="flex items-center gap-2">
                <input
                  readOnly
                  value={generatedLink}
                  className="flex-1 min-w-0 bg-bg-input border border-border rounded px-2 py-1 text-[11px] text-text-primary font-mono focus:outline-none truncate"
                  onClick={(e) => e.target.select()}
                />
                <button
                  onClick={handleCopy}
                  className={`flex items-center gap-1 px-2.5 py-1 rounded-md text-[11px] font-medium border transition-colors shrink-0 ${
                    copied
                      ? 'bg-green/15 text-green border-green/30'
                      : 'bg-bg-hover text-text-secondary border-border hover:text-text-primary'
                  }`}
                >
                  {copied ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
                  {copied ? 'Copied' : 'Copy'}
                </button>
              </div>
              <p className="text-[10px] text-text-muted">
                Anyone with this link can join as <strong className="text-text-secondary">{role}</strong>.
                Share it privately.
              </p>
            </div>
          )}

          {/* ── Active invites ── */}
          <div>
            <p className="text-[10px] font-semibold text-text-muted uppercase tracking-wider mb-2">
              Active invites
            </p>
            {listLoading ? (
              <p className="text-[11px] text-text-muted">Loading…</p>
            ) : tokens.length === 0 ? (
              <p className="text-[11px] text-text-muted">No active invites.</p>
            ) : (
              <div className="space-y-1.5 max-h-52 overflow-y-auto">
                {tokens.map((t) => (
                  <div
                    key={t.id}
                    className="flex items-center justify-between px-3 py-2 bg-bg-secondary border border-border rounded-md"
                  >
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span
                          className={`text-[10px] font-bold uppercase tracking-wider ${
                            t.role === 'editor' ? 'text-yellow' : 'text-text-muted'
                          }`}
                        >
                          {t.role}
                        </span>
                        {t.username && (
                          <span className="text-[11px] text-text-secondary truncate">{t.username}</span>
                        )}
                      </div>
                      <div className="text-[10px] text-text-muted mt-0.5">
                        Created {new Date(t.createdAt).toLocaleDateString()}
                        {t.expiresAt && t.expiresAt !== '0001-01-01T00:00:00Z' && (
                          <> · Expires {new Date(t.expiresAt).toLocaleDateString()}</>
                        )}
                      </div>
                    </div>
                    <button
                      onClick={() => handleRevoke(t.id)}
                      disabled={revoking === t.id}
                      className="ml-2 p-1 rounded text-text-muted hover:text-red hover:bg-red/10 disabled:opacity-40 transition-colors shrink-0"
                      title="Revoke invite"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
