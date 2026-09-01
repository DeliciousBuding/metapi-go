// useUrlTableState — URL-sync guard regression tests.
//
// The P0 "sidebar Routes click snaps back" bug: a list page's table callbacks
// fire while the router is navigating away (the useLocation searchStr
// subscription re-renders the leaving page with the next location's search),
// and the old guard compared hrefs against `window.location.pathname` — which
// lags behind during an in-flight navigation because @tanstack/history
// coalesces queued push/replace calls into one microtask. The guard must
// compare against the router-side pathname instead. These tests pin the
// guard's exact-match semantics against the three scenarios that matter.

import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  useUrlTableState,
  type UrlTableState,
  type UrlTableStateUpdate,
} from '../use-url-table-state'

type Location = { searchStr: string; pathname: string }

const locationState = vi.hoisted(() => ({
  current: {
    searchStr: '?page=1&pageSize=20',
    pathname: '/channels',
  } as Location,
  navigate: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  useLocation: (opts: { select: (loc: Location) => string }) =>
    opts.select(locationState.current),
  useNavigate: () => locationState.navigate,
}))

type Filters = { status: string }

function readSearch(searchString?: string): UrlTableState<Filters> {
  const params = new URLSearchParams(searchString ?? '')
  return {
    q: params.get('q') ?? '',
    pageIndex: Number(params.get('page') ?? 0),
    pageSize: Number(params.get('pageSize') ?? 20),
    sorting: [],
    filters: { status: params.get('status') ?? '' },
  }
}

function buildHref(next: UrlTableStateUpdate<Filters>): string {
  const merged = { ...readSearch(locationState.current.searchStr), ...next }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== 20) params.set('pageSize', String(merged.pageSize))
  if (merged.filters?.status) params.set('status', merged.filters.status)
  const query = params.toString()
  return query ? `/channels?${query}` : '/channels'
}

describe('useUrlTableState URL-sync guard', () => {
  beforeEach(() => {
    locationState.current = {
      searchStr: '?page=1&pageSize=20',
      pathname: '/channels',
    }
    locationState.navigate.mockClear()
  })

  function render() {
    return renderHook(() =>
      useUrlTableState<Filters>({
        basePath: '/channels',
        read: readSearch,
        buildHref,
        toColumnFilters: () => [],
        fromColumnFilters: () => ({}),
      })
    )
  }

  it('writes back when still on this page', () => {
    const { result } = render()
    act(() => {
      result.current.updateUrlState({ pageIndex: 2 })
    })
    expect(locationState.navigate).toHaveBeenCalledTimes(1)
    expect(locationState.navigate).toHaveBeenCalledWith({
      href: '/channels?page=2',
      replace: true,
    })
  })

  it('blocks a write-back while the router is already on the target page (in-flight navigation)', () => {
    // The P0 repro: click sidebar "Routes" on /channels; the router location
    // store moves to /token-routes at navigation start, the leaving /channels
    // page re-renders with the target's search and its table callbacks fire.
    // The write-back must be a no-op.
    const { result, rerender } = render()
    locationState.current = { searchStr: '', pathname: '/token-routes' }
    rerender()
    act(() => {
      result.current.updateUrlState({ pageIndex: 0 })
    })
    expect(locationState.navigate).not.toHaveBeenCalled()
  })

  it('blocks leaving this page under any other target path', () => {
    const { result, rerender } = render()
    locationState.current = { searchStr: '', pathname: '/accounts' }
    rerender()
    act(() => {
      result.current.updateUrlState({ pageIndex: 1 })
    })
    expect(locationState.navigate).not.toHaveBeenCalled()
  })

  it('exact path match — a sibling sharing a prefix is not this page', () => {
    // Guards against the `startsWith` semantics of the old guard: an href for
    // /channels-extra would have passed `startsWith('/channels')`.
    const { result } = renderHook(() =>
      useUrlTableState<Filters>({
        basePath: '/channels',
        read: readSearch,
        buildHref: () => '/channels-extra?page=1',
        toColumnFilters: () => [],
        fromColumnFilters: () => ({}),
      })
    )
    act(() => {
      result.current.updateUrlState({ pageIndex: 1 })
    })
    expect(locationState.navigate).not.toHaveBeenCalled()
  })

  it('onPaginationChange routes through the guard and navigates when on page', () => {
    const { result } = render()
    act(() => {
      result.current.onPaginationChange({ pageIndex: 3, pageSize: 50 })
    })
    expect(locationState.navigate).toHaveBeenCalledWith({
      href: '/channels?page=3&pageSize=50',
      replace: true,
    })
  })
})

describe('useUrlTableState back-to-back updates (#1108 Reset race)', () => {
  // Two table callbacks firing in the same tick (Reset clearing column
  // filters AND the global filter) used to build both hrefs from the same
  // stale window.location snapshot — @tanstack/history coalesces same-tick
  // replaces to the LAST href, so the first update was silently dropped.
  // The hook now chains each update off the previous href.
  it('chains two same-tick updates so both land in the final href', async () => {
    locationState.current = {
      searchStr: '?q=x&status=active',
      pathname: '/channels',
    }
    locationState.navigate.mockClear()

    const buildHrefSpy = vi.fn(
      (next: UrlTableStateUpdate<Filters>, currentSearch?: string) => {
        const base = new URLSearchParams(
          currentSearch ?? locationState.current.searchStr
        )
        if (next.q !== undefined) {
          if (next.q) base.set('q', next.q)
          else base.delete('q')
        }
        if (next.filters?.status !== undefined) {
          if (next.filters.status) base.set('status', next.filters.status)
          else base.delete('status')
        }
        const query = base.toString()
        return query ? `/channels?${query}` : '/channels'
      }
    )

    const { result } = renderHook(() =>
      useUrlTableState<Filters>({
        basePath: '/channels',
        read: readSearch,
        buildHref: buildHrefSpy,
        toColumnFilters: () => [],
        fromColumnFilters: () => ({ filters: {} }),
      })
    )

    // Reset sequence: column filters first, then the global filter.
    act(() => {
      result.current.updateUrlState({ filters: { status: '' } })
      result.current.updateUrlState({ q: '' })
    })

    // The second navigate must carry BOTH updates — the chained href has no
    // q and no status (a full reset), not a half-reset.
    const lastNavigate = locationState.navigate.mock.calls.at(-1) as
      | [{ href: string }]
      | undefined
    expect(lastNavigate?.[0]?.href).toBe('/channels')
    // And the second buildHref received the first update's href as its
    // currentSearch (so it merged over the fresh state, not the stale URL).
    const secondBuildCall = buildHrefSpy.mock.calls[1]
    expect(secondBuildCall?.[1]).toBe('?q=x')
  })
})
