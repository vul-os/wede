import { X, Globe } from 'lucide-react'

export default function EditorTabs({ tabs, activeTab, onSelect, onClose }) {
  if (tabs.length === 0) return null

  return (
    <div className="flex bg-bg-secondary border-b border-border overflow-x-auto shrink-0 compact-touch" style={{ scrollbarWidth: 'none' }}>
      {tabs.map((tab) => {
        const isActive = activeTab === tab.path
        const isBrowser = tab.type === 'browser'
        return (
          <div
            key={tab.path}
            className={`relative flex items-center gap-1.5 px-3 py-0 h-9 cursor-pointer border-r border-border shrink-0 transition-colors select-none ${
              isActive
                ? 'bg-bg-primary text-text-primary'
                : 'text-text-muted hover:text-text-secondary hover:bg-bg-hover'
            }`}
            onClick={() => onSelect(tab.path)}
          >
            {/* Active indicator — top edge line */}
            {isActive && (
              <span className="absolute top-0 left-0 right-0 h-[1.5px] bg-accent rounded-b" />
            )}

            {isBrowser && <Globe className="w-3 h-3 text-cyan shrink-0" />}
            {!isBrowser && tab.modified && (
              <span className="w-1.5 h-1.5 rounded-full bg-yellow shrink-0" />
            )}

            <span className={`text-[12px] truncate max-w-40 font-medium leading-none ${
              isActive ? 'text-text-primary' : ''
            } ${tab.preview && !tab.modified ? 'italic' : ''}`}>{tab.name}</span>

            <button
              onClick={(e) => { e.stopPropagation(); onClose(tab.path) }}
              className={`ml-0.5 w-4 h-4 flex items-center justify-center rounded transition-colors shrink-0 ${
                isActive
                  ? 'text-text-secondary hover:text-text-primary hover:bg-bg-hover'
                  : 'text-transparent group-hover:text-text-muted hover:text-text-primary hover:bg-bg-hover'
              }`}
            >
              <X className="w-3 h-3" />
            </button>
          </div>
        )
      })}
    </div>
  )
}
