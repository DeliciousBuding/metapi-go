// metapi-go/lib/helpers — the open-redirect guard's accept/reject matrix.
//
// This function is the only thing standing between an attacker-controlled
// `?redirect=` and a `navigate()` after login, and it is called from three
// places that must agree (sign-in route, login form, HTTP client 401). Every
// branch it has is therefore pinned here: the shapes it accepts, the shapes it
// refuses, and the fact that what it returns can never carry an origin.
import { describe, expect, it } from 'vitest'

import { sanitizeAuthRedirect } from '../sanitize-auth-redirect'

const ORIGIN = 'https://metapi.example'

describe('sanitizeAuthRedirect accepts same-origin targets', () => {
  it.each([
    ['/dashboard', '/dashboard'],
    ['/models?q=gpt-4o#top', '/models?q=gpt-4o#top'],
    ['  /sites  ', '/sites'],
    ['https://metapi.example/sites', '/sites'],
    ['HTTPS://METAPI.EXAMPLE/sites?a=1', '/sites?a=1'],
    ['/', '/'],
  ])('sanitizes %s to %s', (input, expected) => {
    expect(sanitizeAuthRedirect(input, ORIGIN)).toBe(expected)
  })

  it('never returns an origin, so the router cannot re-parse one', () => {
    const result = sanitizeAuthRedirect('https://metapi.example/a?b#c', ORIGIN)
    expect(result).toBe('/a?b#c')
    expect(result).not.toContain('//')
  })
})

describe('sanitizeAuthRedirect refuses everything else', () => {
  it.each([
    ['another origin', 'https://evil.example/steal'],
    ['protocol-relative', '//evil.example/steal'],
    ['backslash normalised to protocol-relative', '/\\evil.example/steal'],
    ['javascript scheme', 'javascript:alert(1)'],
    ['data scheme', 'data:text/html,<script>alert(1)</script>'],
    ['non-http scheme on the trusted host', 'ftp://metapi.example/x'],
    ['empty string', ''],
    ['whitespace only', '   '],
  ])('rejects %s', (_label, input) => {
    expect(sanitizeAuthRedirect(input, ORIGIN)).toBeNull()
  })

  it.each([
    ['null', null],
    ['a number', 42],
    ['an object', { pathname: '/dashboard' }],
    ['an array', ['/dashboard']],
    ['undefined', undefined],
  ])('rejects non-string input (%s)', (_label, input) => {
    expect(sanitizeAuthRedirect(input, ORIGIN)).toBeNull()
  })

  it.each([
    ['not a URL', 'metapi.example'],
    ['empty', ''],
    ['non-http origin', 'ftp://metapi.example'],
  ])(
    'rejects an untrustworthy origin (%s) rather than trusting every target',
    (_label, origin) => {
      expect(sanitizeAuthRedirect('/dashboard', origin)).toBeNull()
      expect(
        sanitizeAuthRedirect('https://metapi.example/dashboard', origin)
      ).toBeNull()
    }
  )
})
