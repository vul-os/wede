import { Image as ImageIcon, Package } from 'lucide-react'

function formatBytes(bytes) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

export function ImagePreview({ dataUrl, filename, size }) {
  return (
    <div className="h-full flex flex-col items-center justify-center bg-bg-primary p-6 select-none">
      {/* Checkerboard container */}
      <div
        className="relative flex items-center justify-center rounded-lg border border-border overflow-hidden mb-3"
        style={{
          backgroundImage:
            'repeating-conic-gradient(var(--c-bg-hover) 0% 25%, var(--c-bg-secondary) 0% 50%)',
          backgroundSize: '20px 20px',
          maxWidth: '100%',
          maxHeight: 'calc(100% - 64px)',
        }}
      >
        <img
          src={dataUrl}
          alt={filename}
          className="max-w-full max-h-full object-contain"
          style={{ maxHeight: 'calc(100vh - 200px)' }}
          draggable={false}
        />
      </div>
      <div className="flex items-center gap-1.5 text-[11px] text-text-muted">
        <ImageIcon className="w-3.5 h-3.5 shrink-0" />
        <span className="font-medium text-text-secondary">{filename}</span>
        {size != null && <span className="opacity-60">· {formatBytes(size)}</span>}
      </div>
    </div>
  )
}

export function BinaryNotice({ filename, size }) {
  return (
    <div className="h-full flex flex-col items-center justify-center bg-bg-primary p-6 select-none">
      <div className="w-14 h-14 rounded-2xl bg-bg-hover border border-border flex items-center justify-center mb-4">
        <Package className="w-7 h-7 text-text-muted opacity-40" />
      </div>
      <p className="text-[13px] font-semibold text-text-secondary mb-1">{filename}</p>
      <p className="text-[11px] text-text-muted">Binary file — cannot be displayed as text</p>
      {size != null && (
        <p className="text-[11px] text-text-muted mt-0.5">{formatBytes(size)}</p>
      )}
    </div>
  )
}
