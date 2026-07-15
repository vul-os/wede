import { describe, it, expect, beforeEach } from 'vitest'
import { setActiveWorkspaceId, getActiveWorkspaceId, scopedUrl } from './activeWorkspace'

// activeWorkspace holds the focused workspace id in a module singleton and
// rewrites legacy /api/<service> paths to the workspace-scoped form. Reset the
// singleton before each test so cases don't leak into one another.
beforeEach(() => setActiveWorkspaceId(null))

describe('setActiveWorkspaceId / getActiveWorkspaceId', () => {
  it('round-trips the active id', () => {
    expect(getActiveWorkspaceId()).toBe(null)
    setActiveWorkspaceId('w1')
    expect(getActiveWorkspaceId()).toBe('w1')
  })
})

describe('scopedUrl', () => {
  it('passes everything through when no workspace is active', () => {
    expect(scopedUrl('/api/files?path=a')).toBe('/api/files?path=a')
  })

  it('rewrites each workspace-scoped service to the active workspace path', () => {
    setActiveWorkspaceId('w1')
    for (const svc of ['files', 'git', 'search', 'watch', 'lsp', 'terminal', 'tasks', 'trust', 'dap']) {
      expect(scopedUrl(`/api/${svc}/x`)).toBe(`/api/workspaces/w1/${svc}/x`)
    }
  })

  it('matches a service at a query boundary or end of string, not just a slash', () => {
    setActiveWorkspaceId('w1')
    expect(scopedUrl('/api/files?path=a')).toBe('/api/workspaces/w1/files?path=a')
    expect(scopedUrl('/api/git')).toBe('/api/workspaces/w1/git')
  })

  it('only rewrites the leading /api segment, leaving path-like query values intact', () => {
    setActiveWorkspaceId('w1')
    expect(scopedUrl('/api/files/read?path=api/foo.go'))
      .toBe('/api/workspaces/w1/files/read?path=api/foo.go')
  })

  it('url-encodes the workspace id so it cannot inject extra path segments', () => {
    setActiveWorkspaceId('a/b')
    expect(scopedUrl('/api/files')).toBe('/api/workspaces/a%2Fb/files')
  })

  it('does NOT rewrite non-workspace services (auth, folder, tunnel, etc.)', () => {
    setActiveWorkspaceId('w1')
    for (const url of ['/api/auth/login', '/api/folder/open', '/api/tunnel/status', '/api/workspaces']) {
      expect(scopedUrl(url)).toBe(url)
    }
  })

  it('does not double-scope an already-scoped url (prefix is /api/workspaces, not a scoped service)', () => {
    setActiveWorkspaceId('w2')
    expect(scopedUrl('/api/workspaces/w1/files')).toBe('/api/workspaces/w1/files')
  })

  it('guards against a service name only as a prefix of a longer word', () => {
    setActiveWorkspaceId('w1')
    // "/api/filesystem" is not the "files" service — the regex requires a
    // following /, ?, or end, so it must pass through untouched.
    expect(scopedUrl('/api/filesystem')).toBe('/api/filesystem')
  })

  it('passes through non-string urls', () => {
    setActiveWorkspaceId('w1')
    expect(scopedUrl(undefined)).toBe(undefined)
    expect(scopedUrl(null)).toBe(null)
  })
})
