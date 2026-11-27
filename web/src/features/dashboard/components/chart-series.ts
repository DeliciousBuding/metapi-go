import type { IncomeOutcomePoint, SiteTrendPoint } from '../types'

/** Number of ordinal colours exposed by the theme (--chart-1..5). */
const PALETTE_SIZE = 5

/** CSS var() reference for the i-th ordinal chart colour (cycles 1..5). */
export function chartColor(index: number): string {
  return `var(--chart-${(index % PALETTE_SIZE) + 1})`
}

/** Safe, stable dataKey for the i-th dynamic series (keeps CSS-var names valid). */
export function seriesKey(index: number): string {
  return `s${index}`
}

type IncomeOutcomeRow = { day: string; income: number; outcome: number }

export function pivotIncomeOutcome(
  data: IncomeOutcomePoint[]
): IncomeOutcomeRow[] {
  const byDay = new Map<string, IncomeOutcomeRow>()
  for (const point of data) {
    let row = byDay.get(point.day)
    if (!row) {
      row = { day: point.day, income: 0, outcome: 0 }
      byDay.set(point.day, row)
    }
    if (point.type === 'income') {
      row.income = point.value
    } else if (point.type === 'outcome') {
      row.outcome = point.value
    }
  }
  return [...byDay.values()]
}

export function pivotSiteTrend(data: SiteTrendPoint[]): {
  rows: Array<Record<string, string | number>>
  sites: string[]
} {
  const siteIndex = new Map<string, number>()
  for (const point of data) {
    if (!siteIndex.has(point.site)) {
      siteIndex.set(point.site, siteIndex.size)
    }
  }
  const sites = [...siteIndex.keys()]
  const byDate = new Map<string, Record<string, string | number>>()
  for (const point of data) {
    let row = byDate.get(point.date)
    if (!row) {
      row = { date: point.date }
      byDate.set(point.date, row)
    }
    row[seriesKey(siteIndex.get(point.site) ?? 0)] = point.spend
  }
  return { rows: [...byDate.values()], sites }
}

type LatencyTrendRow = { date: string; s0?: number; s1?: number }

export function pivotLatencyTrend(
  data: Array<{ date: string; metric: string; latency: number }>,
  avgLabel: string,
  p95Label: string
): LatencyTrendRow[] {
  const byDate = new Map<string, LatencyTrendRow>()
  for (const point of data) {
    let row = byDate.get(point.date)
    if (!row) {
      row = { date: point.date }
      byDate.set(point.date, row)
    }
    if (point.metric === avgLabel) {
      row.s0 = point.latency
    } else if (point.metric === p95Label) {
      row.s1 = point.latency
    }
  }
  return [...byDate.values()]
}
