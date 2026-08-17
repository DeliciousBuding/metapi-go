// Unit tests for the import wizard URL helpers. `parseUrlLines` only dedups
// raw trimmed lines; canonical dedup (trailing slash, query, fragment) is
// `canonicalizeUrl`'s job — exercised separately so the split of concerns
// stays explicit.

import { describe, expect, it } from 'vitest'

import { canonicalizeUrl, parseUrlLines } from '../lib/utils'

describe('parseUrlLines', () => {
  it('returns an empty array for empty input', () => {
    expect(parseUrlLines('')).toEqual([])
  })

  it('filters blank and whitespace-only lines', () => {
    expect(parseUrlLines('\n  \n\t\n \n')).toEqual([])
  })

  it('trims leading and trailing whitespace from each line', () => {
    expect(parseUrlLines('  https://a.com  \n\t https://b.com\t')).toEqual([
      'https://a.com',
      'https://b.com',
    ])
  })

  it('dedups exact raw lines after trimming', () => {
    expect(parseUrlLines('https://a.com\nhttps://a.com')).toEqual([
      'https://a.com',
    ])
  })

  it('keeps lines that only differ by trailing slash (canonicalization is separate)', () => {
    // parseUrlLines deliberately does not canonicalize — the wizard relies
    // on canonicalizeUrl for that comparison. Asserting this keeps the
    // boundary honest instead of papering over it here.
    expect(parseUrlLines('https://a.com\nhttps://a.com/')).toHaveLength(2)
  })

  it('preserves order of first occurrence after dedup', () => {
    expect(
      parseUrlLines('https://b.com\nhttps://a.com\nhttps://b.com')
    ).toEqual(['https://b.com', 'https://a.com'])
  })
})

describe('canonicalizeUrl', () => {
  it('returns empty string for empty or whitespace-only input', () => {
    expect(canonicalizeUrl('')).toBe('')
    expect(canonicalizeUrl('   ')).toBe('')
  })

  it('strips the query string and fragment', () => {
    expect(canonicalizeUrl('https://a.com/path?x=1#frag')).toBe(
      'https://a.com/path'
    )
  })

  it('strips a trailing slash', () => {
    expect(canonicalizeUrl('https://a.com/')).toBe('https://a.com')
  })

  it('maps trailing-slash + query + fragment variants to one canonical form so a Set dedups them', () => {
    const urls = [
      'https://a.com',
      'https://a.com/',
      'https://a.com?x=1',
      'https://a.com/#frag',
      'https://a.com/?x=1#frag',
    ]
    const canonical = new Set(urls.map((url) => canonicalizeUrl(url)))
    expect(canonical.size).toBe(1)
    expect(canonical.has('https://a.com')).toBe(true)
  })

  it('handles mixed http/https: different schemes stay distinct, bare host gets https', () => {
    expect(canonicalizeUrl('http://a.com')).toBe('http://a.com')
    expect(canonicalizeUrl('https://a.com')).toBe('https://a.com')
    expect(canonicalizeUrl('a.com')).toBe('https://a.com')
  })

  it('falls back to a trimmed trailing-slash strip for non-URL strings', () => {
    expect(canonicalizeUrl('not a url/')).toBe('not a url')
  })
})
