import type { OverviewReport } from '@/lib/api'

// A missing bucket is zero recorded traffic, never 100% success. Keep gaps in
// the success line while showing the whole selected time span on the axis.
export function overviewTrendRows(report?: OverviewReport) {
  if (!report) return []
  const hourly = report.period === '24h'
  const first = report.window.from ?? report.points[0]?.date
  if (!first) return []
  const cursor = new Date(first)
  if (hourly) cursor.setUTCMinutes(0, 0, 0)
  else cursor.setUTCHours(0, 0, 0, 0)
  const end = Date.parse(report.window.to)
  const points = new Map(report.points.map((point) => [point.date, point]))
  const rows = []
  while (cursor.getTime() <= end) {
    const date = hourly
      ? cursor.toISOString().replace('.000Z', 'Z')
      : cursor.toISOString().slice(0, 10)
    const point = points.get(date)
    const requests = point?.requests ?? 0
    rows.push({
      date,
      requests,
      successPercent: requests > 0 && point ? point.successRate * 100 : null,
    })
    cursor.setTime(cursor.getTime() + (hourly ? 3_600_000 : 86_400_000))
  }
  return rows
}
