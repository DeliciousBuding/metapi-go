// Unit tests for the site display-balance resolution shared by the list
// column and the detail sheet.

import { describe, expect, it } from 'vitest'

import { resolveSiteBalanceUsd } from '../lib/site-balance'
import type { Site } from '../types'

function makeSite(overrides: Partial<Site>): Site {
  return { id: 1, name: 'Site', url: 'https://site.example', ...overrides }
}

describe('resolveSiteBalanceUsd', () => {
  it('prefers the site-level totalBalance when present', () => {
    const site = makeSite({
      totalBalance: 42.5,
      subscriptionSummary: {
        activeCount: 1,
        totalRemainingUsd: 10,
      },
    })
    expect(resolveSiteBalanceUsd(site)).toBe(42.5)
  })

  it('falls back to the subscription remaining USD when totalBalance is missing', () => {
    const site = makeSite({
      subscriptionSummary: {
        activeCount: 1,
        totalRemainingUsd: 87.5,
      },
    })
    expect(resolveSiteBalanceUsd(site)).toBe(87.5)
  })

  it('respects a real zero totalBalance instead of falling through', () => {
    const site = makeSite({
      totalBalance: 0,
      subscriptionSummary: {
        activeCount: 1,
        totalRemainingUsd: 87.5,
      },
    })
    expect(resolveSiteBalanceUsd(site)).toBe(0)
  })

  it('returns null when neither source carries a number (caller renders em dash)', () => {
    expect(resolveSiteBalanceUsd(makeSite({}))).toBeNull()
    expect(
      resolveSiteBalanceUsd(
        makeSite({ subscriptionSummary: { activeCount: 0 } })
      )
    ).toBeNull()
    expect(
      resolveSiteBalanceUsd(
        makeSite({
          subscriptionSummary: { activeCount: 0, totalRemainingUsd: null },
        })
      )
    ).toBeNull()
  })
})
