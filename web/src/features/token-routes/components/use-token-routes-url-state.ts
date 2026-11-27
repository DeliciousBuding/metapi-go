import type { ColumnFiltersState } from '@tanstack/react-table'

import {
  useUrlTableState,
  type UrlTableState,
  type UrlTableStateUpdate,
} from '@/components/data-table'
import { asStringParam } from '@/lib/helpers/searchParams'

import { routesSearchSchema } from '../lib/routes-schema'

const DEFAULT_PAGE_SIZE = 20

/** Page-specific URL filters: the `enabled` column filter + chain context.
 * `routeId` is a one-shot drilldown param (proxy-log detail sheet) that the
 * page consumes into the detail sheet and then strips; it is deliberately
 * never serialized back into hrefs. */
type TokenRoutesUrlFilters = {
  enabled: string
  accountId: string
  tokenId: string
  siteId: string
  routeId: string
  workspaceModel: string
  workspaceEndpoint: string
  workspaceKeyId: string
  workspaceClientIp: string
}

/**
 * Parse the raw search string into URL table state, reusing the route's
 * `routesSearchSchema` so the derived state matches the loader's validated
 * search. The page uses a 1-based `page` param.
 */
function readTokenRoutesSearch(
  searchString: string
): UrlTableState<TokenRoutesUrlFilters> {
  const entries = Object.fromEntries(
    new URLSearchParams(
      searchString.startsWith('?') ? searchString.slice(1) : searchString
    ).entries()
  )
  const parsed = routesSearchSchema.safeParse(entries)
  const search = parsed.success ? parsed.data : null
  return {
    q: asStringParam(search?.q) ?? '',
    pageIndex: (search?.page ?? 1) - 1,
    pageSize: search?.pageSize ?? DEFAULT_PAGE_SIZE,
    sorting: [],
    filters: {
      enabled: asStringParam(search?.enabled) ?? '',
      accountId:
        search?.accountId === undefined ? '' : String(search.accountId),
      tokenId: search?.tokenId === undefined ? '' : String(search.tokenId),
      siteId: search?.siteId === undefined ? '' : String(search.siteId),
      routeId: search?.routeId === undefined ? '' : String(search.routeId),
      workspaceModel: asStringParam(search?.workspaceModel) ?? '',
      workspaceEndpoint: asStringParam(search?.workspaceEndpoint) ?? '',
      workspaceKeyId:
        search?.workspaceKeyId === undefined
          ? ''
          : String(search.workspaceKeyId),
      workspaceClientIp: asStringParam(search?.workspaceClientIp) ?? '',
    },
  }
}

/** Serialize a partial state update back to the token-routes href, merging
 *  over the CURRENT url state (preserves chain context accountId/siteId). */
function buildTokenRoutesHref(
  next: UrlTableStateUpdate<TokenRoutesUrlFilters>
): string {
  const current = readTokenRoutesSearch(window.location.search)
  const merged: UrlTableState<TokenRoutesUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex + 1))
  if (merged.pageSize !== DEFAULT_PAGE_SIZE) {
    params.set('pageSize', String(merged.pageSize))
  }
  if (merged.filters.enabled) params.set('enabled', merged.filters.enabled)
  if (merged.filters.accountId) {
    params.set('accountId', merged.filters.accountId)
  }
  if (merged.filters.siteId) params.set('siteId', merged.filters.siteId)
  if (merged.filters.tokenId) params.set('tokenId', merged.filters.tokenId)
  if (merged.filters.workspaceModel) {
    params.set('workspaceModel', merged.filters.workspaceModel)
  }
  if (merged.filters.workspaceEndpoint) {
    params.set('workspaceEndpoint', merged.filters.workspaceEndpoint)
  }
  if (Number(merged.filters.workspaceKeyId) > 0) {
    params.set('workspaceKeyId', merged.filters.workspaceKeyId)
  }
  if (merged.filters.workspaceClientIp) {
    params.set('workspaceClientIp', merged.filters.workspaceClientIp)
  }
  // Preserve the one-shot `edit` deep-link param across table-state
  // navigations (page-clamp / sort / filter) until the page's consumption
  // effect strips it — mirrors the sites page's buildHref guard for its
  // guided `create`/`edit` params.
  const guidedEdit = new URLSearchParams(window.location.search).get('edit')
  if (guidedEdit) params.set('edit', guidedEdit)
  const queryString = params.toString()
  return queryString ? `/token-routes?${queryString}` : '/token-routes'
}

export function useTokenRoutesUrlState() {
  return useUrlTableState<TokenRoutesUrlFilters>({
    basePath: '/token-routes',
    read: readTokenRoutesSearch,
    buildHref: buildTokenRoutesHref,
    toColumnFilters: (filters) => {
      const out: ColumnFiltersState = []
      const enabledValues = filters.enabled.split(',').filter(Boolean)
      if (enabledValues.length) {
        out.push({ id: 'enabled', value: enabledValues })
      }
      return out
    },
    fromColumnFilters: (filters) => {
      const enabledEntry = filters.find((filter) => filter.id === 'enabled')
      return {
        filters: {
          enabled: Array.isArray(enabledEntry?.value)
            ? enabledEntry.value.join(',')
            : '',
        },
      }
    },
    resetPageIndexOnFilterChange: true,
  })
}
