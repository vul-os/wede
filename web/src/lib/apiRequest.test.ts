import { describe, it, expect } from 'vitest'
import { parseReq, subst, buildSend } from './apiRequest'

describe('parseReq', () => {
  it('returns objects unchanged', () => {
    const o = { method: 'POST' }
    expect(parseReq(o)).toBe(o)
  })
  it('parses JSON strings', () => {
    expect(parseReq('{"method":"GET"}')).toEqual({ method: 'GET' })
  })
  it('falls back to {} for empty or malformed input', () => {
    expect(parseReq('')).toEqual({})
    expect(parseReq(null)).toEqual({})
    expect(parseReq('not json')).toEqual({})
  })
})

describe('subst', () => {
  const vars = { base: 'http://x', token: 'abc' }
  it('replaces known variables', () => {
    expect(subst('{{base}}/api?t={{token}}', vars)).toBe('http://x/api?t=abc')
  })
  it('leaves unknown variables intact', () => {
    expect(subst('{{missing}}', vars)).toBe('{{missing}}')
  })
  it('trims whitespace inside the braces', () => {
    expect(subst('{{ base }}', vars)).toBe('http://x')
  })
  it('handles empty/undefined input', () => {
    expect(subst('', vars)).toBe('')
    expect(subst(undefined, vars)).toBe('')
  })
})

describe('buildSend', () => {
  const vars = { base: 'http://api.test', token: 'secret' }

  it('substitutes the URL and appends enabled query params', () => {
    const out = buildSend({
      method: 'GET', url: '{{base}}/tasks',
      params: [
        { key: 'page', value: '2', enabled: true },
        { key: 'skip', value: 'x', enabled: false },
      ],
    }, vars)
    expect(out.method).toBe('GET')
    expect(out.url).toBe('http://api.test/tasks?page=2')
  })

  it('uses & when the URL already has a query string', () => {
    const out = buildSend({ url: 'http://x?a=1', params: [{ key: 'b', value: '2', enabled: true }] }, {})
    expect(out.url).toBe('http://x?a=1&b=2')
  })

  it('forwards enabled headers with substitution', () => {
    const out = buildSend({
      url: 'http://x',
      headers: [
        { key: 'Accept', value: 'application/json', enabled: true },
        { key: 'X-Off', value: 'no', enabled: false },
      ],
    }, vars)
    expect(out.headers.Accept).toBe('application/json')
    expect(out.headers['X-Off']).toBeUndefined()
  })

  it('builds bearer / basic / api-key auth headers', () => {
    expect(buildSend({ url: 'x', auth: { type: 'bearer', token: '{{token}}' } }, vars).headers.Authorization)
      .toBe('Bearer secret')
    expect(buildSend({ url: 'x', auth: { type: 'basic', username: 'u', password: 'p' } }, {}).headers.Authorization)
      .toBe('Basic ' + btoa('u:p'))
    expect(buildSend({ url: 'x', auth: { type: 'apikey', key: 'X-Key', value: '{{token}}' } }, vars).headers['X-Key'])
      .toBe('secret')
  })

  it('sets a JSON content-type and substitutes the body', () => {
    const out = buildSend({ url: 'x', body: { type: 'json', content: '{"t":"{{token}}"}' } }, vars)
    expect(out.headers['Content-Type']).toBe('application/json')
    expect(out.body).toBe('{"t":"secret"}')
  })

  it('url-encodes form bodies', () => {
    const out = buildSend({ url: 'x', body: { type: 'form', form: [{ key: 'a b', value: 'c&d', enabled: true }] } }, {})
    expect(out.headers['Content-Type']).toBe('application/x-www-form-urlencoded')
    expect(out.body).toBe('a%20b=c%26d')
  })

  it('emits no body for type none', () => {
    expect(buildSend({ url: 'x', body: { type: 'none' } }, {}).body).toBe('')
  })
})
