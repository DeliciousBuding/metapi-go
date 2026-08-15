import { describe, expect, it } from 'vitest'

import { accountsSearchSchema } from '@/routes/_authenticated/accounts'

// ---------------------------------------------------------------------------
// accountsSearchSchema (route-level validateSearch) — tolerant URL contract
// ---------------------------------------------------------------------------

describe('accountsSearchSchema', () => {
  it('accepts the page-written URL shape', () => {
    const result = accountsSearchSchema.parse({
      page: '2',
      pageSize: '50',
      q: 'alice',
      status: 'active,disabled',
      site: '1,3',
    })
    expect(result.page).toBe(2)
    expect(result.pageSize).toBe(50)
    expect(result.q).toBe('alice')
    expect(result.status).toBe('active,disabled')
    expect(result.site).toBe('1,3')
  })

  it('tolerates router-parsed primitives without throwing', () => {
    const result = accountsSearchSchema.parse({
      page: 0,
      pageSize: 'bogus',
      q: 123,
      status: true,
      site: 7,
    })
    expect(result.page).toBe(1)
    expect(result.pageSize).toBe(20)
    expect(result.q).toBe(123)
    expect(result.status).toBe(true)
    expect(result.site).toBe(7)
  })

  it('falls back to page 1 / pageSize 20 for an empty input', () => {
    const result = accountsSearchSchema.parse({})
    expect(result.page).toBe(1)
    expect(result.pageSize).toBe(20)
    expect(result.q).toBeUndefined()
  })
})
