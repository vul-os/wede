import { Moon, Sun } from 'lucide-react'
import Logo from './Logo'

export default function ThemePicker({ onSelect }) {
  return (
    <div className="min-h-screen flex items-center justify-center p-4" style={{ background: '#0c0e14' }}>
      <div className="animate-fade-in text-center w-full max-w-md">
        <div className="inline-flex items-center justify-center w-14 h-14 rounded-2xl mb-6 overflow-hidden border"
          style={{ background: '#161921', borderColor: '#1e2130' }}>
          <Logo size={36} />
        </div>
        <h1 className="text-2xl font-semibold mb-1.5" style={{ color: '#e8eaf2' }}>
          Welcome to <span className="tracking-tight">wede</span>
        </h1>
        <p className="mb-8 text-sm" style={{ color: '#545b75' }}>Choose your preferred theme to get started</p>

        <div className="grid grid-cols-2 gap-4 px-2">
          {/* Dark — Midnight */}
          <button
            onClick={() => onSelect('dark')}
            className="group rounded-xl border-2 p-4 transition-all"
            style={{ background: '#11131a', borderColor: '#1e2130' }}
            onMouseEnter={e => e.currentTarget.style.borderColor = '#7c8cf8'}
            onMouseLeave={e => e.currentTarget.style.borderColor = '#1e2130'}
          >
            {/* Mini IDE preview */}
            <div className="rounded-lg p-3 mb-3 border" style={{ background: '#0c0e14', borderColor: '#1e2130' }}>
              <div className="flex gap-1.5 mb-2.5">
                <div className="w-2 h-2 rounded-full" style={{ background: '#f87171' }} />
                <div className="w-2 h-2 rounded-full" style={{ background: '#fbbf24' }} />
                <div className="w-2 h-2 rounded-full" style={{ background: '#4ade80' }} />
              </div>
              <div className="space-y-1.5">
                <div className="h-1.5 w-3/4 rounded" style={{ background: 'rgba(124,140,248,0.35)' }} />
                <div className="h-1.5 w-1/2 rounded" style={{ background: 'rgba(192,132,252,0.30)' }} />
                <div className="h-1.5 w-5/6 rounded" style={{ background: 'rgba(74,222,128,0.20)' }} />
                <div className="h-1.5 w-2/3 rounded" style={{ background: 'rgba(139,145,171,0.20)' }} />
              </div>
            </div>
            <div className="flex items-center justify-center gap-2" style={{ color: '#e8eaf2' }}>
              <Moon className="w-4 h-4" />
              <span className="font-semibold text-sm">Midnight</span>
            </div>
          </button>

          {/* Light — Daylight */}
          <button
            onClick={() => onSelect('light')}
            className="group rounded-xl border-2 p-4 transition-all"
            style={{ background: '#11131a', borderColor: '#1e2130' }}
            onMouseEnter={e => e.currentTarget.style.borderColor = '#7c8cf8'}
            onMouseLeave={e => e.currentTarget.style.borderColor = '#1e2130'}
          >
            <div className="rounded-lg p-3 mb-3 border" style={{ background: '#ffffff', borderColor: '#dde1ef' }}>
              <div className="flex gap-1.5 mb-2.5">
                <div className="w-2 h-2 rounded-full" style={{ background: '#dc2626' }} />
                <div className="w-2 h-2 rounded-full" style={{ background: '#d97706' }} />
                <div className="w-2 h-2 rounded-full" style={{ background: '#16a34a' }} />
              </div>
              <div className="space-y-1.5">
                <div className="h-1.5 w-3/4 rounded" style={{ background: 'rgba(79,95,247,0.30)' }} />
                <div className="h-1.5 w-1/2 rounded" style={{ background: 'rgba(124,58,237,0.25)' }} />
                <div className="h-1.5 w-5/6 rounded" style={{ background: 'rgba(22,163,74,0.20)' }} />
                <div className="h-1.5 w-2/3 rounded" style={{ background: 'rgba(139,145,171,0.25)' }} />
              </div>
            </div>
            <div className="flex items-center justify-center gap-2" style={{ color: '#e8eaf2' }}>
              <Sun className="w-4 h-4" />
              <span className="font-semibold text-sm">Daylight</span>
            </div>
          </button>
        </div>

        <p className="text-xs mt-6" style={{ color: '#545b75' }}>You can change this later in Settings</p>
      </div>
    </div>
  )
}
