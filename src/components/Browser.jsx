import { useState, useRef, useEffect } from 'react'
import { Globe, ExternalLink, RefreshCw, ArrowLeft, ArrowRight } from 'lucide-react'

export default function Browser({ url: initialUrl, onUrlChange }) {
  const [url, setUrl] = useState(initialUrl || 'https://wede.vulos.org')
  const [loadedUrl, setLoadedUrl] = useState(initialUrl || 'https://wede.vulos.org')
  const iframeRef = useRef(null)

  useEffect(() => {
    if (initialUrl && initialUrl !== loadedUrl) {
      setUrl(initialUrl)
      setLoadedUrl(initialUrl)
    }
  }, [initialUrl])

  const navigate = (e) => {
    e?.preventDefault()
    if (!url.trim()) return
    let target = url.trim()
    if (!target.startsWith('http://') && !target.startsWith('https://')) {
      target = 'http://' + target
    }
    setLoadedUrl(target)
    onUrlChange?.(target)
  }

  const refresh = () => {
    if (iframeRef.current) {
      iframeRef.current.src = iframeRef.current.src
    }
  }

  return (
    <div className="h-full flex flex-col bg-bg-secondary">
      <form onSubmit={navigate} className="flex items-center gap-1.5 px-2 py-1.5 border-b border-border shrink-0">
        <Globe className="w-3.5 h-3.5 text-text-muted shrink-0" />
        <input
          type="text"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="Enter URL (e.g. localhost:3000)"
          className="flex-1 bg-bg-input border border-border rounded-md px-2.5 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent font-mono min-w-0"
        />
        <button type="submit"
          className="px-2.5 py-1.5 bg-accent text-white text-xs rounded-md hover:bg-accent-hover transition-colors font-medium shrink-0">
          Go
        </button>
        <button type="button" onClick={refresh}
          className="p-1.5 text-text-muted hover:text-text-primary hover:bg-bg-hover rounded shrink-0">
          <RefreshCw className="w-3.5 h-3.5" />
        </button>
        <a href={loadedUrl} target="_blank" rel="noopener noreferrer"
          className="p-1.5 text-text-muted hover:text-text-primary hover:bg-bg-hover rounded shrink-0">
          <ExternalLink className="w-3.5 h-3.5" />
        </a>
      </form>

      <div className="flex-1 min-h-0">
        <iframe
          ref={iframeRef}
          src={loadedUrl}
          className="w-full h-full border-none bg-white"
          sandbox="allow-same-origin allow-scripts allow-forms allow-popups allow-modals allow-top-navigation-by-user-activation"
          title="Browser"
          onLoad={() => {
            try {
              const iframeUrl = iframeRef.current?.contentWindow?.location?.href
              if (iframeUrl && iframeUrl !== 'about:blank' && iframeUrl !== loadedUrl) {
                setUrl(iframeUrl)
                setLoadedUrl(iframeUrl)
                onUrlChange?.(iframeUrl)
              }
            } catch {
              // Cross-origin — can't read URL, that's fine
            }
          }}
        />
      </div>
    </div>
  )
}
