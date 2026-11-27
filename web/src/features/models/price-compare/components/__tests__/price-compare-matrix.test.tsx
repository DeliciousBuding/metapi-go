import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { PriceComparePage } from '../price-compare-page'

const workspaceState = vi.hoisted(() => ({
  navigate: vi.fn(),
  items: [
    {
      siteId: 1,
      siteName: 'Alpha Relay',
      platform: 'new-api',
      model: 'claude-3-7-sonnet',
      accountId: 11,
      username: 'alice',
      inputPerMillion: 2.0,
      outputPerMillion: 10.0,
      source: 'billing_details',
      ratesSource: 'billing_details',
      estimatedCostSample: 0.012,
      observedSamples: 0,
      configuredUnitCost: null,
      missingPrice: false,
      recommended: true,
    },
    {
      siteId: 2,
      siteName: 'Beta Relay',
      platform: 'one-api',
      model: 'claude-3-7-sonnet',
      accountId: 12,
      username: 'bob',
      inputPerMillion: 4.0,
      outputPerMillion: 20.0,
      source: 'billing_details',
      ratesSource: 'billing_details',
      estimatedCostSample: 0.024,
      observedSamples: 0,
      configuredUnitCost: null,
      missingPrice: false,
      recommended: false,
    },
  ],
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => workspaceState.navigate,
  useSearch: () => ({}),
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
      if (value !== undefined && value !== null) params.set(key, String(value))
    }
    const qs = params.toString()
    return (
      <a href={qs ? `${to}?${qs}` : (to ?? '')} {...rest}>
        {children}
      </a>
    )
  },
}))

vi.mock('../../api', () => ({
  usePriceWorkspace: () => ({
    data: { items: workspaceState.items },
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }),
}))

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
})

afterEach(() => cleanup())

describe('PriceComparePage compact RowDisclosure matrix', () => {
  it('shows winning site, price spread, and route workspace link collapsed, and expands full site table on demand', () => {
    render(<PriceComparePage />)

    // Collapsed summary row shows winning site, input->output rate, and spread.
    expect(screen.getByText('Alpha Relay')).toBeInTheDocument()
    expect(screen.getByText('$2.0000 → $10.0000')).toBeInTheDocument()
    expect(screen.getByText('-33%')).toBeInTheDocument()
    expect(screen.queryByText('Beta Relay')).not.toBeInTheDocument()

    const routeLink = screen.getByRole('link', {
      name: 'Open claude-3-7-sonnet in routes',
    })
    expect(routeLink).toHaveAttribute(
      'href',
      '/token-routes?workspaceModel=claude-3-7-sonnet'
    )

    const toggle = screen.getByRole('button', { name: 'claude-3-7-sonnet' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')

    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Beta Relay')).toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'claude-3-7-sonnet' })
    ).toBeInTheDocument()
  })
})
