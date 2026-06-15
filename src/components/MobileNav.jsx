import { Files, Code, TerminalSquare, GitBranch, Menu } from 'lucide-react'

const tabs = [
  { id: 'files', icon: Files, label: 'Files' },
  { id: 'code', icon: Code, label: 'Code' },
  { id: 'terminal', icon: TerminalSquare, label: 'Terminal' },
  { id: 'git', icon: GitBranch, label: 'Git' },
  { id: 'menu', icon: Menu, label: 'More' },
]

export default function MobileNav({ active, onSelect, hasModified }) {
  return (
    <nav className="flex items-stretch bg-bg-tertiary border-t border-border mobile-safe-bottom">
      {/* eslint-disable-next-line no-unused-vars */}
      {tabs.map(({ id, icon: Icon, label }) => {
        const isActive = active === id
        return (
          <button
            key={id}
            onClick={() => onSelect(id)}
            className={`flex-1 flex flex-col items-center justify-center py-2 gap-0.5 transition-colors relative ${
              isActive ? 'text-accent' : 'text-text-muted'
            }`}
          >
            {isActive && (
              <div className="absolute top-0 left-1/4 right-1/4 h-0.5 bg-accent rounded-b" />
            )}
            <div className="relative">
              <Icon className="w-5 h-5" />
              {id === 'code' && hasModified && (
                <div className="absolute -top-0.5 -right-0.5 w-2 h-2 bg-accent rounded-full" />
              )}
            </div>
            <span className="text-[10px] font-medium">{label}</span>
          </button>
        )
      })}
    </nav>
  )
}
