// Unit tests for the quota-payload classification helpers that keep the
// OAuth detail sheet honest (issue #887 S4).
//
// The three-state contract (`supported` / `unsupported` / `error` at the
// snapshot level, plus a per-window `supported` flag with nullable numbers)
// is easy to collapse into "render the number, defaulting to 0". These tests
// pin the distinction between "the provider says zero" and "the provider
// said nothing", which is the difference between a true and a fabricated
// quota reading.

import { describe, expect, it } from 'vitest'

import {
  hasOAuthSubscriptionDetails,
  resolveOAuthQuotaAvailability,
  resolveOAuthQuotaWindowState,
  type OAuthQuotaSnapshot,
} from '../lib/oauth-quota'

function buildSnapshot(
  overrides: Partial<OAuthQuotaSnapshot>
): OAuthQuotaSnapshot {
  return {
    status: 'supported',
    source: 'official',
    windows: {
      fiveHour: { supported: true, used: 1, limit: 10 },
      sevenDay: { supported: true, used: 2, limit: 20 },
    },
    ...overrides,
  } as OAuthQuotaSnapshot
}

describe('resolveOAuthQuotaAvailability', () => {
  it('reports a missing snapshot for null / undefined quota', () => {
    expect(resolveOAuthQuotaAvailability(null)).toBe('missing')
    expect(resolveOAuthQuotaAvailability(undefined)).toBe('missing')
  })

  it('distinguishes unsupported providers from failed syncs', () => {
    expect(
      resolveOAuthQuotaAvailability(buildSnapshot({ status: 'unsupported' }))
    ).toBe('unsupported')
    expect(
      resolveOAuthQuotaAvailability(buildSnapshot({ status: 'error' }))
    ).toBe('error')
  })

  it('reports a usable snapshot only when windows are present', () => {
    expect(resolveOAuthQuotaAvailability(buildSnapshot({}))).toBe('reported')
    expect(
      resolveOAuthQuotaAvailability(
        buildSnapshot({ windows: undefined as unknown as never })
      )
    ).toBe('missing')
  })
})

describe('resolveOAuthQuotaWindowState', () => {
  it('reports unsupported for absent or provider-unsupported windows', () => {
    expect(resolveOAuthQuotaWindowState(null)).toBe('unsupported')
    expect(resolveOAuthQuotaWindowState({ supported: false })).toBe(
      'unsupported'
    )
    // Numbers on an unsupported window are not trustworthy data.
    expect(
      resolveOAuthQuotaWindowState({ supported: false, used: 3, limit: 9 })
    ).toBe('unsupported')
  })

  it('reports noData for a supported window with every number absent', () => {
    expect(resolveOAuthQuotaWindowState({ supported: true })).toBe('noData')
    expect(
      resolveOAuthQuotaWindowState({
        supported: true,
        used: null,
        limit: null,
        remaining: null,
      })
    ).toBe('noData')
  })

  it('reports reported when any single number is present', () => {
    expect(resolveOAuthQuotaWindowState({ supported: true, used: 0 })).toBe(
      'reported'
    )
    expect(resolveOAuthQuotaWindowState({ supported: true, limit: 50 })).toBe(
      'reported'
    )
    expect(
      resolveOAuthQuotaWindowState({ supported: true, remaining: 0 })
    ).toBe('reported')
  })
})

describe('hasOAuthSubscriptionDetails', () => {
  it('is false when the snapshot or subscription is absent', () => {
    expect(hasOAuthSubscriptionDetails(null)).toBe(false)
    expect(hasOAuthSubscriptionDetails(buildSnapshot({}))).toBe(false)
    expect(
      hasOAuthSubscriptionDetails(buildSnapshot({ subscription: null }))
    ).toBe(false)
  })

  it('is false when every subscription field is null or blank', () => {
    expect(
      hasOAuthSubscriptionDetails(
        buildSnapshot({
          subscription: {
            planType: null,
            activeStart: null,
            activeUntil: null,
          },
        })
      )
    ).toBe(false)
    expect(
      hasOAuthSubscriptionDetails(
        buildSnapshot({ subscription: { planType: '   ' } })
      )
    ).toBe(false)
  })

  it('is true when at least one subscription field carries a value', () => {
    expect(
      hasOAuthSubscriptionDetails(
        buildSnapshot({ subscription: { planType: 'pro' } })
      )
    ).toBe(true)
    expect(
      hasOAuthSubscriptionDetails(
        buildSnapshot({ subscription: { activeUntil: '2026-09-01T00:00:00Z' } })
      )
    ).toBe(true)
  })
})
