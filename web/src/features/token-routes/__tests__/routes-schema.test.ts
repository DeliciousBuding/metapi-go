import { describe, expect, it } from 'vitest'

import { buildChannelDraftSeed, routesSearchSchema } from '../lib/routes-schema'

// ---------------------------------------------------------------------------
// routesSearchSchema — tolerant URL search contract
// ---------------------------------------------------------------------------

describe('routesSearchSchema', () => {
  it('accepts the page-written URL shape', () => {
    const result = routesSearchSchema.parse({
      q: 'gpt',
      enabled: 'enabled,disabled',
      accountId: '7',
      siteId: '3',
      page: '2',
      pageSize: '50',
    })
    expect(result).toEqual({
      q: 'gpt',
      enabled: 'enabled,disabled',
      accountId: 7,
      siteId: 3,
      page: 2,
      pageSize: 50,
    })
  })

  it('tolerates router-parsed primitives without throwing', () => {
    const result = routesSearchSchema.parse({
      q: 123,
      enabled: true,
      accountId: 'bogus',
      siteId: 0,
      page: 0,
      pageSize: 'bogus',
    })
    expect(result.q).toBe(123)
    expect(result.enabled).toBe(true)
    expect(result.accountId).toBeUndefined()
    expect(result.siteId).toBeUndefined()
    expect(result.page).toBe(1)
    expect(result.pageSize).toBe(20)
  })

  it('falls back to page 1 / pageSize 20 for an empty input', () => {
    const result = routesSearchSchema.parse({})
    expect(result.page).toBe(1)
    expect(result.pageSize).toBe(20)
    expect(result.q).toBeUndefined()
  })

  it('parses the one-shot edit deep-link param like routeId', () => {
    // String form arrives from raw URL parsing, number form from the
    // router's JSON parsing — both must land on the same positive int.
    expect(routesSearchSchema.parse({ edit: '12' }).edit).toBe(12)
    expect(routesSearchSchema.parse({ edit: 12 }).edit).toBe(12)
  })

  it('drops a stale or malformed edit param instead of throwing', () => {
    expect(routesSearchSchema.parse({ edit: 'bogus' }).edit).toBeUndefined()
    expect(routesSearchSchema.parse({ edit: 0 }).edit).toBeUndefined()
    expect(routesSearchSchema.parse({ edit: -3 }).edit).toBeUndefined()
    expect(routesSearchSchema.parse({}).edit).toBeUndefined()
  })
})

// ---------------------------------------------------------------------------
// buildChannelDraftSeed — account → route guided-chain draft
// ---------------------------------------------------------------------------

describe('buildChannelDraftSeed', () => {
  it('seeds a single draft for a positive integer accountId', () => {
    expect(buildChannelDraftSeed(7)).toEqual([{ accountId: 7 }])
  })

  it('returns an empty array for a missing accountId', () => {
    expect(buildChannelDraftSeed(undefined)).toEqual([])
  })

  it('returns an empty array for a non-positive accountId', () => {
    expect(buildChannelDraftSeed(0)).toEqual([])
    expect(buildChannelDraftSeed(-1)).toEqual([])
  })

  it('returns an empty array for a fractional accountId', () => {
    expect(buildChannelDraftSeed(1.5)).toEqual([])
  })
})
