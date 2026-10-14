import { describe, expect, it } from 'vitest'

import { isKeyRestricted, keyRestrictionDimensions } from '../key-restrictions'

describe('keyRestrictionDimensions', () => {
  it('treats an empty key as unrestricted', () => {
    expect(keyRestrictionDimensions({})).toEqual([])
    expect(isKeyRestricted({})).toBe(false)
  })

  it('does not count the model wildcard as a restriction', () => {
    expect(keyRestrictionDimensions({ supportedModels: ['*'] })).toEqual([])
  })

  it('covers every dimension with parsed values', () => {
    const item = {
      supportedModels: ['gpt-5'],
      allowedRouteIds: '[1,2]',
      allowedSiteIds: [3],
      excludedCredentialRefs: JSON.stringify([{ siteId: 1, accountId: 2 }]),
      ipAllowlist: '10.0.0.0/8',
      maxRpm: 60,
      maxCost: 5,
      accessPolicy: { blockReason: 'banned' },
    }
    expect(keyRestrictionDimensions(item)).toEqual([
      'models',
      'routes',
      'sites',
      'credentials',
      'ip',
      'rate',
      'budget',
      'policy',
    ])
    expect(isKeyRestricted(item)).toBe(true)
  })

  it('ignores empty collections and nullish policy fields', () => {
    expect(
      keyRestrictionDimensions({
        supportedModels: [],
        allowedSiteIds: '[]',
        allowedCredentialRefs: 'not-json',
        ipAllowlist: '  ',
        accessPolicy: {},
      })
    ).toEqual([])
  })

  it('treats an unknown legacy policy as not restricted', () => {
    expect(keyRestrictionDimensions({ accessPolicy: '{broken' })).toEqual([])
  })

  it('counts token-per-minute and request quotas', () => {
    expect(keyRestrictionDimensions({ maxTpm: 1000 })).toEqual(['rate'])
    expect(keyRestrictionDimensions({ maxRequests: 10 })).toEqual(['budget'])
  })
})
