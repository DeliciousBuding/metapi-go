import { describe, expect, it } from 'vitest'

import { overviewTrendRows } from '../overview-trend'
import { reportFixture } from './report-fixture'

describe('overview time buckets', () => {
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
})
