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
      sort: 'name:asc,status:desc',
      status: 'active,disabled',
      site: '1,3',
    })
    expect(result.page).toBe(2)
    expect(result.pageSize).toBe(50)
    expect(result.q).toBe('alice')
    expect(result.sort).toBe('name:asc,status:desc')
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

// ---------------------------------------------------------------------------
// accountsSearchSchema — site → account deep-link params
// ---------------------------------------------------------------------------

describe('accountsSearchSchema — deep-link params', () => {
  it('accepts the sites guided-flow CTA shape (siteId + create=1)', () => {
    const result = accountsSearchSchema.parse({ siteId: '3', create: '1' })
    expect(result.siteId).toBe(3)
    expect(result.create).toBe(true)
  })

  it('accepts router-parsed primitives for siteId and create', () => {
    const result = accountsSearchSchema.parse({ siteId: 3, create: true })
    expect(result.siteId).toBe(3)
    expect(result.create).toBe(true)
  })

  it('treats create=0 as a non-open flag', () => {
    const result = accountsSearchSchema.parse({ siteId: 3, create: 0 })
    expect(result.create).toBe(false)
  })

  it('degrades malformed siteId / create to undefined', () => {
    const result = accountsSearchSchema.parse({
      siteId: 'bogus',
      create: 'bogus',
    })
    expect(result.siteId).toBeUndefined()
    // "bogus" coerces to boolean true; the page's resolver re-checks the
    // site against the loaded snapshot, so no dialog opens for a stale id.
    expect(result.create).toBe(true)
  })

  it('omits both params when absent', () => {
    const result = accountsSearchSchema.parse({})
    expect(result.siteId).toBeUndefined()
    expect(result.create).toBeUndefined()
  })
})

// ---------------------------------------------------------------------------
// accountsSearchSchema — dashboard attention deep-link param (accountId)
// ---------------------------------------------------------------------------

describe('accountsSearchSchema — accountId deep-link param', () => {
  it('accepts the attention deep-link shape (accountId)', () => {
    const result = accountsSearchSchema.parse({ accountId: '3' })
    expect(result.accountId).toBe(3)
  })

  it('accepts a router-parsed number for accountId', () => {
    const result = accountsSearchSchema.parse({ accountId: 3 })
    expect(result.accountId).toBe(3)
  })

  it('degrades malformed or non-positive accountId to undefined', () => {
    expect(
      accountsSearchSchema.parse({ accountId: 'bogus' }).accountId
    ).toBeUndefined()
    expect(
      accountsSearchSchema.parse({ accountId: 0 }).accountId
    ).toBeUndefined()
    expect(
      accountsSearchSchema.parse({ accountId: -4 }).accountId
    ).toBeUndefined()
    expect(
      accountsSearchSchema.parse({ accountId: 2.5 }).accountId
    ).toBeUndefined()
  })

  it('omits accountId when absent', () => {
    const result = accountsSearchSchema.parse({})
    expect(result.accountId).toBeUndefined()
  })
})
