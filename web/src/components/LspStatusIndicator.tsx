import type { LspStatus } from '../lib/lsp'

export default function LspStatusIndicator({ status }: { status: LspStatus | null }) {
  if (!status) return null
  const unavailable = status.state === 'unavailable'
  return (
    <span
      role="status"
      aria-live="polite"
      className={`px-2 rounded-sm cursor-default ${unavailable ? 'text-yellow' : 'opacity-80'}`}
      title={unavailable
        ? `${status.message} No automatic retry. Switch files or toggle LSP in Settings to try again.`
        : status.message}
    >
      {unavailable ? 'LSP unavailable' : status.state === 'connecting' ? 'LSP connecting…' : 'LSP ready'}
    </span>
  )
}
