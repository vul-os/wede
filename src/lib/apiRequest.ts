// Pure helpers for the built-in API client — extracted from ApiClient.jsx so they
// can be unit-tested without rendering the component.

export interface ApiKeyValue {
  key: string
  value: string
  enabled: boolean
}

export interface ApiAuth {
  type?: 'none' | 'bearer' | 'basic' | 'apikey'
  token?: string
  username?: string
  password?: string
  key?: string
  value?: string
}

export interface ApiBody {
  type?: 'none' | 'json' | 'raw' | 'form'
  content?: string
  form?: ApiKeyValue[]
}

export interface ApiRequest {
  name?: string
  method?: string
  url?: string
  params?: ApiKeyValue[]
  headers?: ApiKeyValue[]
  auth?: ApiAuth
  body?: ApiBody
}

export type ApiVars = Record<string, string>

export interface ApiSendPayload {
  method?: string
  url: string
  headers: Record<string, string>
  body: string
}

// ApiTreeNode is a saved-collections tree entry (a request leaf or a folder),
// as returned by GET /api/workspaces/{id}/apiclient.
export interface ApiTreeNode {
  type: 'folder' | 'request'
  name: string
  path: string
  request?: unknown
  children?: ApiTreeNode[]
}

export interface ApiEnvironment {
  name: string
  variables?: ApiVars
}

// parseReq tolerates a saved request being a JSON object (server RawMessage) or a
// string (older/hand-written files).
export function parseReq(raw: unknown): Partial<ApiRequest> {
  if (!raw) return {}
  // No assertion needed: every member of ApiRequest is optional, so a bare
  // `object` (raw, narrowed by the typeof check and the falsy-null return
  // above) is already structurally assignable to Partial<ApiRequest>.
  if (typeof raw === 'object') return raw
  // JSON.parse returns `any` by design (the shape isn't known until runtime);
  // this function's own signature is the honest boundary — its callers get
  // Partial<ApiRequest>, same as the object branch above.
  try { return JSON.parse(raw as string) as Partial<ApiRequest> } catch { return {} }
}

// subst replaces {{name}} tokens from the active environment's variables; unknown
// tokens are left intact.
export function subst(str: string | undefined | null, vars: ApiVars): string {
  return (str || '').replace(/\{\{([^}]+)\}\}/g, (_, k: string) => {
    const key = k.trim()
    return key in vars ? vars[key] : `{{${key}}}`
  })
}

// buildSend resolves a saved request + active env into the wire payload for /send:
// URL with query params, headers (incl. auth), and the body for the chosen type.
export function buildSend(req: ApiRequest, vars: ApiVars): ApiSendPayload {
  let url = subst(req.url, vars)
  const qp = (req.params || []).filter((p) => p.enabled && p.key)
    .map((p) => `${encodeURIComponent(subst(p.key, vars))}=${encodeURIComponent(subst(p.value, vars))}`)
  if (qp.length) url += (url.includes('?') ? '&' : '?') + qp.join('&')

  const headers: Record<string, string> = {}
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
