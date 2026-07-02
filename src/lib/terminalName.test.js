import { describe, it, expect } from 'vitest'
import { normalizeTitle, applyAutoName } from './terminalName'

describe('normalizeTitle', () => {
  it('trims and collapses whitespace', () => {
    expect(normalizeTitle('  zsh   —  ~/code  ')).toBe('zsh — ~/code')
  })
  it('returns empty string for blank / nullish input', () => {
    expect(normalizeTitle('')).toBe('')
    expect(normalizeTitle('   ')).toBe('')
    expect(normalizeTitle(undefined)).toBe('')
    expect(normalizeTitle(null)).toBe('')
  })
  it('caps overly long titles with an ellipsis', () => {
    const long = 'a'.repeat(60)
    const out = normalizeTitle(long)
    expect(out).toHaveLength(40)
    expect(out.endsWith('…')).toBe(true)
  })
  it('leaves titles at or under the cap intact', () => {
    const s = 'a'.repeat(40)
    expect(normalizeTitle(s)).toBe(s)
  })
})

describe('applyAutoName', () => {
  const base = [{ id: 1, name: 'Terminal 1' }, { id: 2, name: 'Terminal 2' }]

  it('renames the matching terminal from a PTY title', () => {
    const out = applyAutoName(base, 2, 'npm run dev')
    expect(out[1].name).toBe('npm run dev')
    expect(out[0].name).toBe('Terminal 1')
  })
  it('does not override a manually named terminal', () => {
    const manual = [{ id: 1, name: 'build', manual: true }]
    expect(applyAutoName(manual, 1, 'zsh')).toBe(manual)
  })
  it('returns the same array reference when nothing changes (no re-render)', () => {
    expect(applyAutoName(base, 1, 'Terminal 1')).toBe(base)
    expect(applyAutoName(base, 1, '   ')).toBe(base)
    expect(applyAutoName(base, 99, 'ghost')).toBe(base)
  })
  it('normalizes the incoming title before applying', () => {
    const out = applyAutoName(base, 1, '  vim  main.go  ')
    expect(out[0].name).toBe('vim main.go')
  })
})
