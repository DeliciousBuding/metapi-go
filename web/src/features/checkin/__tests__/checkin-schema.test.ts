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
  const parseShapeCases: Array<
    [
      string,
      Record<string, unknown>,
      {
        page: number
        pageSize: number
        accountId?: number | undefined
        status?: string | undefined
        q?: string | number | undefined
      },
    ]
  > = [
    [
      'applies page / pageSize defaults to an empty input',
      {},
      {
        page: 1,
        pageSize: 20,
        accountId: undefined,
        status: undefined,
        q: undefined,
      },
    ],
    [
      'coerces string numerics from a URL query string shape',
      { page: '2', pageSize: '50', accountId: '7' },
      { page: 2, pageSize: 50, accountId: 7 },
    ],
  ]

  it.each(parseShapeCases)('%s', (_label, input, expected) => {
    const result = checkinSearchSchema.parse(input as Record<string, unknown>)
    expect(result.page).toBe(expected.page)
    expect(result.pageSize).toBe(expected.pageSize)
    expect(result.accountId).toBe(expected.accountId)
    expect(result.status).toBe(expected.status)
    expect(result.q).toBe(expected.q)
  })

  it.each([
    ['non-numeric page', { page: 'abc' }, { page: 1 }],
    ['page below 1', { page: '0' }, { page: 1 }],
    ['pageSize above 200', { pageSize: '201' }, { pageSize: 20 }],
    ['pageSize below 1', { pageSize: '0' }, { pageSize: 20 }],
    ['non-positive accountId', { accountId: '-3' }, { accountId: undefined }],
    ['fractional accountId', { accountId: '1.5' }, { accountId: undefined }],
  ])('falls back instead of throwing for %s', (_label, input, fallback) => {
    const result = checkinSearchSchema.safeParse(input)
    expect(result.success).toBe(true)
    if (!result.success) return
    expect(result.data).toMatchObject(fallback)
  })

  it('tolerates router-parsed primitives without throwing', () => {
    const result = checkinSearchSchema.parse({
      page: 0,
      pageSize: 'bogus',
      accountId: 'bogus',
      q: 123,
      from: 2026,
      to: false,
    })
    expect(result.page).toBe(1)
    expect(result.pageSize).toBe(20)
    expect(result.accountId).toBeUndefined()
    expect(result.q).toBe(123)
    expect(result.from).toBe(2026)
    expect(result.to).toBe(false)
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
  it.each([
    [
      'a query string with a leading question mark',
      '?page=3&status=ok,fail',
      { page: 3, status: 'ok,fail' },
    ],
    [
      'a query string without a leading question mark',
      'page=2&pageSize=50&accountId=7',
      { page: 2, pageSize: 50, accountId: 7 },
    ],
    ['malformed input', 'page=abc&pageSize=999', { page: 1, pageSize: 20 }],
  ])('parses %s', (_label, input, expected) => {
    const result = parseCheckinSearch(input)
    expect(result).toMatchObject(expected)
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
