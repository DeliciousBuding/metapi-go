import { describe, expect, it } from 'vitest'

import {
  isKeyRestricted,
  KEY_RESTRICTION_DIMENSIONS,
  keyRestrictionDetails,
  keyRestrictionDimensions,
  restrictionAnchorField,
} from '../key-restrictions'

describe('keyRestrictionDetails', () => {
  it('treats an empty key as unrestricted', () => {
    expect(keyRestrictionDetails({})).toEqual([])
    expect(isKeyRestricted({})).toBe(false)
  })

  it('does not count the model wildcard as a restriction', () => {
    expect(keyRestrictionDetails({ supportedModels: ['*'] })).toEqual([])
  })

  it('covers every dimension with parsed counts, in display order', () => {
    const item = {
      supportedModels: ['gpt-5'],
      allowedRouteIds: '[1,2]',
      allowedSiteIds: [3],
      excludedSiteIds: [4],
      excludedCredentialRefs: JSON.stringify([{ siteId: 1, accountId: 2 }]),
      allowedCredentialRefs: JSON.stringify([{ siteId: 1, accountId: 2 }]),
      ipAllowlist: '10.0.0.0/8',
      ipBlocklist: '192.168.0.0/16, 172.16.0.0/12',
      maxRpm: 60,
      maxCost: 5,
      accessPolicy: { blockReason: 'banned' },
    }
    expect(keyRestrictionDetails(item)).toEqual([
      { dimension: 'models', count: 1 },
      { dimension: 'routes', count: 2 },
      { dimension: 'sites', count: 2 },
      { dimension: 'credentials', count: 2 },
      { dimension: 'ip', count: 3 },
      { dimension: 'rate', count: 1 },
      { dimension: 'budget', count: 1 },
      { dimension: 'policy', count: 1 },
    ])
    expect(isKeyRestricted(item)).toBe(true)
  })

  it('ignores empty collections and nullish policy fields', () => {
    expect(
      keyRestrictionDetails({
        supportedModels: [],
        allowedSiteIds: '[]',
        allowedCredentialRefs: 'not-json',
        ipAllowlist: '  ',
        accessPolicy: {},
      })
    ).toEqual([])
  })

  it('counts an unreadable stored policy as restricted with unknown count', () => {
    // The request path fails closed on InvalidAccessPolicy
    // (auth/downstream.go), so an unparseable policy must surface as
    // restricted — never hidden from an audit filter.
    expect(keyRestrictionDetails({ accessPolicy: '{broken' })).toEqual([
      { dimension: 'policy', count: null },
    ])
    expect(isKeyRestricted({ accessPolicy: '{broken' })).toBe(true)
  })

  it('counts token-per-minute and request quotas', () => {
    expect(keyRestrictionDetails({ maxTpm: 1000 })).toEqual([
      { dimension: 'rate', count: 1 },
    ])
    expect(keyRestrictionDetails({ maxRequests: 10 })).toEqual([
      { dimension: 'budget', count: 1 },
    ])
  })

  it('splits IP lists on newlines and commas like the backend', () => {
    // Mirrors splitIPList (auth/downstream.go): newline- and comma-separated,
    // blank entries dropped.
    expect(
      keyRestrictionDetails({ ipAllowlist: ' 10.0.0.1\n10.0.0.2,, 10.0.0.3 ' })
    ).toEqual([{ dimension: 'ip', count: 3 }])
  })
})

describe('keyRestrictionDimensions', () => {
  it('projects the details to bare dimension ids', () => {
    expect(
      keyRestrictionDimensions({ supportedModels: ['gpt-5'], maxRpm: 1 })
    ).toEqual(['models', 'rate'])
  })
})

describe('restrictionAnchorField', () => {
  it('maps form-managed dimensions to their data-key-scope anchors', () => {
    expect(restrictionAnchorField('models')).toBe('supportedModels')
    expect(restrictionAnchorField('sites')).toBe('allowedSiteIds')
    expect(restrictionAnchorField('credentials')).toBe('allowedCredentialRefs')
    expect(restrictionAnchorField('budget')).toBe('maxRequests')
    expect(restrictionAnchorField('policy')).toBe('accessPolicy')
  })

  it('returns null for dimensions without a form surface', () => {
    expect(restrictionAnchorField('routes')).toBeNull()
    expect(restrictionAnchorField('ip')).toBeNull()
    expect(restrictionAnchorField('rate')).toBeNull()
  })
})

describe('KEY_RESTRICTION_DIMENSIONS', () => {
  it('lists every dimension exactly once, in display order', () => {
    expect(KEY_RESTRICTION_DIMENSIONS).toEqual([
      'models',
      'routes',
      'sites',
      'credentials',
      'ip',
      'rate',
      'budget',
      'policy',
    ])
  })
})
