import { describe, expect, it } from 'vitest'

import {
  buildChannelDraftSeed,
  routesSearchSchema,
  setChannelDraftSelection,
} from '../lib/routes-schema'

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

describe('setChannelDraftSelection', () => {
  it('keeps existing token and source-model choices when selecting other accounts', () => {
    const drafts = [
      { accountId: 7, tokenId: 70, sourceModel: 'upstream-a' },
      { accountId: 7, tokenId: 71, sourceModel: 'upstream-b' },
      { accountId: 99, sourceModel: 'guided-model' },
    ]

    expect(setChannelDraftSelection(drafts, [7, 8], true)).toEqual([
      { accountId: 7, tokenId: 70, sourceModel: 'upstream-a' },
      { accountId: 7, tokenId: 71, sourceModel: 'upstream-b' },
      { accountId: 99, sourceModel: 'guided-model' },
      { accountId: 8 },
    ])
    expect(drafts).toHaveLength(3)
  })

  it('does not duplicate drafts for repeated selections or repeated account IDs', () => {
    const selected = setChannelDraftSelection([], [7, 8, 8], true)
    expect(selected).toEqual([{ accountId: 7 }, { accountId: 8 }])
    expect(setChannelDraftSelection(selected, [7, 8], true)).toEqual(selected)
  })

  it('deselects only the displayed accounts and retains an off-list draft', () => {
    const drafts = [
      { accountId: 7, tokenId: 70, sourceModel: 'upstream-a' },
      { accountId: 8 },
      { accountId: 99, tokenId: 990, sourceModel: 'guided-model' },
    ]

    expect(setChannelDraftSelection(drafts, [7, 8], false)).toEqual([
      { accountId: 99, tokenId: 990, sourceModel: 'guided-model' },
    ])
    expect(drafts).toHaveLength(3)
  })

  it.each([true, false])(
    'does not change drafts for an empty candidate list (checked=%s)',
    (checked) => {
      const drafts = [{ accountId: 7, tokenId: 70, sourceModel: 'upstream-a' }]
      expect(setChannelDraftSelection(drafts, [], checked)).toEqual(drafts)
    }
  )
})
