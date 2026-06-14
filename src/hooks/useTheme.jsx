import { createContext, useContext, useState, useEffect, useCallback } from 'react'

const ThemeContext = createContext()

const STORAGE_KEY = 'wede_theme'

export function ThemeProvider({ children }) {
  const [theme, setThemeState] = useState(() => {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === 'dark' || stored === 'light') return stored
    return null // null = not yet chosen
  })

  const setTheme = useCallback((t) => {
    setThemeState(t)
    localStorage.setItem(STORAGE_KEY, t)
    document.documentElement.setAttribute('data-theme', t)
  }, [])

  useEffect(() => {
    if (theme) {
      document.documentElement.setAttribute('data-theme', theme)
    }
  }, [theme])

  const toggle = useCallback(() => {
    setTheme(theme === 'dark' ? 'light' : 'dark')
  }, [theme, setTheme])

  const isDark = theme === 'dark' || theme === null

  // Terminal theme — matched to new warm-indigo palette
  const terminalTheme = isDark ? {
    background:          '#0e1018',  // --c-bg-tertiary
    foreground:          '#e8eaf2',  // --c-text-primary
    cursor:              '#7c8cf8',  // --c-accent
    cursorAccent:        '#0e1018',
    selectionBackground: 'rgba(124, 140, 248, 0.25)',
    black:               '#1e2130',
    red:                 '#f87171',
    green:               '#4ade80',
    yellow:              '#fbbf24',
    blue:                '#7c8cf8',
    magenta:             '#c084fc',
    cyan:                '#22d3ee',
    white:               '#8b91ab',
    brightBlack:         '#545b75',
    brightRed:           '#f87171',
    brightGreen:         '#4ade80',
    brightYellow:        '#fbbf24',
    brightBlue:          '#9aa4fb',
    brightMagenta:       '#c084fc',
    brightCyan:          '#22d3ee',
    brightWhite:         '#e8eaf2',
  } : {
    background:          '#edf0f7',
    foreground:          '#111827',
    cursor:              '#4f5ff7',
    cursorAccent:        '#ffffff',
    selectionBackground: 'rgba(79, 95, 247, 0.18)',
    black:               '#8b91ab',
    red:                 '#dc2626',
    green:               '#16a34a',
    yellow:              '#d97706',
    blue:                '#4f5ff7',
    magenta:             '#7c3aed',
    cyan:                '#0891b2',
    white:               '#111827',
    brightBlack:         '#4b5577',
    brightRed:           '#dc2626',
    brightGreen:         '#16a34a',
    brightYellow:        '#d97706',
    brightBlue:          '#3b4bef',
    brightMagenta:       '#7c3aed',
    brightCyan:          '#0891b2',
    brightWhite:         '#111827',
  }

  return (
    <ThemeContext.Provider value={{ theme, setTheme, toggle, isDark, terminalTheme }}>
      {children}
    </ThemeContext.Provider>
  )
}

export function useTheme() {
  return useContext(ThemeContext)
}
