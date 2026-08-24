// metapi-go/features/token-routes — route price/allocation truth helpers.
//
// Pure-function coverage for the route-price-truth module: enabled/disabled
// share calculation (with zero-total and all-disabled edges), concrete-model
// resolution, price join by accountId + model, missing-price handling, and
// mixed cost/rate provenance. Fixtures build minimal RouteChannel /
// RouteSummaryRow / PriceCompareItem objects; no DOM or Tailwind snapshots.

import { describe, expect, it } from 'vitest'

import type { PriceCompareItem } from '@/features/models/price-compare/types'

import {
  calculateRouteChannelAllocations,
  formatRouteWeightShare,
  resolveConcreteModelForChannel,
  resolveDistinctConcreteModels,
  resolveRouteChannelPriceTruth,
} from '../lib/route-price-truth'
import type { RouteChannel, RouteSummaryRow } from '../types'

function makeChannel(overrides: Partial<RouteChannel> = {}): RouteChannel {
  return {
    id: 1,
    accountId: 10,
    tokenId: null,
    sourceModel: null,
    priority: 0,
    weight: 10,
    enabled: true,
    manualOverride: false,
    successCount: 0,
    failCount: 0,
    ...overrides,
  }
}

function makeRoute(overrides: Partial<RouteSummaryRow> = {}): RouteSummaryRow {
  return {
    id: 1,
    modelPattern: 'gpt-4o',
    displayName: null,
    displayIcon: null,
    routeMode: 'pattern',
    modelMapping: null,
    enabled: true,
    channelCount: 1,
    enabledChannelCount: 1,
    siteNames: [],
    decisionSnapshot: null,
    decisionRefreshedAt: null,
    ...overrides,
  }
}

function makePrice(
  overrides: Partial<PriceCompareItem> = {}
): PriceCompareItem {
  return {
    siteId: 0,
    siteName: '',
    platform: '',
    model: 'gpt-4o',
    accountId: 10,
    username: null,
    inputPerMillion: 2.5,
    outputPerMillion: 10,
    source: 'observed',
    ratesSource: 'observed',
    estimatedCostSample: 0,
    observedSamples: 0,
    configuredUnitCost: null,
    missingPrice: false,
    recommended: false,
    ...overrides,
  }
}

describe('calculateRouteChannelAllocations', () => {
  it('computes enabled share excluding disabled channels from the denominator', () => {
    const allocations = calculateRouteChannelAllocations([
      makeChannel({ id: 1, enabled: true, weight: 10 }),
      makeChannel({ id: 2, enabled: true, weight: 30 }),
      makeChannel({ id: 3, enabled: false, weight: 60 }),
    ])
    expect(allocations).toEqual([
      { channelId: 1, configuredWeight: 10, enabledWeightShare: 0.25 },
      { channelId: 2, configuredWeight: 30, enabledWeightShare: 0.75 },
      { channelId: 3, configuredWeight: 60, enabledWeightShare: null },
    ])
  })

  it('returns null share for a zero total weight', () => {
    const allocations = calculateRouteChannelAllocations([
      makeChannel({ id: 1, enabled: true, weight: 0 }),
      makeChannel({ id: 2, enabled: true, weight: 0 }),
    ])
    expect(
      allocations.map((allocation) => allocation.enabledWeightShare)
    ).toEqual([null, null])
  })

  it('returns null share when every channel is disabled', () => {
    const allocations = calculateRouteChannelAllocations([
      makeChannel({ id: 1, enabled: false, weight: 10 }),
    ])
    expect(allocations[0].enabledWeightShare).toBeNull()
  })
})

