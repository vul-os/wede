import { useMemo } from 'react'
import { marked } from 'marked'

// MarkdownPreview renders a markdown string to HTML.
//
// Trust model: the content is the user's own workspace file, rendered for that
// same user. marked does not execute scripts by default (no raw-HTML eval of
// <script>), and there is no cross-user exposure here, so any XSS would be
// self-inflicted. If wede later renders OTHER users' markdown, add DOMPurify.
export default function MarkdownPreview({ content }) {
  const html = useMemo(() => {
    try {
      return marked.parse(content || '', { breaks: true, gfm: true })
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
