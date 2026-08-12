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
 * Normalize URL by removing query parameters and trailing slashes
 */

/**
 * Check if a navigation item is active
 * @param href - Current URL
 * @param item - Navigation item
 * @param mainNav - Whether this is a main navigation item (matches first-level path)
 */
export function checkIsActive(
  href: string,
  item: NavItem,
  mainNav = false
): boolean {
  const hrefWithoutQuery = href.split('?')[0]

  if (item.activeUrls?.some((url) => urlToString(url) === hrefWithoutQuery)) {
    return true
  }

  // Prefix match for drill-in items (e.g. a settings subarea stays active for
  // any of its section URLs: /settings/general matches /settings/general/*).
  if (item.activePrefix) {
    const prefix = item.activePrefix.split('?')[0]
    const cleanPrefix = prefix.length > 1 ? prefix.replace(/\/+$/, '') : prefix
    const cleanHref =
      hrefWithoutQuery.length > 1
        ? hrefWithoutQuery.replace(/\/+$/, '')
        : hrefWithoutQuery
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
        const subItemUrlWithoutQuery = subItemUrl.split('?')[0]
        const subItemUrlHasQuery = subItemUrl.includes('?')
        if (subItemUrlWithoutQuery === hrefWithoutQuery) {
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

  const itemUrlWithoutQuery = itemUrl.split('?')[0]
  const itemUrlHasQuery = itemUrl.includes('?')

  // If both URLs have the same base path
  if (hrefWithoutQuery === itemUrlWithoutQuery) {
    if (!itemUrlHasQuery) return true
    if (itemUrlHasQuery && href === itemUrl) return true
  }

  // Main navigation match (matches first-level path)
  if (mainNav && href.split('/')[1] && itemUrl) {
    const hrefFirstPath = href.split('/')[1]
    const itemFirstPath = itemUrl.split('/')[1]
    return hrefFirstPath === itemFirstPath
  }

  return false
}
