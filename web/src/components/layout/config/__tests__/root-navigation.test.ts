// metapi-go/layout — root navigation contract.

import { describe, expect, it } from 'vitest'

import { ROOT_NAVIGATION } from '../root-navigation'

describe('ROOT_NAVIGATION', () => {
  it('organizes the shell by operator task domains', () => {
    expect(ROOT_NAVIGATION.navGroups.map((group) => group.id)).toEqual([
      'overview',
      'upstream',
      'models-routes',
      'access',
      'requests-diagnostics',
      'settings',
    ])
  })

  it('preserves the existing root URLs and entry order within each domain', () => {
    expect(
      ROOT_NAVIGATION.navGroups.flatMap((group) =>
        group.items.map((item) => item.url)
      )
    ).toEqual([
      '/',
      '/sites',
      '/accounts',
      '/checkin',
      '/site-announcements',
      '/oauth',
      '/channels',
      '/token-routes',
      '/models',
      '/model-tester',
      '/price-compare',
      '/downstream-keys',
      '/proxy-logs',
      '/observability',
      '/settings',
      '/about',
    ])
  })
})
