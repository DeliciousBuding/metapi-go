// metapi-go/layout — checkIsActive: which sidebar entry the current URL selects.
//
// The sidebar is the only navigation surface for the Settings workspace, so this
// has to be right for the three ways an entry can declare itself active, and
// each exists for a reason:
//
//   `url`          the entry's own target;
//   `activeUrls`   extra URLs that also select it (a section index that has to
//                  light up for its default section);
//   `activePrefix` a whole subtree (a settings subarea stays active for every
//                  section URL beneath it).
//
// Query and hash are stripped before comparing pathnames, so `/models#top` still
// highlights `/models`. A declared URL that *does* carry a query only matches an
// href carrying the same one — that is what keeps `/observability` and
// `/observability?section=traffic` two distinct entries instead of one stealing
// the other's highlight.

import type { LinkProps } from '@tanstack/react-router'

import type { NavCollapsible, NavItem } from '../types'

type NavUrl = LinkProps['to'] | (string & {})

/**
 * A declared nav URL as a comparable string. TanStack's `to` is either a string
 * or a `{ pathname, search }` object; anything else yields null, i.e. "never
 * active", rather than throwing on a malformed entry.
 */
function urlToString(url: NavUrl): string | null {
  if (typeof url === 'string') return url
  if (!url || typeof url !== 'object' || Array.isArray(url)) return null

  const { pathname, search } = url as Record<string, unknown>
  const path = typeof pathname === 'string' ? pathname : ''
  const query = typeof search === 'string' ? search : ''
  return path + query
}

/** Pathname only: query and hash are transient state, not identity. */
function stripQueryAndHash(url: string): string {
  return url.split('?')[0].split('#')[0]
}

/** Does the declared URL `candidate` select the current `href`? */
function selects(href: string, hrefPath: string, candidate: string): boolean {
  if (href === candidate) return true
  // A candidate that pins a query is exact-match-only: matching it on pathname
  // alone would claim every sibling section that shares its path.
  return !candidate.includes('?') && stripQueryAndHash(candidate) === hrefPath
}

/** Trailing slashes normalised, except that `/` itself stays `/`. */
function withoutTrailingSlash(path: string): string {
  return path.length > 1 ? path.replace(/\/+$/, '') : path
}

/**
 * Subtree match for `activePrefix`. The `prefix + '/'` form is what stops
 * `/settings/basic` from leaking onto `/settings/basics`.
 */
function matchesPrefix(hrefPath: string, activePrefix: string): boolean {
  const prefix = withoutTrailingSlash(activePrefix.split('?')[0])
  const path = withoutTrailingSlash(hrefPath)

  return path === prefix || path.startsWith(`${prefix}/`)
}

export function checkIsActive(href: string, item: NavItem): boolean {
  const hrefPath = stripQueryAndHash(href)

  if (
    item.activeUrls?.some((url) => {
      const declared = urlToString(url)
      // Query-aware in the opposite direction from `selects`: a bare-path entry
      // matches only while the href itself carries no query, so `/observability`
      // highlights the default-section item without also claiming the
      // `?section=…` variants that belong to their own entries.
      return (
        declared !== null &&
        (declared === href || (declared === hrefPath && !href.includes('?')))
      )
    })
  ) {
    return true
  }

  if (item.activePrefix && matchesPrefix(hrefPath, item.activePrefix)) {
    return true
  }

  // A branch is active when any child is, so the group stays open and highlighted
  // while the user is somewhere inside it.
  if ('items' in item && item.items) {
    const branch = item as NavCollapsible
    if (
      branch.items.some((child) => {
        const declared = child?.url ? urlToString(child.url) : null
        return declared !== null && selects(href, hrefPath, declared)
      })
    ) {
      return true
    }
  }

  const declared = item.url ? urlToString(item.url) : null
  return declared !== null && selects(href, hrefPath, declared)
}
