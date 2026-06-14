import { useState } from 'react'
import { Lock, AlertTriangle, Eye, EyeOff } from 'lucide-react'
import Logo from './Logo'

export default function Login({ onLogin, error, locked, remaining }) {
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (locked || !password) return
    setLoading(true)
    await onLogin(password)
    setLoading(false)
    setPassword('')
  }

  return (
    <div className="min-h-screen bg-bg-base flex items-center justify-center p-4">
      <div className="w-full max-w-xs animate-fade-in">
        {/* Logo + wordmark */}
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-14 h-14 rounded-2xl bg-bg-elevated border border-border mb-4 shadow-lg shadow-shadow overflow-hidden">
            <Logo size={36} />
          </div>
          <h1 className="text-xl font-semibold text-text-primary tracking-tight">wede</h1>
          <p className="text-text-muted text-[13px] mt-0.5">Web Development Environment</p>
        </div>

        <form
          onSubmit={handleSubmit}
          className="bg-bg-elevated border border-border rounded-xl p-6 shadow-xl shadow-shadow"
        >
          {locked ? (
            <div className="flex flex-col items-center py-4 text-center">
              <div className="w-12 h-12 rounded-xl bg-red/10 border border-red/20 flex items-center justify-center mb-3">
                <AlertTriangle className="w-6 h-6 text-red" />
              </div>
              <h2 className="text-[15px] font-semibold text-red mb-2">Locked Out</h2>
              <p className="text-text-muted text-[13px]">
                Too many failed attempts.<br />Restart the server to unlock.
              </p>
            </div>
          ) : (
            <>
              <div className="mb-4">
                <label className="block text-[12px] font-semibold text-text-secondary mb-1.5 uppercase tracking-widest">
                  Password
                </label>
                <div className="relative">
                  <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-text-muted" />
                  <input
                    type={showPassword ? 'text' : 'password'}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="w-full bg-bg-input border border-border rounded-md pl-9 pr-9 py-2.5 text-[13px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent/60 focus:ring-1 focus:ring-accent/15 transition-colors"
                    placeholder="Enter password"
                    autoFocus
                    disabled={loading}
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-secondary transition-colors"
                    tabIndex={-1}
                  >
                    {showPassword ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                  </button>
                </div>
              </div>

              {error && (
                <div className="mb-4 px-3 py-2.5 bg-red/8 border border-red/20 rounded-md text-red text-[12px]">
                  {error}
                </div>
              )}

              <button
                type="submit"
                disabled={loading || !password}
                className="w-full bg-accent hover:bg-accent-hover text-white text-[13px] font-semibold py-2.5 rounded-md transition-all shadow-sm shadow-accent/25 disabled:opacity-40 disabled:cursor-not-allowed"
              >
                {loading ? 'Signing in…' : 'Sign In'}
              </button>

              {remaining !== undefined && (
                <p className="mt-3 text-center text-text-muted text-[11px]">
                  {remaining} attempt{remaining !== 1 ? 's' : ''} remaining
                </p>
              )}
            </>
          )}
        </form>
      </div>
    </div>
  )
}
