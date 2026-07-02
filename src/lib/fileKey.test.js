import { describe, it, expect } from 'vitest'
import { fileKey, parseFileKey, isFileKey, isSpecialTabId, normalizeTabIdentity } from './fileKey'

describe('fileKey / parseFileKey', () => {
  it('round-trips a workspace id and relative path', () => {
    const k = fileKey('abc123', 'src/main.go')
    expect(parseFileKey(k)).toEqual({ workspaceId: 'abc123', rel: 'src/main.go' })
  })
  it('handles relative paths containing spaces', () => {
    const k = fileKey('w1', 'my docs/read me.md')
    expect(parseFileKey(k)).toEqual({ workspaceId: 'w1', rel: 'my docs/read me.md' })
  })
  it('handles an empty relative path (workspace root)', () => {
    const k = fileKey('w1', '')
    expect(parseFileKey(k)).toEqual({ workspaceId: 'w1', rel: '' })
  })
  it('treats a separator-less string as a non-file pseudo-id', () => {
    expect(isFileKey('browser:1')).toBe(false)
    expect(parseFileKey('browser:1')).toEqual({ workspaceId: null, rel: 'browser:1' })
  })
  it('recognizes composite keys', () => {
    expect(isFileKey(fileKey('w1', 'a.js'))).toBe(true)
    expect(isFileKey('gitgraph:1')).toBe(false)
    expect(isFileKey(null)).toBe(false)
  })
})

describe('isSpecialTabId', () => {
  it('recognizes browser/gitgraph/apiclient pseudo-ids', () => {
    expect(isSpecialTabId('browser:1')).toBe(true)
    expect(isSpecialTabId('gitgraph:1')).toBe(true)
    expect(isSpecialTabId('apiclient:1')).toBe(true)
    expect(isSpecialTabId('src/main.go')).toBe(false)
    expect(isSpecialTabId(null)).toBe(false)
  })
})

describe('normalizeTabIdentity (upgrade of pre-multi-root persisted tabs)', () => {
  it('repairs a legacy bare-path file tab, backfilling rel/workspaceId and composite path', () => {
    const legacy = { path: 'src/App.jsx', name: 'App.jsx' }
    const out = normalizeTabIdentity(legacy, 'wActive')
    expect(out.rel).toBe('src/App.jsx')
    expect(out.workspaceId).toBe('wActive')
    expect(out.path).toBe(fileKey('wActive', 'src/App.jsx'))
    expect(parseFileKey(out.path)).toEqual({ workspaceId: 'wActive', rel: 'src/App.jsx' })
  })
  it('leaves an already-composite tab unchanged (same reference)', () => {
    const modern = { path: fileKey('w1', 'a.js'), rel: 'a.js', workspaceId: 'w1', name: 'a.js' }
    expect(normalizeTabIdentity(modern, 'wActive')).toBe(modern)
  })
  it('preserves the tab\'s own workspace when its path is composite but fields are missing', () => {
    const out = normalizeTabIdentity({ path: fileKey('wOwn', 'x.go') }, 'wActive')
    expect(out.workspaceId).toBe('wOwn')
    expect(out.rel).toBe('x.go')
  })
  it('passes through special tabs untouched', () => {
    const browser = { path: 'browser:1', type: 'browser', url: 'https://x' }
    expect(normalizeTabIdentity(browser, 'wActive')).toBe(browser)
    const gg = { path: 'gitgraph:1', type: 'gitgraph' }
    expect(normalizeTabIdentity(gg, 'wActive')).toBe(gg)
  })
  it('leaves a tab unchanged when no workspace can be resolved', () => {
    const legacy = { path: 'src/App.jsx' }
    expect(normalizeTabIdentity(legacy, null)).toBe(legacy)
  })
  it('handles nullish input', () => {
    expect(normalizeTabIdentity(null, 'w1')).toBe(null)
    expect(normalizeTabIdentity(undefined, 'w1')).toBe(undefined)
  })
})
