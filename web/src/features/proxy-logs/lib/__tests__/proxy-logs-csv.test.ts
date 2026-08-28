// CSV export contract for the proxy-logs page: hostile cells (formula/DDE
// starters like `=1+1`, `-cmd|...`, `+x`, `@SUM`) must be neutralized with
// a single-quote prefix before reaching the downloaded file — model names
// are downstream-caller-controlled, so the export is a real injection
// surface. Normal cells keep their existing shape.

import { describe, expect, it } from 'vitest'

import type { ProxyLog } from '../../types'
import { csvEscape, proxyLogsToCsv } from '../proxy-logs-csv'

const translate = (key: string, params?: { defaultValue?: string }) =>
  params?.defaultValue ?? key

function makeLog(overrides: Partial<ProxyLog>): ProxyLog {
  return {
    id: 1,
    createdAt: '2026-08-22 12:00:00',
    modelRequested: 'gpt-5.5',
    modelActual: 'gpt-5.5',
    status: 'success',
    httpStatus: 200,
    latencyMs: 120,
    totalTokens: 30,
    retryCount: 0,
    ...overrides,
  }
}

describe('csvEscape', () => {
  it('neutralizes formula starters with a single-quote prefix', () => {
    expect(csvEscape('=1+1')).toBe("'=1+1")
    expect(csvEscape('+1+1')).toBe("'+1+1")
    expect(csvEscape('@SUM(A1:A2)')).toBe("'@SUM(A1:A2)")
    expect(csvEscape("-cmd|'/C calc'!A0")).toBe("'-cmd|'/C calc'!A0")
  })

  it('keeps negative numbers spreadsheet-numeric', () => {
    expect(csvEscape(-5.5)).toBe('-5.5')
  })

  it('still quotes commas, quotes and newlines', () => {
    expect(csvEscape('a,b')).toBe('"a,b"')
    expect(csvEscape('say "hi"')).toBe('"say ""hi"""')
    expect(csvEscape('line1\nline2')).toBe('"line1\nline2"')
  })

  it('renders null/undefined as empty cells', () => {
    expect(csvEscape(null)).toBe('')
    expect(csvEscape(undefined)).toBe('')
  })
})

describe('proxyLogsToCsv', () => {
  it('neutralizes hostile model/account/site cells in the body', () => {
    const csv = proxyLogsToCsv(
      [
        makeLog({
          modelRequested: '=1+1',
          modelActual: '=1+1',
          username: "-cmd|'/C calc'!A0",
          siteName: '=@SUM(site)',
        }),
        makeLog({
          id: 2,
          modelRequested: '@SUM(A1:A2)',
          modelActual: '',
          username: '+1+1',
          siteName: null,
          siteId: 7,
        }),
      ],
      translate
    )
    const [, firstRow, secondRow] = csv.split('\n')
    expect(firstRow).toContain(",'=1+1,")
    expect(firstRow).toContain(",'-cmd|'/C calc'!A0,")
    expect(firstRow).toContain(",'=@SUM(site),")
    expect(secondRow).toContain(",'@SUM(A1:A2),")
    expect(secondRow).toContain(",'+1+1,")
    expect(secondRow).toContain(',#7,')
  })

  it('leaves benign rows byte-identical to the pre-fix shape', () => {
    const csv = proxyLogsToCsv(
      [
        makeLog({
          username: 'alice',
          siteName: 'hub',
          estimatedCost: 0.05,
        }),
      ],
      translate
    )
    const bodyRow = csv.split('\n')[1]
    expect(bodyRow).toBe(
      '2026-08-22 12:00:00,200,success,gpt-5.5,alice,hub,120,30,0.05'
    )
  })
})
