import { describe, expect, it } from 'vitest'

import { routesSearchSchema } from '../lib/routes-schema'

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
      site: undefined,
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
})
