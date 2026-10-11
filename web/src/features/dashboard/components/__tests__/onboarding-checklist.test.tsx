// Account and independent upstreams share the same configuration journey.

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

function renderChecklist(props: {
  siteCount?: number
  accountCount?: number
  upstreamChannelCount?: number
}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <OnboardingChecklist upstreamChannelCount={0} {...props} />
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
  it('continues a direct-only configuration at the key without requiring a site or account', async () => {
    testState.routes = [{ id: 1 }]
    renderChecklist({ siteCount: 0, accountCount: 0, upstreamChannelCount: 3 })
    expect(
      await screen.findByRole('link', { name: /Issue key/i })
    ).toHaveAttribute('href', '/downstream-keys')
    expect(
      screen.queryByRole('link', { name: /Add account/i })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: /Connect upstream/i })
    ).not.toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('keeps an unknown independent inventory unknown instead of inventing a missing upstream', async () => {
    renderChecklist({
      siteCount: 0,
      accountCount: 0,
      upstreamChannelCount: undefined,
    })
    await waitFor(() => expect(mockGetKeys).toHaveBeenCalled())
    expect(
      screen.queryByRole('heading', { name: 'Welcome to Metapi' })
    ).not.toBeInTheDocument()
  })
  it('lists all three journey steps and puts the CTA on the first one', async () => {
    givenKeys([])
    renderChecklist({ siteCount: 0, accountCount: 0 })

    expect(
      await screen.findByRole('heading', {
        name: 'Welcome to Metapi',
        level: 2,
      })
    ).toBeInTheDocument()

    const steps = screen.getAllByRole('listitem')
    expect(steps).toHaveLength(3)
    expect(steps.map((step) => step.textContent)).toEqual([
      expect.stringContaining('Upstreams'),
      expect.stringContaining('Routes'),
      expect.stringContaining('Keys'),
    ])

    // One primary CTA, on the first gap: connect an upstream.
    const ctas = screen.getAllByRole('link')
    expect(ctas).toHaveLength(1)
    expect(ctas[0]).toHaveTextContent('Connect upstream')
    expect(ctas[0]).toHaveAttribute('href', '/channels')
    expect(ctas[0]).toHaveAttribute('data-search', 'null')
  })

  it('advances the CTA to the key step once sites, accounts and routes exist', async () => {
    // Distinct counts per step, so the assertions below cannot pass by
    // reading a neighbouring row's number.
    testState.routes = [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }, { id: 5 }]
    givenKeys([])
    renderChecklist({ siteCount: 2, accountCount: 3 })

    const cta = await screen.findByRole('link', { name: /Issue key/i })
    expect(cta).toHaveAttribute('href', '/downstream-keys')
    // The configured milestones report their stored resource counts.
    expect(screen.getAllByText('Configured')).toHaveLength(2)
    expect(screen.getAllByText('To do')).toHaveLength(1)
    // Each configured milestone reports its count: 3 upstream accounts / 5 routes.
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
  })

  it('offers the existing account path when a site is already configured', async () => {
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
