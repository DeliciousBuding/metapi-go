import type { OverviewReport } from '@/lib/api'

export function reportFixture(
  overrides: Partial<OverviewReport> = {}
): OverviewReport {
  return {
    period: '7d',
    window: { from: '2026-09-29T04:00:00Z', to: '2026-10-06T04:00:00Z' },
    summary: {
      totalCount: 0,
      successCount: 0,
      failedCount: 0,
      totalTokensAll: 0,
      totalCost: 0,
      averageLatencyMs: null,
    },
    points: [],
    siteAvailability: [],
    models: [],
    ...overrides,
  }
}
