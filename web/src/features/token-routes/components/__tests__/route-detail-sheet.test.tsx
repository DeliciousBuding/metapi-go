// Regression test for the route detail sheet's per-channel hit counters
// (successCount/failCount from the route-channel contract). Follows the
// sheet-test style: queries mocked at the api boundary, assertions on the
// rendered counters.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import i18n from 'i18next'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import type { RouteChannel, RouteSummaryRow } from '../../types'
import { RouteDetailSheet } from '../route-detail-sheet'

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })

  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    writable: true,
    value: ResizeObserverStub,
  })
})

const testState = vi.hoisted(() => ({
  channels: [] as RouteChannel[],
}))

vi.mock('../../api', () => ({
  useRouteChannels: () => ({
    data: testState.channels,
    isFetching: false,
  }),
  useClearRouteCooldown: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRebuildRoutes: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

// The price-true panel issues one useQueries entry per concrete model; this
// fixture route uses a wildcard pattern, so no queries are built. Stub the
// hook anyway so the sheet renders without a QueryClientProvider.
vi.mock('@tanstack/react-query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-query')>()
  return {
    ...actual,
    useQueries: () => [],
  }
})

vi.mock('@/features/models/price-compare/api', () => ({
  priceCompareQueryOptions: () => ({
    queryKey: ['price-compare-stub'],
    queryFn: () => Promise.resolve(null),
  }),
}))

function makeRoute(): RouteSummaryRow {
  return {
    id: 3,
    modelPattern: 'gpt-*',
    displayName: null,
    displayIcon: null,
    routeMode: 'pattern',
    modelMapping: null,
    routingStrategy: 'weighted',
    contextLength: null,
    enabled: true,
    channelCount: 2,
    enabledChannelCount: 2,
    siteNames: ['Site A'],
    decisionSnapshot: null,
    decisionRefreshedAt: null,
  }
}

function makeChannel(overrides: Partial<RouteChannel>): RouteChannel {
  return {
    id: 10,
    accountId: 1,
    tokenId: null,
    sourceModel: null,
    priority: 1,
    weight: 1,
    enabled: true,
    manualOverride: false,
    successCount: 0,
    failCount: 0,
    cooldownUntil: null,
    account: { username: 'alice' },
    site: { id: 1, name: 'Site A', platform: 'openai' },
    token: null,
    ...overrides,
  }
}

afterEach(() => {
  cleanup()
  testState.channels = []
})

describe('RouteDetailSheet channel hit counters', () => {
  it('renders the success/fail pair with the hit-count label', () => {
    testState.channels = [
      makeChannel({ id: 10, successCount: 123, failCount: 2 }),
    ]

    render(<RouteDetailSheet route={makeRoute()} open onOpenChange={vi.fn()} />)

    expect(screen.getByText('Success / fail')).toBeInTheDocument()
    expect(screen.getByText('123')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
  })

  it('keeps zero counts visible and honest', () => {
    testState.channels = [
      makeChannel({ id: 11, successCount: 0, failCount: 0 }),
    ]

    render(<RouteDetailSheet route={makeRoute()} open onOpenChange={vi.fn()} />)

    expect(
      screen.getByText('0', { selector: '.text-success' })
    ).toBeInTheDocument()
  })
})

// A channel with no token cannot serve anything, and the label used to be a bare
// "Unbound" -- which is how an operator ends up looking for a binding form that
// does not exist. Tokens are synced from the account; there is no manual step.
describe('RouteDetailSheet channel credential guidance', () => {
  it('names the mechanism and the action when a channel has no token', () => {
    testState.channels = [makeChannel({ id: 12, tokenId: null, token: null })]

    render(<RouteDetailSheet route={makeRoute()} open onOpenChange={vi.fn()} />)

    const hint = String(i18n.t('tokenRoutes.detail.channelTokenUnboundHint'))
    const line = screen.getByTitle(hint)
    expect(line).toBeInTheDocument()
    expect(line).toHaveTextContent(
      String(i18n.t('tokenRoutes.detail.channelTokenUnbound'))
    )
  })

  it('does not warn about a channel that does have a token', () => {
    testState.channels = [makeChannel({ id: 13, tokenId: 7, token: null })]

    render(<RouteDetailSheet route={makeRoute()} open onOpenChange={vi.fn()} />)

    expect(
      screen.queryByTitle(
        String(i18n.t('tokenRoutes.detail.channelTokenUnboundHint'))
      )
    ).not.toBeInTheDocument()
    expect(
      screen.getByText(
        String(i18n.t('tokenRoutes.detail.fallbackToken', { id: 7 })),
        { exact: false }
      )
    ).toBeInTheDocument()
  })
})
