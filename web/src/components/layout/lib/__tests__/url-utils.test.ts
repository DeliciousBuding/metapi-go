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
})
