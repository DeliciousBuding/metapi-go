// Behavior test for the dashboard's four-step onboarding checklist.
//
// Replaces the old "zero sites → Create site" banner, which retired the moment
// the first site existed and left the rest of the journey (account → route →
// key) unguided. Locks:
//   (a) the four steps render in journey order with the CTA on the FIRST gap
//       only — one next action, not four competing ones;
//   (b) each CTA points at the page that owns that step, with the `create`
//       deep link only where the page's search schema accepts it;
//   (c) the panel retires once every step is built;
//   (d) the panel never renders from an unanswered count (no flash of a
//       four-step "to do" list before the first byte, and no invented gap
//       when a source errors).

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { api } from '@/lib/api'

import { OnboardingChecklist } from '../onboarding-checklist'

const testState = vi.hoisted(() => ({
  routes: undefined as unknown[] | undefined,
}))

vi.mock('@/features/token-routes', () => ({
  useRoutes: () => ({ data: testState.routes }),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getDownstreamApiKeys: vi.fn(),
  },
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    search,
    children,
  }: {
    to?: string
    search?: unknown
    children: ReactNode
  }) => (
    <a href={to} data-search={JSON.stringify(search ?? null)}>
      {children}
    </a>
  ),
}))

const mockGetKeys = vi.mocked(api.getDownstreamApiKeys)

function renderChecklist(props: { siteCount?: number; accountCount?: number }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <OnboardingChecklist {...props} />
    </QueryClientProvider>
  )
}

/**
 * Arm the keys query BEFORE the render: react-query settles the observer on
 * whatever the mock returns at mount time, so arming afterwards would leave
 * the checklist parked on an unanswered count (which it must treat as
 * "unknown" and stay hidden).
 */
function givenKeys(items: unknown[]) {
  mockGetKeys.mockResolvedValue({ items })
}

beforeEach(() => {
  mockGetKeys.mockReset()
  givenKeys([])
  testState.routes = []
})

afterEach(() => cleanup())

describe('OnboardingChecklist', () => {
  it('lists all four journey steps and puts the CTA on the first one', async () => {
    givenKeys([])
    renderChecklist({ siteCount: 0, accountCount: 0 })

    expect(
      await screen.findByRole('heading', {
        name: 'Welcome to Metapi',
        level: 2,
      })
    ).toBeInTheDocument()

    const steps = screen.getAllByRole('listitem')
    expect(steps).toHaveLength(4)
    expect(steps.map((step) => step.textContent)).toEqual([
      expect.stringContaining('Sites'),
      expect.stringContaining('Accounts'),
      expect.stringContaining('Routes'),
      expect.stringContaining('Keys'),
    ])

    // Exactly one CTA, on the first gap: create a site.
    const ctas = screen.getAllByRole('link')
    expect(ctas).toHaveLength(1)
    expect(ctas[0]).toHaveTextContent('Create site')
    expect(ctas[0]).toHaveAttribute('href', '/sites')
    expect(ctas[0]).toHaveAttribute('data-search', '{"create":true}')
  })

  it('advances the CTA to the key step once sites, accounts and routes exist', async () => {
    // Distinct counts per step, so the assertions below cannot pass by
    // reading a neighbouring row's number.
    testState.routes = [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }, { id: 5 }]
    givenKeys([])
    renderChecklist({ siteCount: 2, accountCount: 3 })

    const cta = await screen.findByRole('link', { name: /Issue key/i })
    expect(cta).toHaveAttribute('href', '/downstream-keys')
    // The three built steps report Done with their real counts.
    expect(screen.getAllByText('Done')).toHaveLength(3)
    expect(screen.getAllByText('To do')).toHaveLength(1)
    // Each built row reports its real count: 2 sites / 3 accounts / 5 routes.
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
  })

  it('stops at the account step when the site exists but no account does', async () => {
    givenKeys([])
    renderChecklist({ siteCount: 1, accountCount: 0 })

    const cta = await screen.findByRole('link', { name: /Add account/i })
    expect(cta).toHaveAttribute('href', '/accounts')
    expect(cta).toHaveAttribute('data-search', '{"create":true}')
  })

  it('routes the CTA to the routes page (no create deep link there)', async () => {
    givenKeys([])
    renderChecklist({ siteCount: 1, accountCount: 1 })

    const cta = await screen.findByRole('link', { name: /Add route/i })
    expect(cta).toHaveAttribute('href', '/token-routes')
    // token-routes exposes no `create` search param, so none is invented.
    expect(cta).toHaveAttribute('data-search', 'null')
  })

  it('retires the panel once every step is built', async () => {
    testState.routes = [{ id: 1 }]
    givenKeys([{ id: 9 }])
    renderChecklist({ siteCount: 1, accountCount: 1 })

    await waitFor(() =>
      expect(
        screen.queryByRole('heading', { name: 'Welcome to Metapi' })
      ).not.toBeInTheDocument()
    )
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
  })

  it('does not flash the checklist before the snapshot count arrives', () => {
    renderChecklist({ siteCount: undefined, accountCount: undefined })

    expect(
      screen.queryByRole('heading', { name: 'Welcome to Metapi' })
    ).not.toBeInTheDocument()
  })

  it('does not invent a gap while the routes count is unanswered', async () => {
    testState.routes = undefined
    givenKeys([])
    renderChecklist({ siteCount: 1, accountCount: 1 })

    await waitFor(() => expect(mockGetKeys).toHaveBeenCalled())
    expect(
      screen.queryByRole('heading', { name: 'Welcome to Metapi' })
    ).not.toBeInTheDocument()
  })

  it('does not invent a gap when the key count fails to load', async () => {
    testState.routes = [{ id: 1 }]
    mockGetKeys.mockRejectedValue(new Error('keys unavailable'))
    renderChecklist({ siteCount: 1, accountCount: 1 })

    await waitFor(() => expect(mockGetKeys).toHaveBeenCalled())
    expect(
      screen.queryByRole('heading', { name: 'Welcome to Metapi' })
    ).not.toBeInTheDocument()
  })
})
