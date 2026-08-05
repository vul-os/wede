import { useMemo } from 'react'
import { marked } from 'marked'
import DOMPurify from 'dompurify'

interface MarkdownPreviewProps {
  content?: string
}

// MarkdownPreview renders a markdown string to HTML.
//
// Security: marked emits raw HTML embedded in the markdown verbatim, so the
// rendered output is sanitized with DOMPurify before it is injected via
// dangerouslySetInnerHTML. This blocks stored-XSS payloads (e.g. <img onerror>,
// <script>, javascript: URLs) that could otherwise run in the viewer's session —
// important now that wede renders other collaborators' files, not just the
// viewer's own.
export default function MarkdownPreview({ content }: MarkdownPreviewProps) {
  const html = useMemo(() => {
    try {
      // The options passed don't set `async`, so marked's sync overload applies;
      // the broader `string | Promise<string>` signature still needs the cast.
      const raw = marked.parse(content || '', { breaks: true, gfm: true }) as string
      return DOMPurify.sanitize(raw)
    } catch {
      return '<p>Failed to render markdown.</p>'
    }
  }, [content])

  return (
    <div className="h-full overflow-auto bg-bg-primary">
      <div
        className="wede-markdown max-w-3xl mx-auto px-8 py-6 text-[14px] leading-relaxed text-text-primary"
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  )
}
