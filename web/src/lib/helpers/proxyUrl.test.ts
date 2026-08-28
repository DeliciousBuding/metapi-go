import { describe, expect, it } from 'vitest'

import { isEmptyOrProxyUrl, isProxyUrl } from './proxyUrl'

// Mirrors Go service.IsValidProxyURL (http/https/socks/socks5/socks5h). The
// forms must accept every scheme the backend transport can dial (#1009) and
// reject the rest client-side.
describe('isProxyUrl', () => {
  it.each([
    'http://proxy.example:8080',
    'https://proxy.example',
    'socks://proxy.example:1080',
    'socks5://proxy.example:1080',
    'socks5h://proxy.example:1080',
    'socks5://127.0.0.1:1080',
  ])('accepts %s', (value) => {
    expect(isProxyUrl(value)).toBe(true)
  })

  it('trims surrounding whitespace before validating', () => {
    expect(isProxyUrl('  socks5://proxy.example:1080  ')).toBe(true)
  })

  it.each([
    ['empty string', ''],
    ['whitespace only', '   '],
    ['unsupported ftp scheme', 'ftp://proxy.example'],
    ['socks4 is not backend-supported', 'socks4://proxy.example:1080'],
    ['scheme without a host', 'socks5://'],
    ['bare http scheme', 'http://'],
    ['plain string', 'not a url'],
  ])('rejects %s (%s)', (_label, value) => {
    expect(isProxyUrl(value)).toBe(false)
  })
})

describe('isEmptyOrProxyUrl', () => {
  it('treats blank as valid (no proxy)', () => {
    expect(isEmptyOrProxyUrl('')).toBe(true)
    expect(isEmptyOrProxyUrl('   ')).toBe(true)
  })

  it('delegates non-blank values to isProxyUrl', () => {
    expect(isEmptyOrProxyUrl('socks5://proxy.example:1080')).toBe(true)
    expect(isEmptyOrProxyUrl('http://proxy.example:8080')).toBe(true)
    expect(isEmptyOrProxyUrl('ftp://proxy.example')).toBe(false)
  })
})
