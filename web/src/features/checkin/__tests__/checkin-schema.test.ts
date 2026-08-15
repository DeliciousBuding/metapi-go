import { describe, expect, it } from 'vitest'

import {
  buildInitialCheckinLogsQuery,
  checkinSearchSchema,
  getCheckinSearchDefaultValues,
  parseCheckinSearch,
  parseFilterValues,
} from '../lib/checkin-schema'

// ---------------------------------------------------------------------------
// checkinSearchSchema — defaults + coercion + bounds
// ---------------------------------------------------------------------------

describe('checkinSearchSchema', () => {
  it('applies page / pageSize defaults to an empty input', () => {
    const result = checkinSearchSchema.parse({})
    expect(result.page).toBe(1)
    expect(result.pageSize).toBe(20)
    expect(result.accountId).toBeUndefined()
    expect(result.status).toBeUndefined()
    expect(result.q).toBeUndefined()
  })

  it('coerces string numerics from a URL query string shape', () => {
    const result = checkinSearchSchema.parse({
      page: '2',
      pageSize: '50',
      accountId: '7',
    })
    expect(result.page).toBe(2)
    expect(result.pageSize).toBe(50)
    expect(result.accountId).toBe(7)
  })

  it.each([
    ['non-numeric page', { page: 'abc' }],
    ['page below 1', { page: '0' }],
    ['pageSize above 200', { pageSize: '201' }],
    ['pageSize below 1', { pageSize: '0' }],
    ['non-positive accountId', { accountId: '-3' }],
    ['fractional accountId', { accountId: '1.5' }],
  ])('rejects %s', (_label, input) => {
    const result = checkinSearchSchema.safeParse(input)
    expect(result.success).toBe(false)
  })

  it('preserves free-form filter / datetime-local strings', () => {
    const result = checkinSearchSchema.parse({
      status: 'ok,fail',
      reason: 'timeout',
      site: 'site-a',
      from: '2026-01-01T00:00',
      to: '2026-01-02T00:00',
      q: 'keyword',
    })
    expect(result.status).toBe('ok,fail')
    expect(result.from).toBe('2026-01-01T00:00')
    expect(result.q).toBe('keyword')
  })
})

// ---------------------------------------------------------------------------
// getCheckinSearchDefaultValues
// ---------------------------------------------------------------------------

describe('getCheckinSearchDefaultValues', () => {
  it('matches parse({})', () => {
    expect(getCheckinSearchDefaultValues()).toEqual(
      checkinSearchSchema.parse({})
    )
  })
})

// ---------------------------------------------------------------------------
// parseFilterValues
// ---------------------------------------------------------------------------

describe('parseFilterValues', () => {
  it('splits, trims, and drops empty segments', () => {
    expect(parseFilterValues('a, b,,c ')).toEqual(['a', 'b', 'c'])
  })

  it('returns an empty array for undefined / empty input', () => {
    expect(parseFilterValues(undefined)).toEqual([])
    expect(parseFilterValues('')).toEqual([])
    expect(parseFilterValues('   ')).toEqual([])
  })
})

// ---------------------------------------------------------------------------
// parseCheckinSearch — pure parser shared by the page + route loader
// ---------------------------------------------------------------------------

describe('parseCheckinSearch', () => {
  it('parses a query string with a leading question mark', () => {
    const result = parseCheckinSearch('?page=3&status=ok,fail')
    expect(result.page).toBe(3)
    expect(result.status).toBe('ok,fail')
  })

  it('parses a query string without a leading question mark', () => {
    const result = parseCheckinSearch('page=2&pageSize=50&accountId=7')
    expect(result.page).toBe(2)
    expect(result.pageSize).toBe(50)
    expect(result.accountId).toBe(7)
  })

  it('falls back to defaults on malformed input', () => {
    const result = parseCheckinSearch('page=abc&pageSize=999')
    expect(result.page).toBe(1)
    expect(result.pageSize).toBe(20)
  })

  it('falls back to defaults on an empty string', () => {
    const result = parseCheckinSearch('')
    expect(result).toEqual(getCheckinSearchDefaultValues())
  })
})

// ---------------------------------------------------------------------------
// buildInitialCheckinLogsQuery — pure loader payload builder
// ---------------------------------------------------------------------------

describe('buildInitialCheckinLogsQuery', () => {
  it('derives limit/offset from page/pageSize and forwards accountId + q', () => {
    const result = buildInitialCheckinLogsQuery(
      checkinSearchSchema.parse({ page: 2, pageSize: 50, accountId: 7, q: 'x' })
    )
    expect(result.limit).toBe(50)
    expect(result.offset).toBe(50)
    expect(result.accountId).toBe(7)
    expect(result.search).toBe('x')
  })

  it('splits comma-separated status/reason/site filters', () => {
    const result = buildInitialCheckinLogsQuery(
      checkinSearchSchema.parse({
        status: 'ok,fail',
        reason: 'timeout, quota',
        site: 'site-a',
      })
    )
    // status is single-select: only the first value is forwarded.
    expect(result.status).toBe('ok')
    expect(result.reason).toEqual(['timeout', 'quota'])
    expect(result.site).toEqual(['site-a'])
  })

  it('omits empty date bounds and empty q', () => {
    const result = buildInitialCheckinLogsQuery(checkinSearchSchema.parse({}))
    expect(result.from).toBeUndefined()
    expect(result.to).toBeUndefined()
    expect(result.search).toBeUndefined()
  })

  it('converts a provided date bound to a UTC RFC3339 string', () => {
    const result = buildInitialCheckinLogsQuery(
      checkinSearchSchema.parse({ from: '2026-01-01T00:00' })
    )
    // Timezone-dependent, so assert only the RFC3339 shape (no milliseconds).
    expect(result.from).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/)
    expect(result.to).toBeUndefined()
  })
})
