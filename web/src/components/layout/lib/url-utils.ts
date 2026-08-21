// metapi-go/layout — url-utils ported from newapi. AGPL header stripped.
// checkIsActive drives sidebar active-state highlighting.

import type { LinkProps } from '@tanstack/react-router'

import type { NavItem, NavCollapsible } from '../types'

/**
 * Convert LinkProps['to'] to string
 * Handles both string URLs and object URLs (e.g., { pathname, search })
 */
function urlToString(url: LinkProps['to'] | (string & {})): string | null {
  if (typeof url === 'string') {
    return url
  }
  if (url && typeof url === 'object' && !Array.isArray(url)) {
    const urlObj = url as Record<string, unknown>
    const pathname = typeof urlObj.pathname === 'string' ? urlObj.pathname : ''
    const search = typeof urlObj.search === 'string' ? urlObj.search : ''
    return pathname + search
  }
  return null
}

/**
 * Strip both the query string and the hash fragment from a URL, leaving only
 * the pathname. The active-state comparison should ignore transient query /
 * hash state so `/models#top` still highlights the `/models` nav item.
 */
function stripQueryAndHash(url: string): string {
  return url.split('?')[0].split('#')[0]
}

/**
 * Check if a navigation item is active for the current href.
 * @param href - Current URL (may include query string + hash fragment)
 * @param item - Navigation item
 */
export function checkIsActive(href: string, item: NavItem): boolean {
  const hrefPath = stripQueryAndHash(href)

  // Active URLs are query-aware: an entry matches the *bare* path only when
  // the current href itself carries no query, so `/observability` highlights
  // the default-section item without also matching `/observability?section=…`
  // variants (which belong to their exact-match entries).
  if (
    item.activeUrls?.some((url) => {
      const activeUrl = urlToString(url)
      if (!activeUrl) return false
      return (
        activeUrl === href || (activeUrl === hrefPath && !href.includes('?'))
      )
    })
  ) {
    return true
  }

  // Prefix match for drill-in items (e.g. a settings subarea stays active for
  // any of its section URLs: /settings/general matches /settings/general/*).
  if (item.activePrefix) {
    const prefix = item.activePrefix.split('?')[0]
    const cleanPrefix = prefix.length > 1 ? prefix.replace(/\/+$/, '') : prefix
    const cleanHref =
      hrefPath.length > 1 ? hrefPath.replace(/\/+$/, '') : hrefPath
    if (cleanHref === cleanPrefix || cleanHref.startsWith(`${cleanPrefix}/`)) {
      return true
    }
  }

  // For collapsible items (NavCollapsible), check sub-items first
  if ('items' in item && item.items) {
    const collapsibleItem = item as NavCollapsible
    const items = collapsibleItem.items

    // Check if any sub-item matches
    if (
      items.some((i) => {
        if (!i?.url) return false
        const subItemUrl = urlToString(i.url)
        if (!subItemUrl) return false
        if (href === subItemUrl) return true
        const subItemUrlPath = stripQueryAndHash(subItemUrl)
        const subItemUrlHasQuery = subItemUrl.includes('?')
        if (subItemUrlPath === hrefPath) {
          if (!subItemUrlHasQuery) return true
          if (subItemUrlHasQuery && href === subItemUrl) return true
        }
        return false
      })
    ) {
      return true
    }
  }

  // For regular link items, check the item's URL
  if (!item.url) return false

  const itemUrl = urlToString(item.url)
  if (!itemUrl) return false

  // Exact match
  if (href === itemUrl) return true

  const itemUrlPath = stripQueryAndHash(itemUrl)
  const itemUrlHasQuery = itemUrl.includes('?')

  // If both URLs have the same base path
  if (hrefPath === itemUrlPath) {
    if (!itemUrlHasQuery) return true
    if (itemUrlHasQuery && href === itemUrl) return true
  }

  return false
}
