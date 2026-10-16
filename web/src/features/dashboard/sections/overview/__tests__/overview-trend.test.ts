import { describe, expect, it } from 'vitest'

import { overviewTrendRows } from '../overview-trend'
import { reportFixture } from './report-fixture'

describe('overview time buckets', () => {
  it('never stitches legacy attempt results into gateway request completion', () => {
    const rows = overviewTrendRows(
      reportFixture({
        window: { from: '2026-10-04T12:00:00Z', to: '2026-10-05T12:00:00Z' },
        requestPoints: [
          {
            date: '2026-10-04',
            requests: 100,
            successCount: 100,
            successRate: 1,
          },
        ],
        gatewayRequestPoints: [
          {
            date: '2026-10-05',
            requests: 4,
            completedCount: 1,
            failedCount: 1,
            cancelledCount: 1,
            rejectedCount: 1,
          },
        ],
      }),
      'gateway-request'
    )
    expect(rows).toEqual([
      { date: '2026-10-04', requests: 0, successPercent: null },
      { date: '2026-10-05', requests: 4, successPercent: 25 },
    ])
  })
  it('preserves empty days without presenting them as healthy traffic', () => {
    const rows = overviewTrendRows(
      reportFixture({
        window: { from: '2026-10-04T12:00:00Z', to: '2026-10-06T12:00:00Z' },
        points: [
          {
            date: '2026-10-05',
            requests: 2,
            successCount: 1,
            successRate: 0.5,
            cost: 1,
            tokens: 10,
          },
        ],
      })
    )
    expect(rows).toEqual([
      { date: '2026-10-04', requests: 0, successPercent: null },
      { date: '2026-10-05', requests: 2, successPercent: 50 },
      { date: '2026-10-06', requests: 0, successPercent: null },
    ])
  })
  it('uses hourly buckets for 24h and starts all-time at the first retained record', () => {
    const rows = overviewTrendRows(
      reportFixture({
        period: '24h',
        window: { from: '2026-10-05T12:30:00Z', to: '2026-10-06T12:30:00Z' },
      })
    )
    expect(rows).toHaveLength(25)
    expect(rows[0].date).toBe('2026-10-05T12:00:00Z')
    expect(
      overviewTrendRows(
        reportFixture({ period: 'all', window: { to: '2026-10-06T12:00:00Z' } })
      )
    ).toEqual([])
  })
  it('builds request-level rows from requestPoints', () => {
    const rows = overviewTrendRows(
      reportFixture({
        window: { from: '2026-10-04T12:00:00Z', to: '2026-10-06T12:00:00Z' },
        requestPoints: [
          {
            date: '2026-10-05',
            requests: 3,
            successCount: 2,
            successRate: 2 / 3,
          },
        ],
      }),
      'request'
    )
    expect(rows).toEqual([
      { date: '2026-10-04', requests: 0, successPercent: null },
      { date: '2026-10-05', requests: 3, successPercent: 66.66666666666666 },
      { date: '2026-10-06', requests: 0, successPercent: null },
    ])
  })
})
