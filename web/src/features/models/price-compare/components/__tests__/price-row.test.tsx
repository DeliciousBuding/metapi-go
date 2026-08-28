// Behavior tests for the deep-link action added to each PriceRow. Asserts
// only the user-visible navigation affordance: each row surfaces a single
// link button that targets `/token-routes?q=<model>` (the routes page's
// global search filter) and exposes a model-specific accessible name so
// screen-reader users can distinguish rows. `@tanstack/react-router`'s
// Link is stubbed so the test exercises the wiring in isolation (no router
// context needed).
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import type { PriceCompareItem } from '../../types'
import { PriceRow } from '../price-compare-page'

vi.mock('@tanstack/react-router', () => ({
  // Stub Link: serialize `to` + `search` into a real <a href> so the test
  // can assert on the deep-link target the user will actually land on.
  // Spread `...rest` to mimic Base UI's Button↔render prop merge: the
  // Button forwards its DOM props (aria-label, children, className) onto
  // the rendered Link element, so the stub must forward them too.
  Link: ({
    to,
    search,
    children,
    ...rest
  }: {
    to?: string
    search?: Record<string, unknown>
    children?: ReactNode
    [key: string]: unknown
  }) => {
    const params = new URLSearchParams()
    for (const [key, value] of Object.entries(search ?? {})) {
      if (value !== undefined && value !== null) {
        params.set(key, String(value))
      }
    }
    const queryString = params.toString()
    const href = queryString ? `${to}?${queryString}` : (to ?? '')
    return (
      <a href={href} data-testid='price-row-route-link' {...rest}>
        {children}
      </a>
    )
  },
}))

beforeAll(() => {
  // jsdom does not ship matchMedia; Base UI's Button reads it on mount.
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
})

afterEach(() => cleanup())

// A known price-compare row used across tests. The schema parse happens
// upstream in the page (via priceCompareResponseSchema), so the test casts
// a plain object as PriceCompareItem — only the fields PriceRow reads
// directly (siteName, username, source, model, inputPerMillion,
// outputPerMillion, estimatedCostSample, missingPrice, recommended) need
// to be present.
function buildRow(overrides: Partial<PriceCompareItem> = {}): PriceCompareItem {
  return {
    siteId: 1,
    siteName: 'Primary site',
    platform: 'openai',
    model: 'gpt-5.5',
    accountId: 100,
    username: 'smoke-account',
    inputPerMillion: 2.5,
    outputPerMillion: 10,
    source: 'billing_details',
    ratesSource: 'billing_details',
    estimatedCostSample: 0.0125,
    observedSamples: 0,
    configuredUnitCost: null,
    missingPrice: false,
    recommended: false,
    ...overrides,
  } as PriceCompareItem
}

describe('PriceRow routes deep-link', () => {
  it('renders a link to /token-routes with the row model as the q search param', () => {
    render(<PriceRow row={buildRow({ model: 'gpt-5.5' })} />)

    const link = screen.getByTestId('price-row-route-link')
    expect(link).toHaveAttribute('href', '/token-routes?q=gpt-5.5')
  })

  it('encodes the model into the aria-label so screen-reader users can distinguish rows', () => {
    render(<PriceRow row={buildRow({ model: 'claude-4.5-sonnet' })} />)

    // The accessible name interpolates the model so a user scanning rows
    // hears which model each deep-link targets.
    const link = screen.getByTestId('price-row-route-link')
    expect(link).toHaveAccessibleName('Open claude-4.5-sonnet in routes')
  })

  it('renders the deep-link even when the row has no price signal (status column stays, action still useful)', () => {
    render(
      <PriceRow row={buildRow({ model: 'gpt-5.5', missingPrice: true })} />
    )

    const link = screen.getByTestId('price-row-route-link')
    expect(link).toHaveAttribute('href', '/token-routes?q=gpt-5.5')
    expect(link).toHaveAccessibleName('Open gpt-5.5 in routes')
  })

  it('URL-encodes models with special characters so the deep-link stays valid', () => {
    render(<PriceRow row={buildRow({ model: 'gpt-5-mini' })} />)

    // Hyphen is safe in a query param value, so the literal round-trips.
    const link = screen.getByTestId('price-row-route-link')
    expect(link).toHaveAttribute('href', '/token-routes?q=gpt-5-mini')
  })
})

describe('PriceRow currency display', () => {
  it('prefixes unit prices and the sample cost with the currency symbol', () => {
    render(
      <PriceRow
        row={buildRow({
          inputPerMillion: 2.5,
          outputPerMillion: 10,
          estimatedCostSample: 0.0125,
        })}
      />
    )

    // Unit prices keep 4 decimals, the sample cost keeps 6 — all carry $.
    expect(screen.getByText('$2.5000')).toBeInTheDocument()
    expect(screen.getByText('$10.0000')).toBeInTheDocument()
    expect(screen.getByText('$0.012500')).toBeInTheDocument()
  })

  it('explains the 6-decimal sample cost precision in a tooltip', () => {
    render(<PriceRow row={buildRow({ estimatedCostSample: 0.000001 })} />)

    const costCell = screen.getByText('$0.000001').closest('td')
    expect(costCell).not.toBeNull()
    expect(costCell).toHaveAttribute(
      'title',
      'Sample cost keeps 6 decimals so sub-cent per-call prices stay accurate.'
    )
  })

  it('renders a dash without the precision tooltip when the price is missing', () => {
    const { container } = render(
      <PriceRow row={buildRow({ missingPrice: true })} />
    )

    // The effective-cost cell falls back to a dash and no cell carries the
    // precision hint (unit-price columns keep their values).
    expect(container.querySelector('td[title]')).toBeNull()
    expect(screen.queryByText(/\$0\.0125/)).not.toBeInTheDocument()
  })
})
