import '@testing-library/jest-dom/vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { CheckinStatusBadge } from '../checkin-columns'
import { CheckinDetailSheet } from '../checkin-detail-sheet'
import { CheckinPage } from '../checkin-page'
import { FailureReasonBadge } from '../failure-reason-badge'

const testState = vi.hoisted(() => ({
  logsQuery: {
    data: { items: [], total: 0 } as { items: unknown[]; total: number },
    isLoading: false,
    isFetching: false,
    dataUpdatedAt: 0,
    error: null as Error | null,
    refetch: vi.fn(),
  },
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: () => <div data-testid='checkin-table' />,
  useDataTable: () => ({ table: {} }),
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    filters: {
      status: '',
      reason: '',
      site: '',
      accountId: '',
      from: '',
      to: '',
    },
    updateUrlState: vi.fn(),
  }),
}))

vi.mock('@/features/accounts', () => ({
  useAccounts: () => ({
    data: { generatedAt: '', accounts: [{ id: 1, username: 'alice' }] },
    isLoading: false,
  }),
}))

vi.mock('@/features/sites', () => ({
  useSites: () => ({ data: [] }),
  isValidEndpointUrl: (url: string) => /^https?:\/\//i.test(url),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

vi.mock('../../api', () => ({
  useCheckinLogs: () => testState.logsQuery,
  useManualCheckin: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCheckinAccount: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    variables: null,
  }),
}))

vi.mock('../manual-checkin-dialog', () => ({
  ManualCheckinDialog: () => null,
}))

beforeEach(() => {
  testState.logsQuery = {
    data: { items: [], total: 0 },
    isLoading: false,
    isFetching: false,
    dataUpdatedAt: Date.now(),
    error: null,
    refetch: vi.fn(),
  }
})

afterEach(() => {
  vi.useRealTimers()
  cleanup()
})

describe('checkin dual-channel badges & quiet refresh', () => {
  it.each([
    ['auth', 'destructive', 'destructive'],
    ['verification', 'warning', 'warning'],
    ['network', 'info', 'info'],
    ['site', 'neutral', 'outline'],
    ['state', 'success', 'success'],
    ['unknown', 'neutral', 'outline'],
  ])(
    'FailureReasonBadge renders SoftToneBadge tone=%s for category=%s with tooltip',
    (category, tone, variant) => {
      const { container } = render(
        <FailureReasonBadge
          reason={{
            code: 'manual_turnstile_required',
            category,
            title: 'Manual verification',
            actionHint: 'Sign in once',
            detailHint: 'Cloudflare challenge detected',
          }}
        />
      )

      const badge = container.querySelector('[data-slot="badge"]')
      expect(badge).toHaveAttribute('data-tone', tone)
      expect(badge).toHaveAttribute('data-variant', variant)
      expect(badge?.getAttribute('title')).toBeTruthy()
      const marker = container.querySelector(`[data-tone-marker="${tone}"]`)
      expect(marker).not.toBeNull()
      expect(marker).toHaveAttribute('aria-hidden', 'true')
    }
  )

  it.each([
    ['success', 'success', 'success'],
    ['skipped', 'neutral', 'secondary'],
    ['failed', 'destructive', 'destructive'],
  ])(
    'CheckinStatusBadge renders SoftToneBadge tone=%s for status=%s',
    (status, tone, variant) => {
      const { container } = render(<CheckinStatusBadge status={status} />)
      const badge = container.querySelector('[data-slot="badge"]')
      expect(badge).toHaveAttribute('data-tone', tone)
      expect(badge).toHaveAttribute('data-variant', variant)
      expect(
        container.querySelector(`[data-tone-marker="${tone}"]`)
      ).not.toBeNull()
    }
  )

  it('CheckinDetailSheet renders dual-channel status & failure badges with tabular numerals', () => {
    render(
      <CheckinDetailSheet
        open
        onOpenChange={vi.fn()}
        row={{
          checkin_logs: {
            id: 42,
            accountId: 7,
            status: 'failed',
            message: '403 Turnstile challenge',
            reward: '+100',
            createdAt: '2026-11-28T01:02:03Z',
          },
          accounts: { id: 7, username: 'alice' },
          sites: { name: 'Example', url: 'https://example.com' },
          failureReason: {
            code: 'manual_turnstile_required',
            category: 'verification',
            title: 'Manual verification required',
            actionHint: 'Sign in manually',
            detailHint: 'Turnstile challenge',
          },
        }}
      />
    )

    expect(
      document.querySelector('[data-slot="badge"][data-tone="destructive"]')
    ).not.toBeNull()
    expect(
      document.querySelector('[data-slot="badge"][data-tone="warning"]')
    ).not.toBeNull()
    expect(screen.getByText('#42')).toHaveClass('tabular-nums')
    expect(screen.getByText('+100')).toHaveClass('tabular-nums')
    expect(screen.getByText('403 Turnstile challenge')).toBeInTheDocument()
  })

  it('CheckinPage displays QuietRefreshStatus and delays spinner on quiet refresh', () => {
    vi.useFakeTimers()
    testState.logsQuery.isFetching = true
    testState.logsQuery.dataUpdatedAt = Date.now()

    render(<CheckinPage />)

    expect(screen.getByTestId('quiet-refresh-status')).toBeInTheDocument()
    const refreshButton = screen.getByRole('button', { name: 'Refresh' })
    expect(refreshButton).toBeDisabled()
    expect(refreshButton.querySelector('svg')).not.toHaveClass('animate-spin')

    act(() => {
      vi.advanceTimersByTime(250)
    })
    expect(refreshButton.querySelector('svg')).toHaveClass('animate-spin')
  })

  it('CheckinPage header refresh button triggers refetch when idle', () => {
    render(<CheckinPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(testState.logsQuery.refetch).toHaveBeenCalledTimes(1)
  })
})
