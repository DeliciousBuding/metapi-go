// metapi-go/layout — unit tests for sidebar active-state matching.
//
// checkIsActive drives the drill-in sidebar highlight. The settings subarea
// items carry `activePrefix` so /settings/general stays highlighted on every
// section URL (/settings/general/*) instead of only the bare base path.

import { describe, expect, it } from 'vitest'

import type { NavLink } from '../../types'
import { checkIsActive } from '../url-utils'

function link(partial: Partial<NavLink>): NavLink {
  return { title: 'item', url: '/settings/general', ...partial }
}

describe('checkIsActive', () => {
  it('keeps exact-match behaviour for plain links', () => {
    const item = link({ url: '/settings' })
    expect(checkIsActive('/settings', item)).toBe(true)
    expect(checkIsActive('/settings/general', item)).toBe(false)
  })

  it('activates a subarea item on every one of its section URLs', () => {
    const item = link({
      url: '/settings/general/site',
      activePrefix: '/settings/general',
    })
    expect(checkIsActive('/settings/general/site', item)).toBe(true)
    expect(checkIsActive('/settings/general/auth', item)).toBe(true)
    expect(checkIsActive('/settings/general', item)).toBe(true)
  })

  it('does not leak the prefix to sibling paths', () => {
    const item = link({
      url: '/settings/general/site',
      activePrefix: '/settings/general',
    })
    expect(checkIsActive('/settings/generic', item)).toBe(false)
    expect(checkIsActive('/settings', item)).toBe(false)
    expect(checkIsActive('/accounts', item)).toBe(false)
  })

  it('matches the overview item only on the bare /settings path', () => {
    const item = link({ url: '/settings' })
    expect(checkIsActive('/settings?tab=a', item)).toBe(true)
    expect(checkIsActive('/settings/general', item)).toBe(false)
  })

  it('ignores the hash fragment when matching the item url', () => {
    const item = link({ url: '/models' })
    expect(checkIsActive('/models#top', item)).toBe(true)
    expect(checkIsActive('/models?tab=a#top', item)).toBe(true)
  })

  it('ignores the hash fragment for activePrefix drill-ins', () => {
    const item = link({
      url: '/settings/general/site',
      activePrefix: '/settings/general',
    })
    expect(checkIsActive('/settings/general/site#header', item)).toBe(true)
    expect(checkIsActive('/settings/general/auth#x', item)).toBe(true)
  })

  it('activates an activeUrls entry on the bare path (default section)', () => {
    const item = link({
      url: '/observability?section=overview',
      activeUrls: ['/observability'],
    })
    expect(checkIsActive('/observability', item)).toBe(true)
    expect(checkIsActive('/observability#top', item)).toBe(true)
  })

  it('does not leak the bare-path activeUrls match to other sections', () => {
    const overviewItem = link({
      url: '/observability?section=overview',
      activeUrls: ['/observability'],
    })
    const healthItem = link({ url: '/observability?section=health' })

    // On a non-default section URL only the exact-match entry is active.
    expect(checkIsActive('/observability?section=health', overviewItem)).toBe(
      false
    )
    expect(checkIsActive('/observability?section=health', healthItem)).toBe(
      true
    )
    // On the explicit default section URL the exact match still works.
    expect(checkIsActive('/observability?section=overview', overviewItem)).toBe(
      true
    )
  })

  it('matches activeUrls entries carrying their own query exactly', () => {
    const item = link({
      url: '/other',
      activeUrls: ['/page?tab=a'],
    })
    expect(checkIsActive('/page?tab=a', item)).toBe(true)
    expect(checkIsActive('/page?tab=b', item)).toBe(false)
    expect(checkIsActive('/page', item)).toBe(false)
  })
})
