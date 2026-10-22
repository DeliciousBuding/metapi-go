import { describe, expect, it } from 'vitest'

import { readRequestSearch, requestSearchHref } from '../request-search'

describe('personal request URL state', () => {
  it('keeps filters when changing page and removes detail in one update', () => {
    const params = new URLSearchParams(
      requestSearchHref(
        { pageIndex: 2 },
        '?view=requests&page=1&q=model&status=completed&quotaState=settled&requestId=record'
      ).split('?')[1]
    )
    expect(params.get('page')).toBe('3')
    expect(params.get('q')).toBe('model')
    expect(params.get('status')).toBe('completed')
    expect(params.get('quotaState')).toBe('settled')
    expect(params.has('requestId')).toBe(false)
  })
  it('clears all filters and page without preserving hidden criteria', () => {
    const search = requestSearchHref(
      {
        q: '',
        filters: { status: undefined, quotaState: undefined },
        pageIndex: 0,
      },
      '?q=model&status=completed&quotaState=unknown&page=9'
    )
    expect(readRequestSearch(search.split('?')[1] ?? '')).toMatchObject({
      q: '',
      pageIndex: 0,
      filters: { status: undefined, quotaState: undefined },
    })
  })
  it('rejects invalid URL state and keeps known states', () => {
    expect(
      readRequestSearch('?page=-2&status=bogus&quotaState=bogus')
    ).toMatchObject({
      pageIndex: 0,
      filters: { status: undefined, quotaState: undefined },
    })
    expect(readRequestSearch('?quotaState=missing')).toMatchObject({
      filters: { quotaState: 'missing' },
    })
  })
})
