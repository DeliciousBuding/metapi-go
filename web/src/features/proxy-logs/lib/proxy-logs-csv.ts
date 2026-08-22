// metapi-go features/proxy-logs/lib — CSV export builder.
//
// Lives outside the page component so the export contract (quoting rules +
// formula-injection neutralization) is unit-testable.

import { neutralizeCsvFormulaCell } from '@/lib/helpers/csv-injection'

import type { ProxyLog } from '../types'

// CSV export column header keys. The proxy_logs table does not persist the
// HTTP method or upstream path (only the downstream trace surface does), so
// the CSV sticks to the columns the /api/stats/proxy-logs response actually
// returns. httpStatus IS in the response (pl.*) even though the list type
// historically omitted it, so it is surfaced here as a bonus column.
export const PROXY_LOGS_CSV_COLUMNS = [
  'timestamp',
  'httpStatus',
  'status',
  'model',
  'account',
  'site',
  'duration',
  'tokens',
  'estimatedCost',
] as const

/**
 * Escape one CSV cell: formula/DDE starters are neutralized with a `'`
 * prefix (model names are downstream-caller-controlled, so hostile
 * `=cmd|...` payloads must stay inert when the export is opened in a
 * spreadsheet), and cells containing the CSV metacharacters are quoted
 * with inner quotes doubled.
 */
export function csvEscape(value: string | number | null | undefined): string {
  if (value === null || value === undefined) return ''
  const text = neutralizeCsvFormulaCell(String(value))
  if (/[",\n\r]/.test(text)) {
    return `"${text.replaceAll('"', '""')}"`
  }
  return text
}

export function proxyLogsToCsv(
  rows: ProxyLog[],
  t: (key: string, params?: Record<string, unknown>) => string
): string {
  const header = PROXY_LOGS_CSV_COLUMNS.map((column) =>
    csvEscape(
      t(`proxyLogs.page.exportCsvColumn.${column}`, { defaultValue: column })
    )
  ).join(',')
  const body = rows
    .map((log) => {
      const accountLabel =
        log.username || (log.accountId ? `#${log.accountId}` : '')
      const siteLabel = log.siteName || (log.siteId ? `#${log.siteId}` : '')
      const modelLabel =
        log.modelActual?.trim() || log.modelRequested?.trim() || ''
      const cells = [
        log.createdAt,
        log.httpStatus ?? '',
        log.status,
        modelLabel,
        accountLabel,
        siteLabel,
        log.latencyMs ?? '',
        log.totalTokens ?? '',
        log.estimatedCost ?? '',
      ]
      return cells.map(csvEscape).join(',')
    })
    .join('\n')
  return `${header}\n${body}`
}
