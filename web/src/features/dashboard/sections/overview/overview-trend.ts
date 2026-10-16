import type { OverviewReport } from '@/lib/api'

export type OverviewTrendLevel = 'attempt' | 'request' | 'gateway-request'

// A missing bucket is zero recorded traffic, never 100% success. Keep gaps in
// the success line while showing the whole selected time span on the axis.
export function overviewTrendRows(
  report?: OverviewReport,
  level: OverviewTrendLevel = 'attempt'
) {
  if (!report) return []
  const hourly = report.period === '24h'
  let points = report.points.map((point) => ({
    date: point.date,
    requests: point.requests,
    successRate: point.successRate,
  }))
  if (level === 'request') points = report.requestPoints
  if (level === 'gateway-request')
    points = (report.gatewayRequestPoints ?? []).map((point) => ({
      ...point,
      successRate:
        point.requests > 0 ? point.completedCount / point.requests : 0,
    }))
  const first = report.window.from ?? points[0]?.date
  if (!first) return []
  const cursor = new Date(first)
  if (hourly) cursor.setUTCMinutes(0, 0, 0)
  else cursor.setUTCHours(0, 0, 0, 0)
  const end = Date.parse(report.window.to)
  const pointsByDate = new Map(points.map((point) => [point.date, point]))
  const rows = []
  while (cursor.getTime() <= end) {
    const date = hourly
      ? cursor.toISOString().replace('.000Z', 'Z')
      : cursor.toISOString().slice(0, 10)
    const point = pointsByDate.get(date)
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
