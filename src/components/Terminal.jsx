import { useEffect, useRef, useImperativeHandle, forwardRef } from 'react'
import { Terminal as XTerminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'

export default forwardRef(function Terminal({ token, sessionId, visible, terminalTheme, fontSize = 13 }, ref) {
  const containerRef = useRef(null)
  const termRef = useRef(null)
  const wsRef = useRef(null)
  const fitRef = useRef(null)
  const reconnectRef = useRef(null)
  const mountedRef = useRef(true)

  // Expose send function for external toolbar
  useImperativeHandle(ref, () => ({
    send: (data) => {
      const ws = wsRef.current
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(data)
      termRef.current?.focus()
    }
  }), [])

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  useEffect(() => {
    if (!containerRef.current || !token || termRef.current) return

    const term = new XTerminal({
      cursorBlink: true,
      fontSize,
      fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', 'SF Mono', monospace",
      theme: terminalTheme,
      allowTransparency: true,
      scrollback: 5000,
    })

    const fitAddon = new FitAddon()
    const webLinksAddon = new WebLinksAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(webLinksAddon)
    term.open(containerRef.current)

    // Disable mobile keyboard autocomplete/autocorrect on xterm's hidden textarea
    const textarea = containerRef.current.querySelector('textarea')
    if (textarea) {
      textarea.setAttribute('autocomplete', 'off')
      textarea.setAttribute('autocorrect', 'off')
      textarea.setAttribute('autocapitalize', 'off')
      textarea.setAttribute('spellcheck', 'false')
      textarea.setAttribute('data-gramm', 'false')
    }

    fitRef.current = fitAddon
    termRef.current = term

    setTimeout(() => fitAddon.fit(), 50)

    const sid = sessionId || token

    function connect(isReconnect) {
      if (!mountedRef.current) return

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      // In dev mode (Vite on 5173), connect directly to backend on 9090
      const port = window.location.port
      const host = (port === '5173' || port === '5174') ? window.location.hostname + ':9090' : window.location.host
      // Pass auth token as a WebSocket subprotocol ("auth.<token>") so it never
      // appears in server access logs or browser history. The session ID is a
      // non-secret resumption handle passed as a query param.
      const ws = new WebSocket(
        `${protocol}//${host}/api/terminal?session=${encodeURIComponent(sid)}`,
        [`auth.${token}`]
      )
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onopen = () => {
        if (isReconnect) {
          // Clear screen before replay — the server sends scrollback
          term.clear()
        }
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }

      ws.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) {
          term.write(new Uint8Array(event.data))
        } else {
          term.write(event.data)
        }
      }

      ws.onerror = () => {}

      ws.onclose = () => {
        if (!mountedRef.current) return
        // Only reconnect if this is still the active WebSocket
        if (wsRef.current !== ws) return
        wsRef.current = null
        scheduleReconnect()
      }
    }

    let reconnectDelay = 1000
    function scheduleReconnect() {
      if (!mountedRef.current) return
      reconnectRef.current = setTimeout(() => {
        if (!mountedRef.current) return
        connect(true)
        reconnectDelay = Math.min(reconnectDelay * 1.5, 10000)
      }, reconnectDelay)
    }

    connect(false)

    term.onData((data) => {
      const ws = wsRef.current
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(data)
    })

    term.onResize(({ cols, rows }) => {
      const ws = wsRef.current
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }))
      }
    })

    const ro = new ResizeObserver(() => {
      try { fitAddon.fit() } catch (_) { /* ignore resize errors */ }
    })
    ro.observe(containerRef.current)

    return () => {
      mountedRef.current = false
      if (reconnectRef.current) clearTimeout(reconnectRef.current)
      ro.disconnect()
      if (wsRef.current) wsRef.current.close()
      term.dispose()
      termRef.current = null
      wsRef.current = null
      fitRef.current = null
    }
  }, [token, sessionId])

  // Update theme dynamically
  useEffect(() => {
    if (termRef.current && terminalTheme) {
      termRef.current.options.theme = terminalTheme
    }
  }, [terminalTheme])

  useEffect(() => {
    if (visible) {
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          try { fitRef.current?.fit() } catch (_) { /* ignore */ }
          try { termRef.current?.focus() } catch (_) { /* ignore */ }
        })
      })
    }
  }, [visible])

  return <div ref={containerRef} className="h-full w-full" style={{ display: visible ? 'block' : 'none' }} />
})
