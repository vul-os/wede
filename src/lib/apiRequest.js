// Pure helpers for the built-in API client — extracted from ApiClient.jsx so they
// can be unit-tested without rendering the component.

// parseReq tolerates a saved request being a JSON object (server RawMessage) or a
// string (older/hand-written files).
export function parseReq(raw) {
  if (!raw) return {}
  if (typeof raw === 'object') return raw
  try { return JSON.parse(raw) } catch { return {} }
}

// subst replaces {{name}} tokens from the active environment's variables; unknown
// tokens are left intact.
export function subst(str, vars) {
  return (str || '').replace(/\{\{([^}]+)\}\}/g, (_, k) => {
    const key = k.trim()
    return key in vars ? vars[key] : `{{${key}}}`
  })
}

// buildSend resolves a saved request + active env into the wire payload for /send:
// URL with query params, headers (incl. auth), and the body for the chosen type.
export function buildSend(req, vars) {
  let url = subst(req.url, vars)
  const qp = (req.params || []).filter((p) => p.enabled && p.key)
    .map((p) => `${encodeURIComponent(subst(p.key, vars))}=${encodeURIComponent(subst(p.value, vars))}`)
  if (qp.length) url += (url.includes('?') ? '&' : '?') + qp.join('&')

  const headers = {}
  ;(req.headers || []).filter((h) => h.enabled && h.key).forEach((h) => {
    headers[subst(h.key, vars)] = subst(h.value, vars)
  })
  const a = req.auth || {}
  if (a.type === 'bearer' && a.token) headers['Authorization'] = 'Bearer ' + subst(a.token, vars)
  else if (a.type === 'basic') headers['Authorization'] = 'Basic ' + btoa(`${subst(a.username || '', vars)}:${subst(a.password || '', vars)}`)
  else if (a.type === 'apikey' && a.key) headers[subst(a.key, vars)] = subst(a.value || '', vars)

  let body = ''
  const b = req.body || {}
  if (b.type === 'json' || b.type === 'raw') {
    body = subst(b.content || '', vars)
    if (b.type === 'json' && !headers['Content-Type']) headers['Content-Type'] = 'application/json'
  } else if (b.type === 'form') {
    body = (b.form || []).filter((f) => f.enabled && f.key)
      .map((f) => `${encodeURIComponent(subst(f.key, vars))}=${encodeURIComponent(subst(f.value, vars))}`).join('&')
    if (!headers['Content-Type']) headers['Content-Type'] = 'application/x-www-form-urlencoded'
  }
  return { method: req.method, url, headers, body }
}
