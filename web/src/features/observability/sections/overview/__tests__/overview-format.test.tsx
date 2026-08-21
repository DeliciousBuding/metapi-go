// Behavior tests for the observability Overview's display formatting
// (issue #889): slow-request timestamps render as localized date-time
// (never raw ISO), truncated model/site cells expose hover titles, and the
// usage heatmap labels its buckets as UTC so the grid is not mistaken for
// local time.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { OverviewSection } from '../overview-section'

const fixtures = vi.hoisted(() => {
  const SLOW_CREATED_AT_ISO = '2026-08-20T14:30:05Z'
  const slowItem = {
    id: 1,
    model: 'a-very-long-model-name-that-truncates',
    status: 'success',
    latencyMs: 12_345,
    firstByteLatencyMs: 900,
    httpStatus: 200,
    requestId: 'req-1',
    accountId: 1,
    siteId: 1,
    siteName: 'Primary site',
    createdAt: SLOW_CREATED_AT_ISO,
  }
  const heatmapCell = {
    bucket: '2026-08-20T14:00:00Z',
    key: 'site-1',
    label: 'Primary',
    calls: 7,
    tokens: 100,
    spend: 0.01,
  }
  return { SLOW_CREATED_AT_ISO, slowItem, heatmapCell }
})

const SLOW_CREATED_AT_ISO = fixtures.SLOW_CREATED_AT_ISO
const SLOW_CREATED_AT_EPOCH_MS = Date.UTC(2026, 7, 20, 14, 30, 5)

function expectedSlowDateTime(): string {
  return new Intl.DateTimeFormat('en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(SLOW_CREATED_AT_EPOCH_MS)
}

vi.mock('../../../api', () => ({
  useSlowRequests: () => ({
    data: { items: [fixtures.slowItem] },
    isLoading: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useUsageHeatmap: () => ({
    data: { cells: [fixtures.heatmapCell] },
    isLoading: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
  }),
}))

afterEach(() => cleanup())

describe('OverviewSection slow requests', () => {
  it('renders the created-at timestamp localized, not as raw ISO', () => {
    render(<OverviewSection />)

    expect(screen.getByText(expectedSlowDateTime())).toBeInTheDocument()
    expect(screen.queryByText(SLOW_CREATED_AT_ISO)).not.toBeInTheDocument()
  })

  it('renders sub-second latency in seconds and exposes model/site titles', () => {
    render(<OverviewSection />)

    expect(screen.getByText('12.3 s')).toBeInTheDocument()
    const modelCell = screen.getByText('a-very-long-model-name-that-truncates')
    expect(modelCell).toHaveAttribute(
      'title',
      'a-very-long-model-name-that-truncates'
    )
    const siteCell = screen.getByText('Primary site')
    expect(siteCell).toHaveAttribute('title', 'Primary site')
  })
})

describe('OverviewSection usage heatmap', () => {
  it('annotates the bucket axis as UTC', () => {
    render(<OverviewSection />)

    expect(screen.getByText('Bucket times are UTC')).toBeInTheDocument()
  })

  it('labels bucket cells with the UTC hour instead of the raw bucket string', () => {
    const { container } = render(<OverviewSection />)

    const bucketHeader = container.querySelector('[aria-label*="UTC"]')
    expect(bucketHeader).not.toBeNull()
    expect(bucketHeader?.getAttribute('title')).toContain('08-20 14:00 UTC')
  })
})