describe('resolveConcreteModelForChannel', () => {
  it('uses the channel sourceModel when it is a concrete model name', () => {
    const route = makeRoute({ modelPattern: 'gpt-4o' })
    const channel = makeChannel({ sourceModel: 'claude-3-5-sonnet' })
    expect(resolveConcreteModelForChannel(route, channel)).toBe(
      'claude-3-5-sonnet'
    )
  })

  it('falls back to the route model pattern for pattern routes', () => {
    const route = makeRoute({ modelPattern: 'gpt-4o', routeMode: 'pattern' })
    const channel = makeChannel({ sourceModel: null })
    expect(resolveConcreteModelForChannel(route, channel)).toBe('gpt-4o')
  })

  it('returns null for explicit-group routes without a concrete sourceModel', () => {
    const route = makeRoute({
      modelPattern: 'gpt-4*',
      routeMode: 'explicit_group',
    })
    const channel = makeChannel({ sourceModel: null })
    expect(resolveConcreteModelForChannel(route, channel)).toBeNull()
  })

  it('returns null for a pattern-only route model', () => {
    const route = makeRoute({ modelPattern: 'gpt-4*', routeMode: 'pattern' })
    const channel = makeChannel({ sourceModel: null })
    expect(resolveConcreteModelForChannel(route, channel)).toBeNull()
  })
})

describe('resolveDistinctConcreteModels', () => {
  it('deduplicates models case-insensitively', () => {
    const route = makeRoute({ modelPattern: 'gpt-4o' })
    const models = resolveDistinctConcreteModels(route, [
      makeChannel({ sourceModel: 'gpt-4o' }),
      makeChannel({ sourceModel: 'GPT-4O' }),
      makeChannel({ sourceModel: 'claude-3' }),
    ])
    expect(models).toEqual(['gpt-4o', 'claude-3'])
  })
})

describe('resolveRouteChannelPriceTruth', () => {
  it('joins price by accountId and concrete model', () => {
    const route = makeRoute({ modelPattern: 'gpt-4o' })
    const channel = makeChannel({ accountId: 10, sourceModel: 'gpt-4o' })
    const priceRow = makePrice({
      accountId: 10,
      model: 'gpt-4o',
      inputPerMillion: 2.5,
      outputPerMillion: 10,
    })
    const map = new Map([['gpt-4o', [priceRow]]])
    const truth = resolveRouteChannelPriceTruth(route, channel, map)
    expect(truth.concreteModel).toBe('gpt-4o')
    expect(truth.inputPerMillion).toBe(2.5)
    expect(truth.outputPerMillion).toBe(10)
    expect(truth.provenance.source).toBe('observed')
  })

  it('exposes null prices for a missing-price row', () => {
    const route = makeRoute({ modelPattern: 'gpt-4o' })
    const channel = makeChannel({ accountId: 10 })
    const priceRow = makePrice({
      accountId: 10,
      model: 'gpt-4o',
      missingPrice: true,
    })
    const map = new Map([['gpt-4o', [priceRow]]])
    const truth = resolveRouteChannelPriceTruth(route, channel, map)
    expect(truth.inputPerMillion).toBeNull()
    expect(truth.outputPerMillion).toBeNull()
  })

  it('returns null price when the accountId does not match', () => {
    const route = makeRoute({ modelPattern: 'gpt-4o' })
    const channel = makeChannel({ accountId: 99 })
    const priceRow = makePrice({ accountId: 10, model: 'gpt-4o' })
    const map = new Map([['gpt-4o', [priceRow]]])
    const truth = resolveRouteChannelPriceTruth(route, channel, map)
    expect(truth.price).toBeNull()
    expect(truth.inputPerMillion).toBeNull()
  })

  it('derives mixed cost/rate provenance from the row', () => {
    const route = makeRoute({ modelPattern: 'gpt-4o' })
    const channel = makeChannel({ accountId: 10 })
    const priceRow = makePrice({
      accountId: 10,
      model: 'gpt-4o',
      source: 'configured',
      ratesSource: 'observed',
    })
    const map = new Map([['gpt-4o', [priceRow]]])
    const truth = resolveRouteChannelPriceTruth(route, channel, map)
    expect(truth.provenance.source).toBe('configured')
    expect(truth.provenance.ratesSource).toBe('observed')
  })
})

describe('format helpers', () => {
  it('formats weight share as a percentage or an em dash for null', () => {
    expect(formatRouteWeightShare(0.25)).toBe('25.0%')
    expect(formatRouteWeightShare(null)).toBe('—')
  })
})
