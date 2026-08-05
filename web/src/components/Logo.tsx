import { useTheme } from '../hooks/useTheme'

interface LogoProps {
  size?: number
  showName?: boolean
  nameClass?: string
  className?: string
}

export default function Logo({ size = 24, showName = false, nameClass = '', className = '' }: LogoProps) {
  const { isDark } = useTheme()
  const src = isDark ? '/icon.svg' : '/icon-light.svg'

  return (
    <span className={`inline-flex items-center gap-1.5 shrink-0 ${className}`}>
      <img src={src} alt="wede" width={size} height={size} className="shrink-0" />
      {showName && (
        <span className={`font-brand font-semibold tracking-tight ${nameClass}`}>
          wede
        </span>
      )}
    </span>
  )
}
