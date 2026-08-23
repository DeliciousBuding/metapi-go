// Behavior tests for the heatmap time-axis tick row (issue: heatmap had no
// visible time-axis ticks — headers were 14px cells with title only). With
// N buckets the axis labels every ceil(N/6)-th track at 15px stride so ticks
// never collide; exact UTC hover metadata must survive on the cells.
//
// Live-verified against 127.0.0.1:4120 (see wave7-fix-l7b evidence).

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { OverviewSection } from '../overview-section'

function makeCells(hours: number[]): Array<Record<string, unknown>> {
  const cells: Array<Record<string, unknown>> = []
  for (const h of hours) {
    for (const k of [1, 2, 3]) {
      cells.push({
        bucket: `2026-08-23T${String(h).padStart(2, '0')}:00:00Z`,
        key: String(k),
        label: `site-${k}`,
        calls: h * 3 + k,
        tokens: 0,
        spend: 0,
      })
    }
  }
  return cells
}

vi.mock('../../../api', () => ({
  useSlowRequests: () => ({
    data: { items: [] },
    isLoading: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useUsageHeatmap: vi.fn(),
}))

import { useUsageHeatmap } from '../../../api'

const mockedHeatmap = vi.mocked(useUsageHeatmap)

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('OverviewSection heatmap time axis', () => {
  it('renders visible HH:00 ticks over a 6-bucket span, spaced without collision', () => {
    mockedHeatmap.mockReturnValue({
      data: { cells: makeCells([5, 6, 7, 8, 9, 10]) },
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
    })
    render(<OverviewSection />)

    // tickEvery = max(2, ceil(6/6)) = 2 → ticks at tracks 0, 2, 4
    const tick05 = screen.getByText('05:00')
    const tick07 = screen.getByText('07:00')
    const tick09 = screen.getByText('09:00')
    expect(tick05).toBeInTheDocument()
    expect(tick07).toBeInTheDocument()
    expect(tick09).toBeInTheDocument()
    // unlabeled tracks must not leak to the axis row
    expect(screen.queryByText('06:00')).not.toBeInTheDocument()
    expect(screen.queryByText('08:00')).not.toBeInTheDocument()
    expect(screen.queryByText('10:00')).not.toBeInTheDocument()
  })

  it('keeps the exact UTC bucket title on the hover-only cells', () => {
    mockedHeatmap.mockReturnValue({
      data: { cells: makeCells([5, 6, 7, 8, 9, 10]) },
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
    })
    const { container } = render(<OverviewSection />)

    const headers = container.querySelectorAll('[aria-label]')
    const bucketHeader = [...headers].find((el) =>
      el.getAttribute('aria-label')?.includes('UTC')
    )
    expect(bucketHeader).not.toBeNull()
    expect(bucketHeader?.getAttribute('aria-label')).toContain(
      '08-23 05:00 UTC'
    )
  })

  it('renders at least one tick when the seed holds a single non-empty bucket', () => {
    mockedHeatmap.mockReturnValue({
      data: { cells: makeCells([5]) },
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
    })
    render(<OverviewSection />)

    expect(screen.getByText('05:00')).toBeInTheDocument()
  })
})
