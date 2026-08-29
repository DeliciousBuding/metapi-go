// Contract tests for downstream-key credential-ref parsing/serialization
// (#1026 UI follow-up). The backend GET endpoints return stored columns as
// raw JSON strings (or null), while create/update request bodies use real
// arrays — these helpers own that boundary.

import { describe, expect, it } from 'vitest'

import {
  parseCredentialRefs,
  parseIdArray,
  serializeCredentialRefs,
} from '../credential-refs'

describe('parseCredentialRefs', () => {
  it('parses the raw JSON string returned by GET endpoints', () => {
    expect(
      parseCredentialRefs(
        '[{"kind":"account_token","siteId":1,"accountId":2,"tokenId":7},{"kind":"default_api_key","siteId":1,"accountId":2}]'
      )
    ).toEqual([
      { kind: 'account_token', siteId: 1, accountId: 2, tokenId: 7 },
      { kind: 'default_api_key', siteId: 1, accountId: 2 },
    ])
  })

  it('treats null, empty string and empty array as unrestricted', () => {
    expect(parseCredentialRefs(null)).toEqual([])
    expect(parseCredentialRefs('')).toEqual([])
    expect(parseCredentialRefs([])).toEqual([])
  })

  it('drops malformed entries instead of crashing', () => {
    expect(
      parseCredentialRefs([
        null,
        42,
        { kind: 'account_token', siteId: 1, accountId: 2 },
        { kind: 'wat', siteId: 1, accountId: 2 },
        {
          kind: 'account_token',
          siteId: 0,
          accountId: 2,
          tokenId: 7,
        },
      ])
    ).toEqual([])
  })

  it('treats legacy entries without kind as default_api_key', () => {
    expect(parseCredentialRefs([{ siteId: 1, accountId: 2 }])).toEqual([
      { kind: 'default_api_key', siteId: 1, accountId: 2 },
    ])
  })

  it('deduplicates equivalent refs while preserving stable uniqueness', () => {
    expect(
      parseCredentialRefs([
        { kind: 'account_token', siteId: 1, accountId: 2, tokenId: 7 },
        { kind: 'account_token', siteId: 1, accountId: 2, tokenId: 7 },
      ])
    ).toEqual([{ kind: 'account_token', siteId: 1, accountId: 2, tokenId: 7 }])
  })
})

describe('serializeCredentialRefs', () => {
  it('round-trips a canonical list with no extra tokenId on default refs', () => {
    const refs = [
      {
        kind: 'default_api_key' as const,
        siteId: 1,
        accountId: 2,
        tokenId: 99,
      } as { kind: 'default_api_key'; siteId: number; accountId: number },
      { kind: 'account_token' as const, siteId: 1, accountId: 2, tokenId: 7 },
    ]
    expect(serializeCredentialRefs(refs)).toEqual([
      { kind: 'account_token', siteId: 1, accountId: 2, tokenId: 7 },
      { kind: 'default_api_key', siteId: 1, accountId: 2 },
    ])
  })

  it('sorts, canonicalizes and dedupes wire entries', () => {
    expect(
      serializeCredentialRefs([
        { kind: 'account_token', siteId: 2, accountId: 1, tokenId: 9 },
        { kind: 'account_token', siteId: 1, accountId: 2, tokenId: 7 },
        { kind: 'account_token', siteId: 2, accountId: 1, tokenId: 9 },
      ])
    ).toEqual([
      { kind: 'account_token', siteId: 1, accountId: 2, tokenId: 7 },
      { kind: 'account_token', siteId: 2, accountId: 1, tokenId: 9 },
    ])
  })
})

describe('parseIdArray', () => {
  it('parses JSON strings and arrays for site/route ID columns', () => {
    expect(parseIdArray('[1,2]')).toEqual([1, 2])
    expect(parseIdArray([2, 1])).toEqual([2, 1])
  })

  it('returns empty for null, malformed JSON and invalid IDs', () => {
    expect(parseIdArray(null)).toEqual([])
    expect(parseIdArray('not-json')).toEqual([])
    expect(parseIdArray([1, -2, 0, '3'])).toEqual([1])
  })
})
