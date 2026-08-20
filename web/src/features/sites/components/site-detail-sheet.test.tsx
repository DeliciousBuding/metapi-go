import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { SiteDetailSheet } from './site-detail-sheet'

const navigate = vi.hoisted(() => vi.fn())

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
}))

const site = {
  id: 7,
  name: 'Primary site',
  url: 'https://primary.example',
  platform: 'openai',
  status: 'active' as const,
}

beforeEach(() => {
  navigate.mockReset()
})

afterEach(() => cleanup())

describe('SiteDetailSheet guided-flow links', () => {
  it('uses the validated account-create and token-route destinations', () => {
    render(<SiteDetailSheet site={site} open onOpenChange={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: /Manage accounts/ }))
    fireEvent.click(screen.getByRole('button', { name: /Manage routes/ }))

    expect(navigate).toHaveBeenNthCalledWith(1, {
      to: '/accounts',
      search: { siteId: 7, create: true },
      replace: true,
    })
    expect(navigate).toHaveBeenNthCalledWith(2, {
      to: '/token-routes',
      search: { siteId: 7 },
      replace: true,
    })
  })
})

describe('SiteDetailSheet balance & subscription section', () => {
  it('is hidden when the backend returned no balance or subscription data', () => {
    render(<SiteDetailSheet site={site} open onOpenChange={vi.fn()} />)

    expect(screen.queryByText('Balance & subscription')).not.toBeInTheDocument()
  })

  it('renders the totalBalance and the subscription usage/plan rows', () => {
    const siteWithBalance = {
      ...site,
      totalBalance: 123.45,
      subscriptionSummary: {
        activeCount: 2,
        totalUsedUsd: 12.5,
        totalMonthlyLimitUsd: 100,
        totalRemainingUsd: 87.5,
        planNames: ['Pro'],
        nextExpiresAt: new Date(Date.now() + 2 * 3600 * 1000).toISOString(),
      },
    }
    render(
      <SiteDetailSheet site={siteWithBalance} open onOpenChange={vi.fn()} />
    )

    expect(screen.getByText('Balance & subscription')).toBeInTheDocument()
    expect(screen.getByText('$123.45')).toBeInTheDocument()
    expect(screen.getByText('$12.50 / $100.00')).toBeInTheDocument()
    // Remaining renders separately because totalBalance is present.
    expect(screen.getByText('$87.50')).toBeInTheDocument()
    expect(screen.getByText('Pro')).toBeInTheDocument()
    expect(screen.getByText(/in 2 hours/)).toBeInTheDocument()
  })

  it('falls back to the subscription remaining USD without duplicating it', () => {
    const siteWithRemainingOnly = {
      ...site,
      subscriptionSummary: {
        activeCount: 1,
        totalRemainingUsd: 87.5,
      },
    }
    render(
      <SiteDetailSheet
        site={siteWithRemainingOnly}
        open
        onOpenChange={vi.fn()}
      />
    )

    // One occurrence only: the balance row reuses the remaining figure and
    // the separate Remaining row is suppressed.
    expect(screen.getAllByText('$87.50')).toHaveLength(1)
  })
})

describe('SiteDetailSheet endpoint status', () => {
  it('shows the cooldown badge and the failure reason when present', () => {
    const siteWithEndpoints = {
      ...site,
      apiEndpoints: [
        {
          url: 'https://api.example/v1',
          enabled: true,
          cooldownUntil: new Date(Date.now() + 30 * 60 * 1000).toISOString(),
          lastFailureReason: 'upstream 503',
        },
      ],
    }
    render(
      <SiteDetailSheet site={siteWithEndpoints} open onOpenChange={vi.fn()} />
    )

    expect(screen.getByText('Cooling down')).toBeInTheDocument()
    expect(screen.getByText(/in 30 minutes/)).toBeInTheDocument()
    // The failure reason renders with the full text available via title.
    expect(screen.getByTitle('upstream 503')).toBeInTheDocument()
  })

  it('hides stale cooldowns and renders no status rows for healthy endpoints', () => {
    const siteWithHealthyEndpoint = {
      ...site,
      apiEndpoints: [
        {
          url: 'https://api.example/v1',
          enabled: true,
          cooldownUntil: new Date(Date.now() - 60 * 1000).toISOString(),
          lastFailureReason: null,
        },
      ],
    }
    render(
      <SiteDetailSheet
        site={siteWithHealthyEndpoint}
        open
        onOpenChange={vi.fn()}
      />
    )

    expect(screen.queryByText('Cooling down')).not.toBeInTheDocument()
    expect(screen.queryByText(/Failure reason/)).not.toBeInTheDocument()
  })
})
